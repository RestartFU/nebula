package internal

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	btcecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"golang.org/x/crypto/ripemd160"
)

func RecoverAddressFromTransaction(tx Transaction) (string, error) {
	if tx.Signature == "" {
		return "", errors.New("missing signature")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(tx.Signature)
	if err != nil {
		return "", err
	}

	txCopy := tx
	txCopy.Signature = ""

	b, err := json.Marshal(txCopy)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(b)

	pubKey, _, err := btcecdsa.RecoverCompact(sigBytes, hash[:])
	if err != nil {
		return "", err
	}

	pubBytes := pubKey.SerializeCompressed()

	// HASH160(pubkey) for address derivation
	shaHash := sha256.Sum256(pubBytes)
	ripemdHasher := ripemd160.New()
	ripemdHasher.Write(shaHash[:])
	pubKeyHash := ripemdHasher.Sum(nil)

	return hex.EncodeToString(pubKeyHash), nil
}
