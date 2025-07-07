package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"nebula/internal"
)

type Node struct {
	Chain *internal.Blockchain
	Pool  []internal.Transaction
	Peers []string
	sync.Mutex
}

func main() {
	config, err := internal.LoadConfig("nebula.conf")
	if err != nil {
		log.Printf("[WARN] Failed to load nebula.conf, using defaults: %v\n", err)
	}

	blockchain, err := internal.NewBlockchain(config.RewardAddress, config.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	CloseOnProgramEnd(blockchain)

	node := &Node{
		Chain: blockchain,
		Pool:  []internal.Transaction{},
		Peers: config.BootstrapPeers,
	}

	go node.SyncLoop()

	http.HandleFunc("/tx", node.HandleTx)
	http.HandleFunc("/blocks", node.HandleBlocks)
	http.HandleFunc("/tx/confirm", node.HandleConfirm)
	http.HandleFunc("/tx/pool", node.HandleMempool)
	http.HandleFunc("/block", node.HandleSubmitBlock)
	http.HandleFunc("/peers", node.HandlePeers)

	log.Printf("Nebula node '%s' running at :%s\n", config.NodeName, config.Port)
	log.Fatal(http.ListenAndServe(":"+config.Port, nil))
}

func CloseOnProgramEnd(blockchain *internal.Blockchain) {
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Printf("[SHUTDOWN] Received signal %s. Closing blockchain DB...", sig)
		if err := blockchain.Close(); err != nil {
			log.Fatalf("[ERROR] Failed to close blockchain DB: %v", err)
		}
		log.Println("[SHUTDOWN] Blockchain DB closed successfully. Exiting now.")
		os.Exit(0)
	}()
}

func (n *Node) SyncLoop() {
	for {
		time.Sleep(10 * time.Second)
		for _, peer := range n.Peers {
			// Sync blockchain
			resp, err := http.Get(peer + "/blocks")
			if err == nil {
				var theirBlocks []*internal.Block
				if err := json.NewDecoder(resp.Body).Decode(&theirBlocks); err == nil {
					n.Lock()
					if err := n.Chain.ReplaceChain(theirBlocks); err == nil {
						log.Printf("[SYNC] Chain updated from %s\n", peer)
					} else {
						panic(err)
					}
					n.Unlock()
				} else {
					panic(err)
				}
				resp.Body.Close()
			}

			// Sync mempool
			resp, err = http.Get(peer + "/tx/pool")
			if err == nil {
				var theirPool []internal.Transaction
				if err := json.NewDecoder(resp.Body).Decode(&theirPool); err == nil {
					n.Lock()
					existing := make(map[string]bool)
					for _, tx := range n.Pool {
						existing[tx.Hash()] = true
					}
					added := 0
					for _, tx := range theirPool {
						if existing[tx.Hash()] {
							continue
						}
						addr, err := internal.RecoverAddressFromTransaction(tx)
						if err != nil || addr != tx.From {
							log.Printf("[MEMPOOL SYNC] Rejected invalid tx from %s\n", tx.From)
							continue
						}
						balance := n.Chain.GetBalanceWithPending(tx.From, n.Pool)
						if balance < tx.Price+tx.Fee {
							log.Printf("[MEMPOOL SYNC] Rejected tx from %s: insufficient balance\n", tx.From)
							continue
						}
						n.Pool = append(n.Pool, tx)
						added++
					}
					if added > 0 {
						log.Printf("[MEMPOOL SYNC] Added %d txs from %s\n", added, peer)
					}
					n.Unlock()
				}
				resp.Body.Close()
			}

			// Sync peer list
			resp, err = http.Get(peer + "/peers")
			if err == nil {
				var newPeers []string
				if err := json.NewDecoder(resp.Body).Decode(&newPeers); err == nil {
					n.Lock()
					known := make(map[string]bool)
					for _, p := range n.Peers {
						known[p] = true
					}
					for _, p := range newPeers {
						if !known[p] {
							n.Peers = append(n.Peers, p)
							log.Printf("[DISCOVERY] Found new peer: %s\n", p)
						}
					}
					n.Unlock()
				}
				resp.Body.Close()
			}
		}
	}
}

func (n *Node) HandlePeers(w http.ResponseWriter, r *http.Request) {
	n.Lock()
	defer n.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n.Peers)
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
