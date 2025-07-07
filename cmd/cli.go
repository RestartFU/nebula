package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"nebula/internal"
)

const nodeURL = "http://localhost:8081"

func main() {
	wallet, err := internal.LoadWalletFromFile("wallet.key")
	if err != nil {
		fmt.Println("Failed to load wallet.key:", err)
		return
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Nebula CLI Wallet")
	fmt.Println("-----------------")

	for {
		fmt.Println("\nCommands: balance | send | register | buy | sell | history | exit")
		fmt.Print("> ")
		cmd, _ := reader.ReadString('\n')
		cmd = strings.TrimSpace(cmd)

		switch cmd {
		case "balance":
			balance(wallet.Address)
		case "send":
			send(wallet, reader)
		case "register":
			registerDomain(wallet, reader)
		case "buy":
			buyDomain(wallet, reader)
		case "sell":
			sellDomain(wallet, reader)
		case "history":
			showHistory(wallet.Address)
		case "exit":
			fmt.Println("Bye!")
			return
		default:
			fmt.Println("Unknown command")
		}
	}
}

func balance(addr string) {
	resp, err := http.Get(nodeURL + "/balance?address=" + addr)
	if err != nil {
		fmt.Println("Error fetching balance:", err)
		return
	}
	defer resp.Body.Close()

	var res struct {
		Balance float64 `json:"balance"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Println("Error decoding response:", err)
		return
	}

	fmt.Printf("Balance for %s: %.8f\n", addr, res.Balance)
}

func send(wallet *internal.Wallet, reader *bufio.Reader) {
	fmt.Print("Send to address: ")
	to, _ := reader.ReadString('\n')
	to = strings.TrimSpace(to)

	fmt.Print("Amount to send: ")
	amtStr, _ := reader.ReadString('\n')
	amtStr = strings.TrimSpace(amtStr)
	amt, err := strconv.ParseFloat(amtStr, 64)
	if err != nil {
		fmt.Println("Invalid amount")
		return
	}

	fmt.Print("Fee: ")
	feeStr, _ := reader.ReadString('\n')
	feeStr = strings.TrimSpace(feeStr)
	fee, err := strconv.ParseFloat(feeStr, 64)
	if err != nil {
		fmt.Println("Invalid fee")
		return
	}

	tx := internal.Transaction{
		Type:  internal.TxTransfer,
		From:  wallet.Address,
		To:    to,
		Price: amt,
		Fee:   fee,
	}

	if err := internal.SignTransaction(&tx, wallet.PrivateKey); err != nil {
		fmt.Println("Failed to sign tx:", err)
		return
	}

	sendTx(tx)
}

func registerDomain(wallet *internal.Wallet, reader *bufio.Reader) {
	fmt.Print("Domain name to register: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	fmt.Print("Price: ")
	priceStr, _ := reader.ReadString('\n')
	priceStr = strings.TrimSpace(priceStr)
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		fmt.Println("Invalid price")
		return
	}

	tx := internal.Transaction{
		Type:  internal.TxRegister,
		From:  wallet.Address,
		To:    "nebula",
		Name:  name,
		Price: price,
		Fee:   1,
	}

	if err := internal.SignTransaction(&tx, wallet.PrivateKey); err != nil {
		fmt.Println("Failed to sign tx:", err)
		return
	}

	sendTx(tx)
}

func buyDomain(wallet *internal.Wallet, reader *bufio.Reader) {
	fmt.Print("Domain name to buy: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	fmt.Print("Price: ")
	priceStr, _ := reader.ReadString('\n')
	priceStr = strings.TrimSpace(priceStr)
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		fmt.Println("Invalid price")
		return
	}

	tx := internal.Transaction{
		Type:  internal.TxBuy,
		From:  wallet.Address,
		Name:  name,
		Price: price,
		Fee:   1,
	}

	if err := internal.SignTransaction(&tx, wallet.PrivateKey); err != nil {
		fmt.Println("Failed to sign tx:", err)
		return
	}

	sendTx(tx)
}

func sellDomain(wallet *internal.Wallet, reader *bufio.Reader) {
	fmt.Print("Domain name to sell: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	fmt.Print("Price: ")
	priceStr, _ := reader.ReadString('\n')
	priceStr = strings.TrimSpace(priceStr)
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		fmt.Println("Invalid price")
		return
	}

	tx := internal.Transaction{
		Type:  internal.TxSell,
		From:  wallet.Address,
		Name:  name,
		Price: price,
		Fee:   1,
	}

	if err := internal.SignTransaction(&tx, wallet.PrivateKey); err != nil {
		fmt.Println("Failed to sign tx:", err)
		return
	}

	sendTx(tx)
}

func sendTx(tx internal.Transaction) {
	data, err := json.Marshal(tx)
	if err != nil {
		fmt.Println("Failed to marshal tx:", err)
		return
	}

	resp, err := http.Post(nodeURL+"/tx", "application/json", bytes.NewBuffer(data))
	if err != nil {
		fmt.Println("Failed to send tx:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Println("Tx rejected by node")
		return
	}

	fmt.Println("Transaction sent successfully")
}

func showHistory(addr string) {
	resp, err := http.Get(nodeURL + "/blocks")
	if err != nil {
		fmt.Println("Error fetching blocks:", err)
		return
	}
	defer resp.Body.Close()

	var blocks []internal.Block
	if err := json.NewDecoder(resp.Body).Decode(&blocks); err != nil {
		fmt.Println("Error decoding blocks:", err)
		return
	}

	fmt.Printf("Transaction history for %s:\n", addr)
	for _, block := range blocks {
		for _, tx := range block.Transactions {
			if tx.From == addr || tx.To == addr {
				fmt.Printf("Block %d | Type: %s | From: %s | To: %s | Amount: %.8f | Name: %s\n",
					block.Index, tx.Type, tx.From, tx.To, tx.Price, tx.Name)
			}
		}
	}
}
