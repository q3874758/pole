package economy

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"pole-core/core/types"
)

// ==================== 经济配置 ====================

// Config 经济配置
type Config struct {
	TotalSupply         decimal.Decimal // 总供应量
	InitialInflation    float64        // 初始通胀率
	DecayFactor         float64        // 衰减因子
	BlockTime           uint64         // 区块时间 (秒)
	BlocksPerYear       uint64         // 每年区块数
	ValidatorRewardShare float64        // 验证者奖励份额
	DelegatorRewardShare float64       // 委托者奖励份额
}

func DefaultConfig() *Config {
	total, _ := decimal.NewFromString("1000000000000000000") // 1 billion * 10^18
	return &Config{
		TotalSupply:          total,
		InitialInflation:    0.20,   // 20%
		DecayFactor:         0.5,
		BlockTime:           3,
		BlocksPerYear:       31536000 / 3, // ~10.5 million
		ValidatorRewardShare: 0.70,
		DelegatorRewardShare: 0.30,
	}
}

// ==================== 供应记录 ====================

// SupplyRecord 供应记录
type SupplyRecord struct {
	Height              types.BlockHeight  `json:"height"`
	TotalSupply        decimal.Decimal   `json:"total_supply"`
	CirculatingSupply  decimal.Decimal   `json:"circulating_supply"`
	InflationRate      float64           `json:"inflation_rate"`
	Timestamp          int64             `json:"timestamp"`
}

// ==================== 代币经济 ====================

// TokenEconomy 代币经济
type TokenEconomy struct {
	config            *Config
	totalSupply       decimal.Decimal
	circulatingSupply decimal.Decimal
	inflationRate     float64
	currentYear       uint32
	balances          map[types.Address]decimal.Decimal
	delegations       map[types.Address]map[types.Address]decimal.Decimal // (delegator, validator) -> amount
	supplyHistory     []SupplyRecord
	mu                sync.RWMutex
}

// NewTokenEconomy 创建代币经济
func NewTokenEconomy(config *Config) *TokenEconomy {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &TokenEconomy{
		config:             config,
		totalSupply:        config.TotalSupply,
		circulatingSupply:  decimal.Zero,
		inflationRate:     config.InitialInflation,
		currentYear:        1,
		balances:           make(map[types.Address]decimal.Decimal),
		delegations:        make(map[types.Address]map[types.Address]decimal.Decimal),
		supplyHistory:      make([]SupplyRecord, 0),
	}
}

// GetInflationRate 获取当前通胀率
func (te *TokenEconomy) GetInflationRate() float64 {
	te.mu.RLock()
	defer te.mu.RUnlock()
	
	return te.inflationRate
}

// CalculateInflation 计算通胀率
func (te *TokenEconomy) CalculateInflation() float64 {
	te.mu.RLock()
	defer te.mu.RUnlock()
	
	// Annual_Inflation_Rate = Initial_Inflation × (1/2)^(Year / 2)
	exponent := float64(te.currentYear) / 2.0
	rate := te.config.InitialInflation * math.Pow(te.config.DecayFactor, exponent)
	
	return rate
}

// CalculateBlockReward 计算区块奖励
func (te *TokenEconomy) CalculateBlockReward() decimal.Decimal {
	te.mu.RLock()
	defer te.mu.RUnlock()
	
	// 年奖励 = 总供应量 * 通胀率
	annualReward := te.totalSupply.Mul(decimal.NewFromFloat(te.inflationRate))
	
	// 区块奖励 = 年奖励 / 年区块数
	blockReward := annualReward.Div(decimal.NewFromInt(int64(te.config.BlocksPerYear)))
	
	return blockReward
}

// MintBlockReward 铸造区块奖励
func (te *TokenEconomy) MintBlockReward(validator types.Address) (
	validatorReward, delegatorReward decimal.Decimal, err error,
) {
	te.mu.Lock()
	defer te.mu.Unlock()
	
	reward := te.CalculateBlockReward()
	
	// 验证者奖励
	validatorReward = reward.Mul(decimal.NewFromFloat(te.config.ValidatorRewardShare))
	delegatorReward = reward.Mul(decimal.NewFromFloat(te.config.DelegatorRewardShare))
	
	// 添加到验证者余额
	te.balances[validator] = te.balances[validator].Add(validatorReward)
	
	// 更新总量
	te.totalSupply = te.totalSupply.Add(reward)
	te.circulatingSupply = te.circulatingSupply.Add(reward)
	
	// 记录历史
	te.recordSupply()
	
	return validatorReward, delegatorReward, nil
}

