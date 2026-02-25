package contracts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// pow10 10^n
func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// ==================== 代币合约配置 ====================

// TokenConfig 代币配置
type TokenConfig struct {
	Name            string
	Symbol         string
	Decimals       uint8
	TotalSupply    *big.Int
	Mintable       bool
	Burnable       bool
	Transferable   bool
}

// DefaultTokenConfig 默认代币配置
func DefaultTokenConfig() *TokenConfig {
	return &TokenConfig{
		Name:         "Proof of Live Engagement",
		Symbol:       "POLE",
		Decimals:     18,
		TotalSupply:  new(big.Int).Mul(big.NewInt(1_000_000_000), pow10(18)), // 10亿 * 10^18
		Mintable:     true,
		Burnable:     true,
		Transferable: true,
	}
}

// ==================== 账户 ====================

// Account 账户
type Account struct {
	Address   string
	Balance   *big.Int
	Nonce     uint64
	Allowance map[string]*big.Int // spender -> amount
	Locked   bool
	LockInfo string
}

// NewAccount 创建账户
func NewAccount(address string) *Account {
	return &Account{
		Address:   address,
		Balance:   big.NewInt(0),
		Nonce:     0,
		Allowance: make(map[string]*big.Int),
		Locked:    false,
	}
}

// ==================== 代币合约 ====================

// TokenContract 代币合约
type TokenContract struct {
	config   *TokenConfig `json:"-"`
	accounts map[string]*Account
	totalSupply *big.Int `json:"-"`
	circulatingSupply *big.Int
	minter map[string]bool `json:"-"`
	paused bool
	pauser string
	mu     sync.RWMutex `json:"-"`
}

// MarshalJSON 自定义序列化（跳过 config, totalSupply, minter, mu）
func (tc *TokenContract) MarshalJSON() ([]byte, error) {
	type Alias TokenContract
	return json.Marshal(struct {
		Accounts          map[string]*Account `json:"accounts"`
		CirculatingSupply string              `json:"circulating_supply"`
		Paused            bool                `json:"paused"`
		Pauser            string              `json:"pauser"`
		Alias
	}{
		Accounts:          tc.accounts,
		CirculatingSupply: tc.circulatingSupply.String(),
		Paused:            tc.paused,
		Pauser:            tc.pauser,
		Alias:             Alias(*tc),
	})
}

// UnmarshalJSON 自定义反序列化
func (tc *TokenContract) UnmarshalJSON(data []byte) error {
	type Alias TokenContract
	aux := struct {
		Accounts          map[string]*Account `json:"accounts"`
		CirculatingSupply string              `json:"circulating_supply"`
		Paused            bool                `json:"paused"`
		Pauser            string              `json:"pauser"`
		*Alias
	}{
		Alias: (*Alias)(tc),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	tc.accounts = aux.Accounts
	tc.circulatingSupply = new(big.Int)
	tc.circulatingSupply.SetString(aux.CirculatingSupply, 10)
	tc.paused = aux.Paused
	tc.pauser = aux.Pauser
	tc.config = DefaultTokenConfig()
	tc.totalSupply = tc.config.TotalSupply
	tc.minter = make(map[string]bool)
	return nil
}

// NewTokenContract 创建代币合约
func NewTokenContract(config *TokenConfig) *TokenContract {
	if config == nil {
		config = DefaultTokenConfig()
	}

	return &TokenContract{
		config:            config,
		accounts:         make(map[string]*Account),
		totalSupply:      config.TotalSupply,
		circulatingSupply: big.NewInt(0),
		minter:           make(map[string]bool),
		paused:           false,
	}
}

// ==================== 基本功能 ====================

// TotalSupply 总供应量
func (tc *TokenContract) TotalSupply() *big.Int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return new(big.Int).Set(tc.totalSupply)
}

// BalanceOf 查询余额
func (tc *TokenContract) BalanceOf(owner string) *big.Int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if acc, ok := tc.accounts[owner]; ok {
		return new(big.Int).Set(acc.Balance)
	}
	return big.NewInt(0)
}

// Transfer 转账
func (tc *TokenContract) Transfer(ctx context.Context, from, to string, amount *big.Int) error {
	if amount.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 检查暂停状态
	if tc.paused {
		return fmt.Errorf("token paused")
	}

	// 检查发送方余额
	fromAcc, ok := tc.accounts[from]
	if !ok {
		return fmt.Errorf("from account not found")
	}

	if fromAcc.Balance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance")
	}

	// 获取或创建接收方账户
	toAcc, ok := tc.accounts[to]
	if !ok {
		toAcc = NewAccount(to)
		tc.accounts[to] = toAcc
	}

	// 执行转账
	fromAcc.Balance = new(big.Int).Sub(fromAcc.Balance, amount)
	toAcc.Balance = new(big.Int).Add(toAcc.Balance, amount)
	fromAcc.Nonce++

	return nil
}

