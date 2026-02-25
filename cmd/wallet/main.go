package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/shopspring/decimal"
	"pole-core/core/types"
	"pole-core/wallet"
)

var (
	walletPath = flag.String("wallet", "wallet.json", "wallet file path")
	action     = flag.String("action", "", "action: create, import, list, balance, sign")
	privKey    = flag.String("privkey", "", "private key (hex) for import")
	address    = flag.String("address", "", "address for balance/query")
	toAddress  = flag.String("to", "", "receiver address for transfer")
	amount     = flag.String("amount", "0", "amount to transfer")
	fee        = flag.String("fee", "0.001", "fee for transaction")
)

func main() {
	flag.Parse()

	if *action == "" {
		fmt.Println("Usage: wallet [options]")
		flag.Usage()
		fmt.Println("\nActions:")
		fmt.Println("  create   - create new account")
		fmt.Println("  import   - import account from private key")
		fmt.Println("  list     - list all accounts")
		fmt.Println("  balance  - query account balance")
		fmt.Println("  sign     - sign transaction")
		os.Exit(1)
	}

	w := wallet.NewWallet()

	// 加载钱包
	if _, err := os.Stat(*walletPath); err == nil {
		if err := w.Load(*walletPath); err != nil {
			fmt.Printf("Error loading wallet: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Loaded wallet from %s\n", *walletPath)
	}

	switch *action {
	case "create":
		handleCreate(w)
	case "import":
		handleImport(w)
	case "list":
		handleList(w)
	case "balance":
		handleBalance(w)
	case "sign":
		handleSign(w)
	default:
		fmt.Printf("Unknown action: %s\n", *action)
		os.Exit(1)
	}

	// 保存钱包
	if err := w.Save(*walletPath); err != nil {
		fmt.Printf("Error saving wallet: %v\n", err)
		os.Exit(1)
	}
}

func handleCreate(w *wallet.Wallet) {
	acc, err := w.GenerateKey()
	if err != nil {
		fmt.Printf("Error generating key: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Account created successfully!")
	fmt.Printf("Address:    %s\n", acc.Address)
	fmt.Printf("PublicKey: %s\n", acc.PublicKey)
	fmt.Println("\nIMPORTANT: Save your wallet file to keep your private key!")
}

func handleImport(w *wallet.Wallet) {
	if *privKey == "" {
		fmt.Println("Error: --privkey required for import")
		os.Exit(1)
	}

	acc, err := w.ImportKey(*privKey)
	if err != nil {
		fmt.Printf("Error importing key: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Account imported successfully!")
	fmt.Printf("Address:    %s\n", acc.Address)
	fmt.Printf("PublicKey:  %s\n", acc.PublicKey)
}

func handleList(w *wallet.Wallet) {
	accounts := w.ListAccounts()
	if len(accounts) == 0 {
		fmt.Println("No accounts in wallet")
		return
	}
	fmt.Println("Accounts:")
	for i, acc := range accounts {
		fmt.Printf("%d. %s\n", i+1, acc.Address)
	}
}

func handleBalance(w *wallet.Wallet) {
	if *address == "" {
		// 使用第一个账户
		accounts := w.ListAccounts()
		if len(accounts) == 0 {
			fmt.Println("No accounts in wallet")
			os.Exit(1)
		}
		*address = accounts[0].Address
	}
	fmt.Printf("Address: %s\n", *address)
	fmt.Println("Balance: (query from node required)")
	fmt.Printf("To query balance, use RPC: curl http://localhost:9090/account/balance?address=%s\n", *address)
}

func handleSign(w *wallet.Wallet) {
	accounts := w.ListAccounts()
	if len(accounts) == 0 {
		fmt.Println("No accounts in wallet")
		os.Exit(1)
	}

	fromAddr := *address
	if fromAddr == "" {
		fromAddr = accounts[0].Address
		fmt.Printf("Using default address: %s\n", fromAddr)
	}

	// 检查账户是否存在
	if _, ok := w.GetAccount(fromAddr); !ok {
		fmt.Printf("Error: address %s not found in wallet\n", fromAddr)
		os.Exit(1)
	}

	// 解析金额
	amt, err := decimal.NewFromString(*amount)
	if err != nil {
		fmt.Printf("Error parsing amount: %v\n", err)
		os.Exit(1)
	}
	feeAmt, _ := decimal.NewFromString(*fee)

	// 创建交易
	from := types.FromPublicKey([]byte(fromAddr))
	to := types.FromPublicKey([]byte(*toAddress))

	tx := &types.Transaction{
		Type: types.TxTransfer,
		From: from,
		Nonce: 0,
		Fee:   types.TokenAmount(feeAmt.Mul(decimal.New(1, 18))),
		Transfer: &types.TransferTx{
			From:   from,
			To:     to,
			Amount: types.TokenAmount(amt.Mul(decimal.New(1, 18))),
			Fee:    types.TokenAmount(feeAmt.Mul(decimal.New(1, 18))),
		},
	}

	// 签名
	if err := w.SignTx(tx, fromAddr); err != nil {
		fmt.Printf("Error signing transaction: %v\n", err)
		os.Exit(1)
	}

	// 输出交易 JSON
	txJSON, _ := json.MarshalIndent(tx, "", "  ")
	fmt.Println("Signed transaction:")
	fmt.Println(string(txJSON))

	// 输出交易哈希
	blockID := types.NewBlockID(&types.Block{Transactions: []types.Transaction{*tx}})
	txHash := hex.EncodeToString(blockID[:])
	fmt.Printf("\nTransaction hash: %s\n", txHash)
	fmt.Println("\nTo broadcast, use RPC: curl -X POST http://localhost:9090/tx/broadcast -d @tx.json")
}
