package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"nebula/internal"
)

type Node struct {
	Chain *internal.Blockchain
	Pool  []internal.Transaction
	sync.Mutex
}

func main() {
	genesisAddress := "c8a2b70333857af1d28aeef93e0ab9acb656662d"

	blockchain, err := internal.NewBlockchain(genesisAddress, "data/blockchain")
	if err != nil {
		log.Fatal(err)
	}
	CloseOnProgramEnd(blockchain)

	node := &Node{
		Chain: blockchain,
		Pool:  []internal.Transaction{},
	}

	http.HandleFunc("/tx", node.HandleTx)
	http.HandleFunc("/blocks", node.HandleBlocks)
	http.HandleFunc("/tx/confirm", node.HandleConfirm)
	http.HandleFunc("/tx/pool", node.HandleMempool)
	http.HandleFunc("/block", node.HandleSubmitBlock)

	log.Println("Nebula node running at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func CloseOnProgramEnd(blockchain *internal.Blockchain) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Printf("[SHUTDOWN] Received signal %s. Closing blockchain DB...", sig)
		err := blockchain.Close()
		if err != nil {
			log.Fatalf("[ERROR] Failed to close blockchain DB: %v", err)
		}
		log.Println("[SHUTDOWN] Blockchain DB closed successfully. Exiting now.")
		os.Exit(0)
	}()
}

func (n *Node) HandleTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "invalid method", http.StatusMethodNotAllowed)
		return
	}

	var tx internal.Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		http.Error(w, "invalid tx", http.StatusBadRequest)
		return
	}

	addr, err := internal.RecoverAddressFromTransaction(tx)
	if err != nil || addr != tx.From {
		http.Error(w, "signature mismatch", http.StatusBadRequest)
		log.Printf("[REJECTED] Invalid signature from claimed address %s\n", tx.From)
		log.Printf("[DEBUG] err: %v, recovered addr: %s\n", err, addr)
		return
	}

	n.Lock()
	defer n.Unlock()

	balance := n.Chain.GetBalanceWithPending(tx.From, n.Pool)
	totalCost := tx.Price + tx.Fee
	if balance < totalCost {
		http.Error(w, "insufficient funds", http.StatusBadRequest)
		log.Printf("[REJECTED] Not enough funds for %s: has %.10f, needs %.10f (including fee %.10f)\n", tx.From, balance, totalCost, tx.Fee)
		return
	}

	n.Pool = append(n.Pool, tx)
	log.Printf("[TX RECEIVED] %s -> %s (%s) amount %.10f\n", tx.From, tx.To, tx.Type, tx.Price)
	w.WriteHeader(http.StatusOK)
}

func (n *Node) HandleBlocks(w http.ResponseWriter, r *http.Request) {
	n.Lock()
	defer n.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n.Chain.Blocks)
}

func (n *Node) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	confirmations := 0

	n.Lock()
	defer n.Unlock()

	for i := len(n.Chain.Blocks) - 1; i >= 0; i-- {
		for _, tx := range n.Chain.Blocks[i].Transactions {
			if tx.Hash() == hash {
				confirmations = len(n.Chain.Blocks) - i - 1
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"confirmations": confirmations})
}

func (n *Node) HandleMempool(w http.ResponseWriter, r *http.Request) {
	n.Lock()
	defer n.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(n.Pool); err != nil {
		http.Error(w, "failed to encode mempool", http.StatusInternalServerError)
	}
}

func (n *Node) HandleSubmitBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "invalid method", http.StatusMethodNotAllowed)
		return
	}

	var block internal.Block
	if err := json.NewDecoder(r.Body).Decode(&block); err != nil {
		http.Error(w, "invalid block data", http.StatusBadRequest)
		return
	}

	n.Lock()
	defer n.Unlock()

	if err := n.Chain.ValidateBlock(&block); err != nil {
		http.Error(w, "block validation failed: "+err.Error(), http.StatusBadRequest)
		log.Printf("[BLOCK REJECTED] #%d: %v", block.Index, err)
		return
	}

	latest := n.Chain.GetLatestBlock()
	if block.PrevHash != latest.Hash {
		http.Error(w, "block PrevHash mismatch with chain tip", http.StatusBadRequest)
		return
	}

	if err := n.Chain.AddBlock(&block); err != nil {
		http.Error(w, "failed to persist block", http.StatusInternalServerError)
		log.Printf("[ERROR] Saving block: %v", err)
		return
	}

	// Remove included txs
	included := make(map[string]struct{})
	for _, tx := range block.Transactions {
		included[tx.Hash()] = struct{}{}
	}
	newPool := n.Pool[:0]
	for _, tx := range n.Pool {
		if _, found := included[tx.Hash()]; !found {
			newPool = append(newPool, tx)
		}
	}
	n.Pool = newPool

	log.Printf("[BLOCK ACCEPTED] #%d (%d txs)", block.Index, len(block.Transactions))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("block accepted"))
}