// ==================== 授权功能 ====================

// Approve 授权
func (tc *TokenContract) Approve(owner, spender string, amount *big.Int) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	ownerAcc, ok := tc.accounts[owner]
	if !ok {
		ownerAcc = NewAccount(owner)
		tc.accounts[owner] = ownerAcc
	}

	ownerAcc.Allowance[spender] = new(big.Int).Set(amount)
	return nil
}

// Allowance 查询授权额度
func (tc *TokenContract) Allowance(owner, spender string) *big.Int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if acc, ok := tc.accounts[owner]; ok {
		if amount, ok := acc.Allowance[spender]; ok {
			return new(big.Int).Set(amount)
		}
	}
	return big.NewInt(0)
}

// TransferFrom 授权转账
func (tc *TokenContract) TransferFrom(ctx context.Context, spender, from, to string, amount *big.Int) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.paused {
		return fmt.Errorf("token paused")
	}

	// 检查授权额度
	allowed := tc.Allowance(from, spender)
	if allowed.Cmp(amount) < 0 {
		return fmt.Errorf("allowance exceeded")
	}

	// 检查余额
	fromAcc, ok := tc.accounts[from]
	if !ok || fromAcc.Balance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance")
	}

	// 获取或创建接收方账户
	toAcc, ok := tc.accounts[to]
	if !ok {
		toAcc = NewAccount(to)
		tc.accounts[to] = toAcc
	}

	// 执行转账
	fromAcc.Balance = new(big.Int).Sub(fromAcc.Balance, amount)
	toAcc.Balance = new(big.Int).Add(toAcc.Balance, amount)

	// 扣减授权额度
	fromAcc.Allowance[spender] = new(big.Int).Sub(allowed, amount)
	fromAcc.Nonce++

	return nil
}

// ==================== 铸造功能 ====================

// Mint 铸造
func (tc *TokenContract) Mint(ctx context.Context, to string, amount *big.Int) error {
	if !tc.config.Mintable {
		return fmt.Errorf("token not mintable")
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 检查铸造者权限
	if !tc.minter[to] && to != "admin" {
		return fmt.Errorf("not authorized to mint")
	}

	// 更新总量
	tc.totalSupply = new(big.Int).Add(tc.totalSupply, amount)
	tc.circulatingSupply = new(big.Int).Add(tc.circulatingSupply, amount)

	// 添加到接收方账户
	acc, ok := tc.accounts[to]
	if !ok {
		acc = NewAccount(to)
		tc.accounts[to] = acc
	}
	acc.Balance = new(big.Int).Add(acc.Balance, amount)

	return nil
}

// Burn 燃烧
func (tc *TokenContract) Burn(from string, amount *big.Int) error {
	if !tc.config.Burnable {
		return fmt.Errorf("token not burnable")
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 检查余额
	acc, ok := tc.accounts[from]
	if !ok || acc.Balance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance")
	}

	// 燃烧代币
	acc.Balance = new(big.Int).Sub(acc.Balance, amount)
	tc.circulatingSupply = new(big.Int).Sub(tc.circulatingSupply, amount)
	tc.totalSupply = new(big.Int).Sub(tc.totalSupply, amount)

	return nil
}

// ==================== 权限管理 ====================

// AddMinter 添加铸造权限
func (tc *TokenContract) AddMinter(minter string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.minter[minter] = true
	return nil
}

// RemoveMinter 移除铸造权限
func (tc *TokenContract) RemoveMinter(minter string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	delete(tc.minter, minter)
	return nil
}

// Pause 暂停
func (tc *TokenContract) Pause(pauser string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.paused = true
	tc.pauser = pauser
	return nil
}

// Unpause 解除暂停
func (tc *TokenContract) Unpause() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.paused = false
	return nil
}

// IsPaused 检查暂停状态
func (tc *TokenContract) IsPaused() bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.paused
}

// ==================== 锁定功能 ====================

// Lock 锁定账户
func (tc *TokenContract) Lock(address, reason string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	acc, ok := tc.accounts[address]
	if !ok {
		return fmt.Errorf("account not found")
	}

	acc.Locked = true
	acc.LockInfo = reason

	return nil
}

