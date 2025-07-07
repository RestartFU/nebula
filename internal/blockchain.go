package internal

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"log"
)

type Block struct {
	Index        int           `json:"index"`
	Timestamp    int64         `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
	PrevHash     string        `json:"prev_hash"`
	Nonce        int           `json:"nonce"`
	Hash         string        `json:"hash"`
}

type Blockchain struct {
	Blocks []*Block
	db     *leveldb.DB
}

func NewBlockchain(rewardAddress string, dbPath string) (*Blockchain, error) {
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return nil, err
	}

	bc := &Blockchain{
		db: db,
	}

	err = bc.loadBlocksFromDB()
	if err != nil {
		genesis := &Block{
			Index:     0,
			Timestamp: -22082082,
			Transactions: []Transaction{
				{
					Type:  TxTransfer,
					From:  "nebula",
					To:    rewardAddress,
					Price: 1000,
					Fee:   0,
				},
			},
			PrevHash: "",
			Nonce:    0,
		}
		genesis.Hash = genesis.CalculateHash()
		bc.Blocks = []*Block{genesis}
		if err := bc.saveBlockToDB(genesis); err != nil {
			log.Printf("[ERROR] Failed to save block to DB: %v\n", err)
			return nil, err
		}
	}

	return bc, nil
}

func (bc *Blockchain) loadBlocksFromDB() error {
	iter := bc.db.NewIterator(nil, nil)
	defer iter.Release()

	var blocks []*Block
	for iter.Next() {
		data := iter.Value()
		var block Block
		if err := json.Unmarshal(data, &block); err != nil {
			return err
		}
		blocks = append(blocks, &block)
	}

	if len(blocks) == 0 {
		return errors.New("no blocks found in DB")
	}

	bc.Blocks = blocks
	return nil
}

func (bc *Blockchain) saveBlockToDB(block *Block) error {
	data, err := json.Marshal(block)
	if err != nil {
		return err
	}
	key := []byte(fmt.Sprintf("block-%09d", block.Index))
	err = bc.db.Put(key, data, nil)
	fmt.Println(bc.db.Has(key, nil))
	return err
}

func (bc *Blockchain) ReplaceChain(newBlocks []*Block) error {
	if len(newBlocks) <= len(bc.Blocks) {
		return errors.New("received chain is not longer")
	}

	// Validate each block in sequence
	for i := range newBlocks {
		if i == 0 {
			// Genesis block: allow mismatch
			continue
		}
		if newBlocks[i].PrevHash != newBlocks[i-1].Hash {
			return fmt.Errorf("invalid chain at block %d: hash mismatch", i)
		}
		if err := bc.ValidateBlock(newBlocks[i]); err != nil {
			return fmt.Errorf("block %d invalid: %v", i, err)
		}
	}

	// Clear LevelDB and replace with new chain
	iter := bc.db.NewIterator(nil, nil)
	for iter.Next() {
		key := iter.Key()
		if err := bc.db.Delete(key, nil); err != nil {
			return err
		}
	}
	iter.Release()

	// Save new chain to DB
	for _, block := range newBlocks {
		if err := bc.saveBlockToDB(block); err != nil {
			return err
		}
	}

	bc.Blocks = newBlocks
	return nil
}

func (b *Block) CalculateHash() string {
	data, _ := json.Marshal(struct {
		Index        int
		Timestamp    int64
		Nonce        int
		PrevHash     string
		Transactions []Transaction
	}{
		Index:        b.Index,
		Timestamp:    b.Timestamp,
		Nonce:        b.Nonce,
		PrevHash:     b.PrevHash,
		Transactions: b.Transactions,
	})
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])
}

func (bc *Blockchain) AddBlock(newBlock *Block) error {
	if err := bc.ValidateBlock(newBlock); err != nil {
		return err
	}

	bc.Blocks = append(bc.Blocks, newBlock)
	if err := bc.saveBlockToDB(newBlock); err != nil {
		return err
	}

	return nil
}

// ValidateBlock checks:
// - Previous hash matches latest block
// - Hash matches difficulty
// - Balances sufficient
// - And signatures valid on all non-reward transactions
func (bc *Blockchain) ValidateBlock(block *Block) error {
	hash := block.CalculateHash()
	if hash != block.Hash {
		return errors.New("block hash mismatch")
	}

	if hash[:4] != "0000" {
		return errors.New("block does not meet difficulty")
	}

	// Prepare balances snapshot
	balanceCache := make(map[string]float64)
	for _, b := range bc.Blocks {
		for _, tx := range b.Transactions {
			balanceCache[tx.From] -= tx.Price + tx.Fee
			balanceCache[tx.To] += tx.Price
		}
	}

	// Validate transactions
	for _, tx := range block.Transactions {
		// Reward tx: From "nebula" can skip signature check
		if tx.Type == TxTransfer && tx.From == "nebula" {
			continue
		}

		// Check signature
		addr, err := RecoverAddressFromTransaction(tx)
		if err != nil {
			return fmt.Errorf("signature invalid on tx from %s: %w", tx.From, err)
		}
		if addr != tx.From {
			return fmt.Errorf("signature does not match sender address %s", tx.From)
		}

		// Check balance
		if balanceCache[tx.From] < tx.Price+tx.Fee {
			return fmt.Errorf("insufficient funds for %s", tx.From)
		}

		balanceCache[tx.From] -= tx.Price + tx.Fee
		balanceCache[tx.To] += tx.Price
	}

	return nil
}

func (bc *Blockchain) GetBalance(addr string) float64 {
	var balance float64 = 0
	for _, block := range bc.Blocks {
		for _, tx := range block.Transactions {
			if tx.From == addr {
				balance -= tx.Price + tx.Fee
			}
			if tx.To == addr {
				balance += tx.Price
			}
		}
	}
	return balance
}

func (bc *Blockchain) GetBalanceWithPending(addr string, pending []Transaction) float64 {
	balance := bc.GetBalance(addr)
	for _, tx := range pending {
		if tx.From == addr {
			balance -= tx.Price + tx.Fee
		}
		if tx.To == addr {
			balance += tx.Price
		}
	}
	return balance
}

func (bc *Blockchain) GetLatestBlock() *Block {
	return bc.Blocks[len(bc.Blocks)-1]
}

func (bc *Blockchain) Close() error {
	return bc.db.Close()
}
