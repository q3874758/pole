package economy

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"pole-core/core/types"
)

// ==================== Slash 配置 ====================

// SlashConfig Slash 配置
type SlashConfig struct {
	DoubleSignSlashPercent   float64       // 双签削减百分比
	UnavailableSlashPercent  float64       // 不可用削减百分比
	MaliciousSlashPercent    float64       // 恶意行为削减百分比
	SlashJailPeriod          int64         // 监禁期 (秒)
	MinStakeForValidator     decimal.Decimal // 验证者最小质押
	MaxSlashPercent          float64       // 最大削减百分比
}

func DefaultSlashConfig() *SlashConfig {
	minStake, _ := decimal.NewFromString("10000000000000000000") // 10000 POLE
	return &SlashConfig{
		DoubleSignSlashPercent:  0.05, // 5%
		UnavailableSlashPercent: 0.01, // 1%
		MaliciousSlashPercent:   0.10, // 10%
		SlashJailPeriod:          2 * 24 * 3600, // 2 天
		MinStakeForValidator:    minStake,
		MaxSlashPercent:         0.30, // 30%
	}
}

// ==================== Slash 记录 ====================

// SlashRecord Slash 记录
type SlashRecord struct {
	Validator    types.Address   `json:"validator"`
	Reason       SlashReason     `json:"reason"`
	Amount       decimal.Decimal `json:"amount"`
	Height       types.BlockHeight `json:"height"`
	Timestamp    int64           `json:"timestamp"`
	JailUntil    int64           `json:"jail_until"`
	RewardsLost  decimal.Decimal `json:"rewards_lost"`
}

// SlashReason Slash 原因
type SlashReason int

const (
	SlashReasonDoubleSign SlashReason = iota
	SlashReasonUnavailable
	SlashReasonMalicious
	SlashReasonDataManipulation
	SlashReasonLowUptime
)

func (sr SlashReason) String() string {
	switch sr {
	case SlashReasonDoubleSign:
		return "DoubleSign"
	case SlashReasonUnavailable:
		return "Unavailable"
	case SlashReasonMalicious:
		return "Malicious"
	case SlashReasonDataManipulation:
		return "DataManipulation"
	case SlashReasonLowUptime:
		return "LowUptime"
	default:
		return "Unknown"
	}
}

// ==================== Slash 管理器 ====================

// SlashManager Slash 管理器
type SlashManager struct {
	config       *SlashConfig
	slashRecords map[types.Address][]SlashRecord
	jailed       map[types.Address]int64 // 解禁时间
	mu           sync.RWMutex
}

// NewSlashManager 创建 Slash 管理器
func NewSlashManager(config *SlashConfig) *SlashManager {
	if config == nil {
		config = DefaultSlashConfig()
	}

	return &SlashManager{
		config:       config,
		slashRecords: make(map[types.Address][]SlashRecord),
		jailed:       make(map[types.Address]int64),
	}
}

// Slash 削减验证者
func (sm *SlashManager) Slash(
	validator types.Address,
	reason SlashReason,
	amount decimal.Decimal,
	height types.BlockHeight,
) (*SlashRecord, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 确定削减百分比
	slashPercent := sm.getSlashPercent(reason)
	
	// 计算实际削减量
	slashAmount := amount.Mul(decimal.NewFromFloat(slashPercent))
	
	// 限制最大削减
	maxSlash := amount.Mul(decimal.NewFromFloat(sm.config.MaxSlashPercent))
	if slashAmount.GreaterThan(maxSlash) {
		slashAmount = maxSlash
	}

	now := time.Now().Unix()

	record := &SlashRecord{
		Validator:   validator,
		Reason:     reason,
		Amount:     slashAmount,
		Height:     height,
		Timestamp:  now,
		JailUntil:  now + sm.config.SlashJailPeriod,
		RewardsLost: decimal.Zero,
	}

	// 保存记录
	sm.slashRecords[validator] = append(sm.slashRecords[validator], *record)
	
	// 监禁验证者
	sm.jailed[validator] = record.JailUntil

	return record, nil
}

// getSlashPercent 获取削减百分比
func (sm *SlashManager) getSlashPercent(reason SlashReason) float64 {
	switch reason {
	case SlashReasonDoubleSign:
		return sm.config.DoubleSignSlashPercent
	case SlashReasonUnavailable:
		return sm.config.UnavailableSlashPercent
	case SlashReasonMalicious:
		return sm.config.MaliciousSlashPercent
	case SlashReasonDataManipulation:
		return sm.config.MaliciousSlashPercent
	case SlashReasonLowUptime:
		return sm.config.UnavailableSlashPercent
	default:
		return sm.config.UnavailableSlashPercent
	}
}

// IsJailed 检查是否被监禁
func (sm *SlashManager) IsJailed(validator types.Address) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if jailUntil, ok := sm.jailed[validator]; ok {
		return time.Now().Unix() < jailUntil
	}
	return false
}