// Unlock 解锁账户
func (tc *TokenContract) Unlock(address string) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	acc, ok := tc.accounts[address]
	if !ok {
		return fmt.Errorf("account not found")
	}

	acc.Locked = false
	acc.LockInfo = ""

	return nil
}

// IsLocked 检查锁定状态
func (tc *TokenContract) IsLocked(address string) (bool, string) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if acc, ok := tc.accounts[address]; ok {
		return acc.Locked, acc.LockInfo
	}
	return false, ""
}

// ==================== 快照功能 ====================

// TokenSnapshot 代币快照
type TokenSnapshot struct {
	BlockHeight uint64
	Timestamp   int64
	TotalSupply *big.Int
	Balances   map[string]*big.Int
}

// CreateSnapshot 创建快照
func (tc *TokenContract) CreateSnapshot(height uint64) *TokenSnapshot {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	snapshot := &TokenSnapshot{
		BlockHeight: height,
		Timestamp:   time.Now().Unix(),
		TotalSupply: new(big.Int).Set(tc.totalSupply),
		Balances:   make(map[string]*big.Int),
	}

	for addr, acc := range tc.accounts {
		snapshot.Balances[addr] = new(big.Int).Set(acc.Balance)
	}

	return snapshot
}

// RestoreSnapshot 恢复快照
func (tc *TokenContract) RestoreSnapshot(snapshot *TokenSnapshot) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.totalSupply = new(big.Int).Set(snapshot.TotalSupply)
	tc.accounts = make(map[string]*Account)

	for addr, balance := range snapshot.Balances {
		acc := NewAccount(addr)
		acc.Balance = new(big.Int).Set(balance)
		tc.accounts[addr] = acc
	}

	return nil
}

// ==================== 事件 ====================

// TransferEvent 转账事件
type TransferEvent struct {
	From    string
	To      string
	Value   *big.Int
	TxHash  string
	Block   uint64
	Index   uint64
}

// ApprovalEvent 授权事件
type ApprovalEvent struct {
	Owner   string
	Spender string
	Value   *big.Int
	TxHash  string
	Block   uint64
}

// ==================== 工具函数 ====================

// Hash 生成交易哈希
func (tc *TokenContract) Hash(from, to string, amount *big.Int, nonce uint64) string {
	data := fmt.Sprintf("%s:%s:%s:%d", from, to, amount.String(), nonce)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GetStats 获取代币统计
func (tc *TokenContract) GetStats() TokenStats {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	return TokenStats{
		Name:              tc.config.Name,
		Symbol:            tc.config.Symbol,
		Decimals:          tc.config.Decimals,
		TotalSupply:       new(big.Int).Set(tc.totalSupply),
		CirculatingSupply:  new(big.Int).Set(tc.circulatingSupply),
		AccountCount:      len(tc.accounts),
		Paused:            tc.paused,
	}
}

// TokenStats 代币统计
type TokenStats struct {
	Name             string
	Symbol           string
	Decimals         uint8
	TotalSupply      *big.Int
	CirculatingSupply *big.Int
	AccountCount     int
	Paused           bool
}

// ==================== 兼容 ERC-20 ====================

// Name 代币名称
func (tc *TokenContract) Name() string {
	return tc.config.Name
}

// Symbol 代币符号
func (tc *TokenContract) Symbol() string {
	return tc.config.Symbol
}

// Decimals 精度
func (tc *TokenContract) Decimals() uint8 {
	return tc.config.Decimals
}

// ==================== Genesis 初始化 ====================

// InitializeGenesis 初始化创世账户
func (tc *TokenContract) InitializeGenesis(allocations map[string]*big.Int) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	for addr, balance := range allocations {
		if balance.Cmp(big.NewInt(0)) <= 0 {
			continue
		}

		acc := NewAccount(addr)
		acc.Balance = new(big.Int).Set(balance)
		tc.accounts[addr] = acc

		tc.circulatingSupply = new(big.Int).Add(tc.circulatingSupply, balance)
	}

	// 验证总量不超过配置
	if tc.circulatingSupply.Cmp(tc.config.TotalSupply) > 0 {
		return fmt.Errorf("initial supply exceeds total supply")
	}

	return nil
}

// GetCirculatingSupply 流通量
func (tc *TokenContract) GetCirculatingSupply() *big.Int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return new(big.Int).Set(tc.circulatingSupply)
}
