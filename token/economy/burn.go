package economy

import (
	"math"

	"github.com/shopspring/decimal"
)

// ==================== 燃烧配置 ====================

// BurnConfig 燃烧配置
type BurnConfig struct {
	FeeBurnPercent       float64       // 交易费燃烧百分比
	RewardBurnThreshold  decimal.Decimal // 奖励燃烧阈值
	RewardBurnPercent    float64       // 奖励燃烧百分比
	GovernanceBurnPercent float64      // 治理燃烧百分比
}

func DefaultBurnConfig() *BurnConfig {
	threshold, _ := decimal.NewFromString("10000000000000000000") // 10000 POLE
	return &BurnConfig{
		FeeBurnPercent:       0.25,  // 25%
		RewardBurnThreshold:  threshold,
		RewardBurnPercent:    0.10,  // 10%
		GovernanceBurnPercent: 0.01,  // 1%
	}
}

// ==================== 代币燃烧 ====================

// TokenBurn 代币燃烧
type TokenBurn struct {
	config      *BurnConfig
	totalBurned decimal.Decimal
}

// NewTokenBurn 创建燃烧器
func NewTokenBurn(config *BurnConfig) *TokenBurn {
	if config == nil {
		config = DefaultBurnConfig()
	}
	
	return &TokenBurn{
		config:      config,
		totalBurned: decimal.Zero,
	}
}

// CalculateFeeBurn 计算交易费燃烧量
func (tb *TokenBurn) CalculateFeeBurn(fee decimal.Decimal) decimal.Decimal {
	return fee.Mul(decimal.NewFromFloat(tb.config.FeeBurnPercent))
}

// CalculateRewardBurn 计算奖励燃烧量
func (tb *TokenBurn) CalculateRewardBurn(reward decimal.Decimal) decimal.Decimal {
	if reward.GreaterThan(tb.config.RewardBurnThreshold) {
		excess := reward.Sub(tb.config.RewardBurnThreshold)
		return excess.Mul(decimal.NewFromFloat(tb.config.RewardBurnPercent))
	}
	return decimal.Zero
}

// CalculateGovernanceBurn 计算治理燃烧量
func (tb *TokenBurn) CalculateGovernanceBurn(locked decimal.Decimal) decimal.Decimal {
	return locked.Mul(decimal.NewFromFloat(tb.config.GovernanceBurnPercent))
}

// Burn 燃烧代币
func (tb *TokenBurn) Burn(amount decimal.Decimal) {
	tb.totalBurned = tb.totalBurned.Add(amount)
}

// GetTotalBurned 获取总燃烧量
func (tb *TokenBurn) GetTotalBurned() decimal.Decimal {
	return tb.totalBurned
}

// ProcessFee 处理交易费
func (tb *TokenBurn) ProcessFee(fee decimal.Decimal) (burnAmount, remaining decimal.Decimal) {
	burnAmount = tb.CalculateFeeBurn(fee)
	remaining = fee.Sub(burnAmount)
	tb.Burn(burnAmount)
	return
}

// ProcessReward 处理奖励
func (tb *TokenBurn) ProcessReward(reward decimal.Decimal) (burnAmount, remaining decimal.Decimal) {
	burnAmount = tb.CalculateRewardBurn(reward)
	remaining = reward.Sub(burnAmount)
	tb.Burn(burnAmount)
	return
}

// ==================== 铸造配置 ====================

// MintConfig 铸造配置
type MintConfig struct {
	InitialInflation float64 // 初始通胀率
	DecayFactor     float64 // 衰减因子
	TargetTNGV      float64 // 目标 TNGV
	BlockTime       uint64  // 区块时间
}

func DefaultMintConfig() *MintConfig {
	return &MintConfig{
		InitialInflation: 0.20,
		DecayFactor:     0.5,
		TargetTNGV:      1000000.0,
		BlockTime:       3,
	}
}

// ==================== 代币铸造 ====================

// TokenMinter 代币铸造
type TokenMinter struct {
	config       *MintConfig
	currentYear uint32
}

// NewTokenMinter 创建铸造器
func NewTokenMinter(config *MintConfig) *TokenMinter {
	if config == nil {
		config = DefaultMintConfig()
	}
	
	return &TokenMinter{
		config:       config,
		currentYear: 1,
	}
}

// GetInflationRate 获取通胀率
func (tm *TokenMinter) GetInflationRate() float64 {
	// Annual_Inflation_Rate = Initial_Inflation × (1/2)^(Year / 2)
	exponent := float64(tm.currentYear) / 2.0
	return tm.config.InitialInflation * math.Pow(tm.config.DecayFactor, exponent)
}

// CalculateAnnualReward 计算年奖励
func (tm *TokenMinter) CalculateAnnualReward(totalSupply decimal.Decimal) decimal.Decimal {
	rate := tm.GetInflationRate()
	return totalSupply.Mul(decimal.NewFromFloat(rate))
}

// CalculateBlockReward 计算区块奖励
func (tm *TokenMinter) CalculateBlockReward(totalSupply decimal.Decimal) decimal.Decimal {
	annual := tm.CalculateAnnualReward(totalSupply)
	blocksPerYear := 31536000 / int(tm.config.BlockTime)
	return annual.Div(decimal.NewFromInt(int64(blocksPerYear)))
}

// CalculateEpochReward 计算纪元奖励
func (tm *TokenMinter) CalculateEpochReward(totalSupply decimal.Decimal, blocksPerEpoch uint64) decimal.Decimal {
	blockReward := tm.CalculateBlockReward(totalSupply)
	return blockReward.Mul(decimal.NewFromInt(int64(blocksPerEpoch)))
}

// AdjustForTNGV 根据 TNGV 调整奖励
func (tm *TokenMinter) AdjustForTNGV(baseReward decimal.Decimal, currentTNGV float64) decimal.Decimal {
	ratio := tm.config.TargetTNGV / max(currentTNGV, 1.0)
	adjustment := math.Sqrt(ratio)
	// 限制在 0.5x 到 2x 之间
	if adjustment < 0.5 {
		adjustment = 0.5
	}
	if adjustment > 2.0 {
		adjustment = 2.0
	}
	
	return baseReward.Mul(decimal.NewFromFloat(adjustment))
}

// AdvanceYear 推进年份
func (tm *TokenMinter) AdvanceYear() {
	tm.currentYear++
}

// GetCurrentYear 获取当前年份
func (tm *TokenMinter) GetCurrentYear() uint32 {
	return tm.currentYear
}