// Transfer 转账
func (te *TokenEconomy) Transfer(from, to types.Address, amount decimal.Decimal) error {
	te.mu.Lock()
	defer te.mu.Unlock()
	
	fromBalance := te.balances[from]
	if fromBalance.LessThan(amount) {
		return fmt.Errorf("insufficient balance")
	}
	
	te.balances[from] = fromBalance.Sub(amount)
	te.balances[to] = te.balances[to].Add(amount)
	
	return nil
}

// Stake 质押
func (te *TokenEconomy) Stake(delegator, validator types.Address, amount decimal.Decimal) error {
	te.mu.Lock()
	defer te.mu.Unlock()
	
	// 检查余额
	balance := te.balances[delegator]
	if balance.LessThan(amount) {
		return fmt.Errorf("insufficient balance")
	}
	
	// 扣除余额
	te.balances[delegator] = balance.Sub(amount)
	
	// 添加到质押
	if te.delegations[delegator] == nil {
		te.delegations[delegator] = make(map[types.Address]decimal.Decimal)
	}
	te.delegations[delegator][validator] = te.delegations[delegator][validator].Add(amount)
	
	return nil
}

// Unstake 解除质押
func (te *TokenEconomy) Unstake(delegator, validator types.Address, amount decimal.Decimal) error {
	te.mu.Lock()
	defer te.mu.Unlock()
	
	// 检查质押
	delegation := te.delegations[delegator][validator]
	if delegation.LessThan(amount) {
		return fmt.Errorf("insufficient delegation")
	}
	
	// 减少质押
	te.delegations[delegator][validator] = delegation.Sub(amount)
	
	// 返还余额
	te.balances[delegator] = te.balances[delegator].Add(amount)
	
	return nil
}

// GetBalance 获取余额
func (te *TokenEconomy) GetBalance(address types.Address) decimal.Decimal {
	te.mu.RLock()
	defer te.mu.RUnlock()
	
	return te.balances[address]
}

// GetTotalStaked 获取总质押
func (te *TokenEconomy) GetTotalStaked() decimal.Decimal {
	te.mu.RLock()
	defer te.mu.RUnlock()
	
	var total decimal.Decimal
	for _, delegatorMap := range te.delegations {
		for _, amount := range delegatorMap {
			total = total.Add(amount)
		}
	}
	
	return total
}

// recordSupply 记录供应变化
func (te *TokenEconomy) recordSupply() {
	record := SupplyRecord{
		Height:             0,
		TotalSupply:       te.totalSupply,
		CirculatingSupply:  te.circulatingSupply,
		InflationRate:      te.inflationRate,
		Timestamp:          time.Now().Unix(),
	}
	
	te.supplyHistory = append(te.supplyHistory, record)
}

// GetSupplyHistory 获取供应历史
func (te *TokenEconomy) GetSupplyHistory() []SupplyRecord {
	te.mu.RLock()
	defer te.mu.RUnlock()
	
	result := make([]SupplyRecord, len(te.supplyHistory))
	copy(result, te.supplyHistory)
	return result
}

// AdvanceYear 推进年份
func (te *TokenEconomy) AdvanceYear() {
	te.mu.Lock()
	defer te.mu.Unlock()
	
	te.currentYear++
	te.inflationRate = te.CalculateInflation()
}

// GetSupplyInfo 获取供应信息
func (te *TokenEconomy) GetSupplyInfo() SupplyInfo {
	te.mu.RLock()
	defer te.mu.RUnlock()
	
	return SupplyInfo{
		TotalSupply:      te.totalSupply,
		CirculatingSupply: te.circulatingSupply,
		StakedSupply:     te.GetTotalStaked(),
		InflationRate:    te.inflationRate,
	}
}

// SupplyInfo 供应信息
type SupplyInfo struct {
	TotalSupply       decimal.Decimal `json:"total_supply"`
	CirculatingSupply decimal.Decimal `json:"circulating_supply"`
	StakedSupply     decimal.Decimal `json:"staked_supply"`
	InflationRate    float64         `json:"inflation_rate"`
}
