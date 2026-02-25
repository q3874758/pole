package economy

import (
	"fmt"
	"sync"

	"github.com/shopspring/decimal"
)

// ==================== 手续费分配配置 ====================

// FeeDistributionConfig 手续费分配配置
type FeeDistributionConfig struct {
	RewardPoolPercent decimal.Decimal // 奖励池份额 (0.5 = 50%)
	BurnPercent       decimal.Decimal // 燃烧份额 (0.25 = 25%)
	TreasuryPercent   decimal.Decimal // 国库份额 (0.25 = 25%)
}

func DefaultFeeDistributionConfig() *FeeDistributionConfig {
	return &FeeDistributionConfig{
		RewardPoolPercent: decimal.NewFromFloat(0.50), // 50%
		BurnPercent:       decimal.NewFromFloat(0.25), // 25%
		TreasuryPercent:   decimal.NewFromFloat(0.25), // 25%
	}
}

// ==================== 手续费分配器 ====================

// FeeDistributor 手续费分配器
type FeeDistributor struct {
	config    *FeeDistributionConfig
	burner    *TokenBurn
	treasury  TreasuryManager
	rewards   map[string]decimal.Decimal // validator -> pending rewards
	mu        sync.RWMutex
}

// TreasuryManager 国库管理器接口
type TreasuryManager interface {
	Deposit(from string, amount decimal.Decimal) error
	GetBalance() decimal.Decimal
}

// NewFeeDistributor 创建手续费分配器
func NewFeeDistributor(config *FeeDistributionConfig, burner *TokenBurn, treasury TreasuryManager) *FeeDistributor {
	if config == nil {
		config = DefaultFeeDistributionConfig()
	}
	return &FeeDistributor{
		config:   config,
		burner:   burner,
		treasury: treasury,
		rewards:  make(map[string]decimal.Decimal),
	}
}

// DistributeFee 分配手续费
// 返回: validatorReward, burnedAmount, treasuryAmount
func (fd *FeeDistributor) DistributeFee(fee decimal.Decimal) (validatorReward, burnedAmount, treasuryAmount decimal.Decimal, err error) {
	if fee.IsZero() || fee.IsNegative() {
		return decimal.Zero, decimal.Zero, decimal.Zero, nil
	}

	fd.mu.Lock()
	defer fd.mu.Unlock()

	// 计算各部分金额
	validatorReward = fee.Mul(fd.config.RewardPoolPercent)
	burnedAmount = fee.Mul(fd.config.BurnPercent)
	treasuryAmount = fee.Mul(fd.config.TreasuryPercent)

	// 燃烧
	fd.burner.Burn(burnedAmount)

	// 存入国库
	if fd.treasury != nil {
		if err := fd.treasury.Deposit("fee_distribution", treasuryAmount); err != nil {
			// 国库存款失败，归入奖励池
			validatorReward = validatorReward.Add(treasuryAmount)
			treasuryAmount = decimal.Zero
		}
	} else {
		// 无国库时归入奖励池
		validatorReward = validatorReward.Add(treasuryAmount)
		treasuryAmount = decimal.Zero
	}

	return validatorReward, burnedAmount, treasuryAmount, nil
}

// AddValidatorReward 添加验证者奖励
func (fd *FeeDistributor) AddValidatorReward(validator string, amount decimal.Decimal) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	fd.rewards[validator] = fd.rewards[validator].Add(amount)
}

// ClaimValidatorReward 领取验证者奖励
func (fd *FeeDistributor) ClaimValidatorReward(validator string) decimal.Decimal {
	fd.mu.Lock()
	defer fd.mu.Unlock()

	reward := fd.rewards[validator]
	fd.rewards[validator] = decimal.Zero
	return reward
}

// GetPendingReward 获取待领取奖励
func (fd *FeeDistributor) GetPendingReward(validator string) decimal.Decimal {
	fd.mu.RLock()
	defer fd.mu.RUnlock()
	return fd.rewards[validator]
}

// GetConfig 获取配置
func (fd *FeeDistributor) GetConfig() *FeeDistributionConfig {
	return fd.config
}

// SetConfig 设置配置 (通过治理)
func (fd *FeeDistributor) SetConfig(config *FeeDistributionConfig) error {
	// 验证总和为 1
	total := config.RewardPoolPercent.Add(config.BurnPercent).Add(config.TreasuryPercent)
	if !total.Equal(decimal.NewFromFloat(1.0)) {
		return ErrInvalidDistribution
	}
	fd.config = config
	return nil
}

var ErrInvalidDistribution = fmt.Errorf("distribution percentages must sum to 1")
