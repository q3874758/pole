package wallet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"

	"pole-core/core/types"
)

// Wallet 钱包
type Wallet struct {
	accounts map[string]*Account // 地址 -> 账户
	mu       sync.RWMutex
}

// Account 账户
type Account struct {
	Address    string         `json:"address"`
	PublicKey  string         `json:"public_key"`
	PrivateKey string         `json:"private_key,omitempty"` // 仅在导入时保留
	Key        *ecdsa.PrivateKey `json:"-"`
}

// NewWallet 创建新钱包
func NewWallet() *Wallet {
	return &Wallet{
		accounts: make(map[string]*Account),
	}
}

// GenerateKey 生成新密钥对
func (w *Wallet) GenerateKey() (*Account, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	addr := keyToAddress(&key.PublicKey)
	privKeyHex := hex.EncodeToString(key.D.Bytes())
	acc := &Account{
		Address:    addr,
		PublicKey:  hex.EncodeToString(keyToPubBytes(&key.PublicKey)),
		PrivateKey: privKeyHex,
		Key:        key,
	}

	w.mu.Lock()
	w.accounts[addr] = acc
	w.mu.Unlock()

	return acc, nil
}

// ImportKey 导入已有私钥
func (w *Wallet) ImportKey(privateKeyHex string) (*Account, error) {
	keyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}

	key := new(ecdsa.PrivateKey)
	key.D = new(big.Int).SetBytes(keyBytes)
	key.PublicKey.Curve = elliptic.P256()
	key.PublicKey.X, key.PublicKey.Y = key.PublicKey.Curve.ScalarBaseMult(keyBytes)

	if !key.PublicKey.Curve.IsOnCurve(key.PublicKey.X, key.PublicKey.Y) {
		return nil, fmt.Errorf("invalid private key")
	}

	addr := keyToAddress(&key.PublicKey)
	acc := &Account{
		Address:    addr,
		PublicKey:  hex.EncodeToString(keyToPubBytes(&key.PublicKey)),
		PrivateKey: privateKeyHex,
		Key:        key,
	}

	w.mu.Lock()
	w.accounts[addr] = acc
	w.mu.Unlock()

	return acc, nil
}

// GetAccount 获取账户
func (w *Wallet) GetAccount(address string) (*Account, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	acc, ok := w.accounts[address]
	if !ok {
		return nil, false
	}
	// 返回副本避免外部修改
	return &Account{
		Address:    acc.Address,
		PublicKey:  acc.PublicKey,
		PrivateKey: acc.PrivateKey,
		Key:        acc.Key,
	}, true
}

// ListAccounts 列出所有账户
func (w *Wallet) ListAccounts() []*Account {
	w.mu.RLock()
	defer w.mu.RUnlock()

	accounts := make([]*Account, 0, len(w.accounts))
	for _, acc := range w.accounts {
		accounts = append(accounts, &Account{
			Address:   acc.Address,
			PublicKey: acc.PublicKey,
		})
	}
	return accounts
}

// Sign 签名数据
func (w *Wallet) Sign(address string, data []byte) ([]byte, error) {
	acc, ok := w.GetAccount(address)
	if !ok {
		return nil, fmt.Errorf("account not found: %s", address)
	}
	if acc.Key == nil {
		return nil, fmt.Errorf("private key not available")
	}

	r, s, err := ecdsa.Sign(rand.Reader, acc.Key, data)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	sig := make([]byte, 0, 64)
	sig = append(sig, r.Bytes()...)
	sig = append(sig, s.Bytes()...)
	return sig, nil
}

// SignTx 签名交易
func (w *Wallet) SignTx(tx *types.Transaction, address string) error {
	signBytes := tx.SignBytes()
	sig, err := w.Sign(address, signBytes)
	if err != nil {
		return err
	}
	tx.Signature = sig
	return nil
}

// Verify 验证签名
func (w *Wallet) Verify(address string, data, signature []byte) error {
	acc, ok := w.GetAccount(address)
	if !ok {
		return fmt.Errorf("account not found")
	}

	if acc.Key == nil {
		return fmt.Errorf("public key not available")
	}

	pubKey := &acc.Key.PublicKey
	if len(signature) < 64 {
		return fmt.Errorf("invalid signature length")
	}

	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])

	if !ecdsa.Verify(pubKey, data, r, s) {
		return fmt.Errorf("verification failed")
	}

	return nil
}

// ==================== 钱包存储 ====================

// WalletData 钱包数据文件
type WalletData struct {
	Version   int         `json:"version"`
	Accounts []*Account  `json:"accounts"`
}

// Save 保存钱包到文件
func (w *Wallet) Save(path string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	data := &WalletData{
		Version:   1,
		Accounts:  make([]*Account, 0, len(w.accounts)),
	}

	for _, acc := range w.accounts {
		data.Accounts = append(data.Accounts, &Account{
			Address:    acc.Address,
			PublicKey:  acc.PublicKey,
			PrivateKey: acc.PrivateKey,
		})
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	return os.WriteFile(path, jsonData, 0600)
}

// Export 导出钱包为 JSON 字节（用于备份，含私钥，务必妥善保管）
func (w *Wallet) Export() ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	data := &WalletData{
		Version:  1,
		Accounts: make([]*Account, 0, len(w.accounts)),
	}
	for _, acc := range w.accounts {
		data.Accounts = append(data.Accounts, &Account{
			Address:    acc.Address,
			PublicKey:  acc.PublicKey,
			PrivateKey: acc.PrivateKey,
		})
	}
	return json.MarshalIndent(data, "", "  ")
}

// Load 从文件加载钱包
func (w *Wallet) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var wd WalletData
	if err := json.Unmarshal(data, &wd); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.accounts = make(map[string]*Account)
	for _, acc := range wd.Accounts {
		// 重建私钥对象
		var key *ecdsa.PrivateKey
		if acc.PrivateKey != "" {
			keyBytes, err := hex.DecodeString(acc.PrivateKey)
			if err == nil {
				key = new(ecdsa.PrivateKey)
				key.D = new(big.Int).SetBytes(keyBytes)
				key.PublicKey.Curve = elliptic.P256()
				key.PublicKey.X, key.PublicKey.Y = key.PublicKey.Curve.ScalarBaseMult(keyBytes)
			}
		}
		acc.Key = key
		w.accounts[acc.Address] = acc
	}

	return nil
}

// ==================== 工具函数 ====================

func keyToAddress(pubKey *ecdsa.PublicKey) string {
	pubBytes := keyToPubBytes(pubKey)
	hash := sha256.Sum256(pubBytes)
	return hex.EncodeToString(hash[:20]) // 取前 20 字节作为地址
}

func keyToPubBytes(pubKey *ecdsa.PublicKey) []byte {
	return elliptic.Marshal(elliptic.P256(), pubKey.X, pubKey.Y)
}

// NewAccountFromAddress 从地址创建账户（仅公钥模式）
func NewAccountFromAddress(address string) *Account {
	return &Account{
		Address: address,
	}
}
