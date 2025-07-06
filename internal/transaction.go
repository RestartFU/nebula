package internal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

const (
	TxRegister = "REGISTER"
	TxTransfer = "TRANSFER"
	TxSetIP    = "SET_IP"
	TxSell     = "SELL"
	TxBuy      = "BUY"
)

type Transaction struct {
	Type      string            `json:"type"`
	From      string            `json:"from"`
	To        string            `json:"to"`
	Name      string            `json:"name"`
	Price     float64           `json:"price"`
	Fee       float64           `json:"fee"`
	Payload   map[string]string `json:"payload"`
	Signature string            `json:"signature"`
}

func (tx *Transaction) Hash() string {
	txCopy := *tx
	txCopy.Signature = ""
	b, err := json.Marshal(txCopy)
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:])
}
