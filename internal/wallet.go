package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/ripemd160"

	"encoding/base64"
	"github.com/btcsuite/btcd/btcec/v2"
	btcecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

type Wallet struct {
	PrivateKey *btcec.PrivateKey
	Address    string
}

// NewWallet creates a new secp256k1 key and derives address
func NewWallet() (*Wallet, error) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}
	addr := PubKeyToAddress(priv.PubKey())
	return &Wallet{PrivateKey: priv, Address: addr}, nil
}

// PubKeyToAddress derives the address as HASH160(pubkey compressed)
func PubKeyToAddress(pub *btcec.PublicKey) string {
	pubBytes := pub.SerializeCompressed()
	shaHash := sha256.Sum256(pubBytes)
	ripemdHasher := ripemd160.New()
	ripemdHasher.Write(shaHash[:])
	pubKeyHash := ripemdHasher.Sum(nil) // 20 bytes
	return hex.EncodeToString(pubKeyHash)
}

// SaveWallet saves the raw private key bytes as hex to a file (0600)
func SaveWallet(w *Wallet, filename string) error {
	privBytes := w.PrivateKey.Serialize() // 32 bytes
	hexPriv := hex.EncodeToString(privBytes)
	return ioutil.WriteFile(filename, []byte(hexPriv), 0600)
}

// LoadWalletFromFile loads a wallet from a hex-encoded private key file
func LoadWalletFromFile(filename string) (*Wallet, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	privBytes, err := hex.DecodeString(string(data))
	if err != nil {
		return nil, err
	}
	privKey, _ := btcec.PrivKeyFromBytes(privBytes)
	addr := PubKeyToAddress(privKey.PubKey())
	return &Wallet{PrivateKey: privKey, Address: addr}, nil
}

// SignTransaction signs the tx hash using compact recoverable signature
func SignTransaction(tx *Transaction, priv *btcec.PrivateKey) error {
	txCopy := *tx
	txCopy.Signature = ""

	b, err := json.Marshal(txCopy)
	if err != nil {
		return err
	}

	hash := sha256.Sum256(b)
	sig := btcecdsa.SignCompact(priv, hash[:], true)
	tx.Signature = base64.StdEncoding.EncodeToString(sig)
	return nil
}