// GetJailReleaseTime 获取释放时间
func (sm *SlashManager) GetJailReleaseTime(validator types.Address) (int64, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	releaseTime, ok := sm.jailed[validator]
	return releaseTime, ok
}

// Unjail 解除监禁
func (sm *SlashManager) Unjail(validator types.Address) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.IsJailed(validator) {
		return fmt.Errorf("validator not jailed")
	}

	delete(sm.jailed, validator)
	return nil
}

// GetSlashRecords 获取 Slash 记录
func (sm *SlashManager) GetSlashRecords(validator types.Address) []SlashRecord {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	records := sm.slashRecords[validator]
	result := make([]SlashRecord, len(records))
	copy(result, records)
	return result
}

// GetSlashCount 获取削减次数
func (sm *SlashManager) GetSlashCount(validator types.Address) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.slashRecords[validator])
}

// GetTotalSlashed 获取总削减量
func (sm *SlashManager) GetTotalSlashed(validator types.Address) decimal.Decimal {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var total decimal.Decimal
	for _, record := range sm.slashRecords[validator] {
		total = total.Add(record.Amount)
	}
	return total
}

// ==================== 奖励分发器 ====================

// RewardDistributor 奖励分发器
type RewardDistributor struct {
	config    *Config
	burner    *TokenBurn
	slashMgr  *SlashManager
	pendingRewards map[types.Address]decimal.Decimal
	mu        sync.RWMutex
}

// NewRewardDistributor 创建奖励分发器
func NewRewardDistributor(config *Config, burner *TokenBurn, slashMgr *SlashManager) *RewardDistributor {
	if config == nil {
		config = DefaultConfig()
	}
	if burner == nil {
		burner = NewTokenBurn(nil)
	}
	if slashMgr == nil {
		slashMgr = NewSlashManager(nil)
	}

	return &RewardDistributor{
		config:         config,
		burner:         burner,
		slashMgr:       slashMgr,
		pendingRewards: make(map[types.Address]decimal.Decimal),
	}
}

// DistributeBlockReward 分发区块奖励
func (rd *RewardDistributor) DistributeBlockReward(
	validator types.Address,
	validatorStake decimal.Decimal,
	delegators map[types.Address]decimal.Decimal,
) (validatorReward, delegatorsReward, burnedReward decimal.Decimal, err error) {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	// 检查验证者是否被监禁
	if rd.slashMgr.IsJailed(validator) {
		return decimal.Zero, decimal.Zero, decimal.Zero, fmt.Errorf("validator is jailed")
	}

	// 计算区块奖励
	blockReward := rd.calculateBlockReward()

	// 计算燃烧量
	burnedReward = rd.burner.CalculateRewardBurn(blockReward)

	// 计算验证者奖励
	validatorShare := blockReward.Mul(decimal.NewFromFloat(rd.config.ValidatorRewardShare))
	validatorReward = validatorShare.Sub(burnedReward)

	// 计算委托者奖励
	delegatorsReward = blockReward.Mul(decimal.NewFromFloat(rd.config.DelegatorRewardShare))

	// 分发奖励
	rd.pendingRewards[validator] = rd.pendingRewards[validator].Add(validatorReward)
	
	for delegator, stakeRatio := range delegators {
		delegatorReward := delegatorsReward.Mul(stakeRatio)
		rd.pendingRewards[delegator] = rd.pendingRewards[delegator].Add(delegatorReward)
	}

	// 更新总量
	rd.burner.Burn(burnedReward)

	return validatorReward, delegatorsReward, burnedReward, nil
}

// calculateBlockReward 计算区块奖励
func (rd *RewardDistributor) calculateBlockReward() decimal.Decimal {
	// 年通胀率
	inflationRate := rd.config.InitialInflation
	
	// 年奖励
	annualReward := rd.config.TotalSupply.Mul(decimal.NewFromFloat(inflationRate))
	
	// 区块奖励
	blocksPerYear := 31536000 / int(rd.config.BlockTime)
	blockReward := annualReward.Div(decimal.NewFromInt(int64(blocksPerYear)))
	
	return blockReward
}

// DistributeFee 分发交易费
func (rd *RewardDistributor) DistributeFee(
	validator types.Address,
	fee decimal.Decimal,
) (validatorFee, burnedFee decimal.Decimal, err error) {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	// 检查验证者是否被监禁
	if rd.slashMgr.IsJailed(validator) {
		return decimal.Zero, decimal.Zero, fmt.Errorf("validator is jailed")
	}

	// 计算燃烧量
	burnedFee = rd.burner.CalculateFeeBurn(fee)
	
	// 验证者获得剩余费用
	validatorFee = fee.Sub(burnedFee)
	
	// 更新奖励
	rd.pendingRewards[validator] = rd.pendingRewards[validator].Add(validatorFee)
	
	// 燃烧
	rd.burner.Burn(burnedFee)

	return validatorFee, burnedFee, nil
}

