package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"nebula/internal"
)

const (
	NodeURL             = "http://localhost:8080"
	MempoolEndpoint     = NodeURL + "/tx/pool"
	BlocksEndpoint      = NodeURL + "/blocks"
	SubmitBlockEndpoint = NodeURL + "/block"
	BlockReward         = float64(100)
	MaxFeePerTx         = float64(1000)
	MaxPendingPerUser   = 5
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: miner <reward_address>")
	}
	rewardAddress := os.Args[1]

	for {
		time.Sleep(10 * time.Second)

		mempool, err := fetchTransactions(MempoolEndpoint)
		if err != nil {
			log.Printf("[MINER] Failed to fetch mempool: %v", err)
			continue
		}

		blocks, err := fetchBlocks(BlocksEndpoint)
		if err != nil {
			log.Printf("[MINER] Failed to fetch blocks: %v", err)
			continue
		}
		if len(blocks) == 0 {
			log.Printf("[MINER] No blocks found, waiting...")
			continue
		}

		chainTip := blocks[len(blocks)-1]

		block, validTxs, err := buildBlock(chainTip, mempool, rewardAddress)
		if err != nil {
			log.Printf("[MINER] Failed to build block: %v", err)
			continue
		}
		if block.Index == 0 {
			log.Printf("[MINER] No valid transactions to mine, waiting...")
			continue
		}

		err = submitBlock(block)
		if err != nil {
			log.Printf("[MINER] Failed to submit block: %v", err)
			continue
		}

		log.Printf("[MINED] Block #%d with %d txs (reward: %.10f)\n", block.Index, len(validTxs), BlockReward)
	}
}

func fetchTransactions(url string) ([]internal.Transaction, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var txs []internal.Transaction
	err = json.NewDecoder(resp.Body).Decode(&txs)
	return txs, err
}

func fetchBlocks(url string) ([]internal.Block, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var blocks []internal.Block
	err = json.NewDecoder(resp.Body).Decode(&blocks)
	return blocks, err
}

func buildBlock(tip internal.Block, mempool []internal.Transaction, rewardAddress string) (internal.Block, []internal.Transaction, error) {
	var validTxs []internal.Transaction
	totalFees := float64(0)
	pendingCount := map[string]int{}

	for _, tx := range mempool {
		fmt.Printf("%#v\n", tx)
		addr, err := internal.RecoverAddressFromTransaction(tx)
		if err != nil || addr != tx.From {
			continue
		}

		if tx.Fee > MaxFeePerTx {
			continue
		}

		pendingCount[tx.From]++
		if pendingCount[tx.From] > MaxPendingPerUser {
			continue
		}

		// Miner can't fully verify balance without chain history,
		// so assume balance is large enough or trust node validation.

		validTxs = append(validTxs, tx)
		totalFees += tx.Fee
	}

	if len(validTxs) == 0 {
		return internal.Block{}, nil, nil
	}

	rewardTx := internal.Transaction{
		Type:  internal.TxTransfer,
		From:  "nebula",
		To:    rewardAddress,
		Price: BlockReward + totalFees,
		Fee:   0,
	}

	newBlock := internal.Block{
		Index:        tip.Index + 1,
		Timestamp:    time.Now().Unix(),
		Transactions: append([]internal.Transaction{rewardTx}, validTxs...),
		PrevHash:     tip.Hash,
		Nonce:        0,
	}

	for {
		hash := newBlock.CalculateHash()
		if hash[:4] == "0000" {
			newBlock.Hash = hash
			break
		}
		newBlock.Nonce++
	}

	return newBlock, validTxs, nil
}

func submitBlock(block internal.Block) error {
	data, err := json.Marshal(block)
	if err != nil {
		return err
	}

	fmt.Printf("%#v\n", block)
	resp, err := http.Post(SubmitBlockEndpoint, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("submit block failed: %s", string(body))
	}
	return nil
}