// SlashAndDistribute 削减并分发
func (rd *RewardDistributor) SlashAndDistribute(
	validator types.Address,
	reason SlashReason,
	height types.BlockHeight,
	validatorStake decimal.Decimal,
) (slashAmount decimal.Decimal, err error) {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	// 执行削减
	slashAmount = validatorStake.Mul(decimal.NewFromFloat(rd.slashMgr.getSlashPercent(reason)))
	
	// 如果验证者有待分发奖励，扣除
	if pending, ok := rd.pendingRewards[validator]; ok {
		if pending.GreaterThanOrEqual(slashAmount) {
			rd.pendingRewards[validator] = pending.Sub(slashAmount)
		} else {
			slashAmount = pending
			rd.pendingRewards[validator] = decimal.Zero
		}
	}

	// 燃烧削减的代币
	rd.burner.Burn(slashAmount)

	// 记录 Slash
	rd.slashMgr.Slash(validator, reason, slashAmount, height)

	return slashAmount, nil
}

// ClaimRewards 领取奖励
func (rd *RewardDistributor) ClaimRewards(address types.Address) (reward decimal.Decimal) {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	reward = rd.pendingRewards[address]
	rd.pendingRewards[address] = decimal.Zero

	return reward
}

// GetPendingRewards 获取待领取奖励
func (rd *RewardDistributor) GetPendingRewards(address types.Address) decimal.Decimal {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	return rd.pendingRewards[address]
}

// GetTotalBurned 获取总燃烧量
func (rd *RewardDistributor) GetTotalBurned() decimal.Decimal {
	return rd.burner.GetTotalBurned()
}

// ==================== 完整经济模块 ====================

// Economy 完整经济模块
type Economy struct {
	*TokenEconomy
	*TokenBurn
	*RewardDistributor
	slashMgr *SlashManager
}

// NewEconomy 创建完整经济模块
func NewEconomy() *Economy {
	config := DefaultConfig()
	burnConfig := DefaultBurnConfig()
	slashConfig := DefaultSlashConfig()

	tokenEconomy := NewTokenEconomy(config)
	tokenBurn := NewTokenBurn(burnConfig)
	slashMgr := NewSlashManager(slashConfig)
	rewardDistributor := NewRewardDistributor(config, tokenBurn, slashMgr)

	return &Economy{
		TokenEconomy:      tokenEconomy,
		TokenBurn:         tokenBurn,
		RewardDistributor: rewardDistributor,
		slashMgr:          slashMgr,
	}
}

// ProcessTransactionFee 处理交易费
func (e *Economy) ProcessTransactionFee(from types.Address, fee decimal.Decimal) error {
	burnedFee, remainingFee := e.TokenBurn.ProcessFee(fee)
	
	_ = remainingFee
	_ = burnedFee

	return nil
}

// ProcessBlockRewards 处理区块奖励
func (e *Economy) ProcessBlockRewards(
	validator types.Address,
	delegators map[types.Address]decimal.Decimal,
) (validatorReward, delegatorsReward, burned decimal.Decimal, err error) {
	stake := e.TokenEconomy.GetTotalStaked()
	
	return e.RewardDistributor.DistributeBlockReward(
		validator,
		stake,
		delegators,
	)
}

// SlashValidator 削减验证者
func (e *Economy) SlashValidator(
	validator types.Address,
	reason SlashReason,
	height types.BlockHeight,
) (decimal.Decimal, error) {
	stake := e.TokenEconomy.GetBalance(validator)
	
	return e.RewardDistributor.SlashAndDistribute(
		validator,
		reason,
		height,
		stake,
	)
}

// GetEconomyStats 获取经济统计
func (e *Economy) GetEconomyStats() EconomyStats {
	return EconomyStats{
		TotalSupply:      e.TokenEconomy.totalSupply,
		CirculatingSupply: e.TokenEconomy.circulatingSupply,
		TotalStaked:     e.TokenEconomy.GetTotalStaked(),
		InflationRate:   e.TokenEconomy.GetInflationRate(),
		TotalBurned:     e.TokenBurn.GetTotalBurned(),
		ActiveValidators: 0, // 需要从共识模块获取
	}
}

// EconomyStats 经济统计
type EconomyStats struct {
	TotalSupply        decimal.Decimal `json:"total_supply"`
	CirculatingSupply  decimal.Decimal `json:"circulating_supply"`
	TotalStaked       decimal.Decimal `json:"total_staked"`
	InflationRate     float64        `json:"inflation_rate"`
	TotalBurned       decimal.Decimal `json:"total_burned"`
	ActiveValidators  int            `json:"active_validators"`
}
