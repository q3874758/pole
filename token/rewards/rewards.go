package rewards

import (
	"fmt"
	"sync"

	"github.com/shopspring/decimal"
	"pole-core/core/types"
)

// ==================== 奖励配置 ====================

// Config 奖励配置
type Config struct {
	BaseReward          decimal.Decimal // 基础奖励
	Tier1Weight         float64         // Tier1 权重
	Tier2Weight         float64         // Tier2 权重
	Tier3Weight         float64         // Tier3 权重
	QualityMultiplier   float64         // 质量乘数
	ExplorationBonus    float64         // 探索奖励乘数
}

func DefaultConfig() *Config {
	base, _ := decimal.NewFromString("1000000000000000") // 0.001 POLE
	return &Config{
		BaseReward:        base,
		Tier1Weight:       1.0,
		Tier2Weight:       0.45,
		Tier3Weight:       0.10,
		QualityMultiplier: 1.5,
		ExplorationBonus:  2.0,
	}
}

// ==================== 节点分数 ====================

// NodeScore 节点分数
type NodeScore struct {
	NodeID            types.NodeID `json:"node_id"`
	DataPoints        uint64       `json:"data_points"`
	Quality           float64      `json:"quality"`
	Uptime            float64      `json:"uptime"`
	GamesCovered      uint32       `json:"games_covered"`
	ExplorationBonus  decimal.Decimal `json:"exploration_bonus"`
}

// ==================== 奖励分发器 ====================

// RewardDistributor 奖励分发器
type RewardDistributor struct {
	config        *Config
	nodeScores    map[types.NodeID]*NodeScore
	pendingRewards map[types.Address]decimal.Decimal
	mu            sync.RWMutex
}

// NewRewardDistributor 创建奖励分发器
func NewRewardDistributor(config *Config) *RewardDistributor {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &RewardDistributor{
		config:         config,
		nodeScores:     make(map[types.NodeID]*NodeScore),
		pendingRewards: make(map[types.Address]decimal.Decimal),
	}
}

// CalculateDataReward 计算数据贡献奖励
func (rd *RewardDistributor) CalculateDataReward(
	tier types.Tier,
	qualityFactor float64,
	isExploration bool,
) decimal.Decimal {
	// 获取层级权重
	var tierWeight float64
	switch tier {
	case types.Tier1:
		tierWeight = rd.config.Tier1Weight
	case types.Tier2:
		tierWeight = rd.config.Tier2Weight
	case types.Tier3:
		tierWeight = rd.config.Tier3Weight
	default:
		tierWeight = 0.1
	}
	
	// 基础计算
	reward := rd.config.BaseReward.Mul(decimal.NewFromFloat(tierWeight))
	
	// 质量乘数
	qualityMult := 1.0 + (qualityFactor * (rd.config.QualityMultiplier - 1.0))
	reward = reward.Mul(decimal.NewFromFloat(qualityMult))
	
	// 探索奖励
	if isExploration {
		reward = reward.Mul(decimal.NewFromFloat(rd.config.ExplorationBonus))
	}
	
	return reward
}

// UpdateNodeScore 更新节点分数
func (rd *RewardDistributor) UpdateNodeScore(
	nodeID types.NodeID,
	dataPointsDelta uint64,
	quality float64,
) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	
	score, ok := rd.nodeScores[nodeID]
	if !ok {
		score = &NodeScore{
			NodeID:     nodeID,
			Quality:    0.5,
			Uptime:    1.0,
			ExplorationBonus: decimal.Zero,
		}
		rd.nodeScores[nodeID] = score
	}
	
	score.DataPoints += dataPointsDelta
	score.Quality = (score.Quality + quality) / 2.0
}

// CalculateEpochReward 计算纪元奖励
func (rd *RewardDistributor) CalculateEpochReward(
	nodeID *types.NodeID,
	epochReward decimal.Decimal,
) (decimal.Decimal, error) {
	rd.mu.RLock()
	defer rd.mu.RUnlock()
	
	score, ok := rd.nodeScores[*nodeID]
	if !ok {
		return decimal.Zero, fmt.Errorf("node not found")
	}
	
	// 计算总分
	var totalScore float64
	for _, s := range rd.nodeScores {
		totalScore += float64(s.DataPoints) * s.Quality
	}
	
	if totalScore == 0 {
		return decimal.Zero, nil
	}
	
	// 节点份额
	nodeContribution := float64(score.DataPoints) * score.Quality
	share := nodeContribution / totalScore
	
	return epochReward.Mul(decimal.NewFromFloat(share)), nil
}

// DistributeRewards 分发奖励
func (rd *RewardDistributor) DistributeRewards(
	rewards map[types.Address]decimal.Decimal,
) error {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	
	for address, amount := range rewards {
		rd.pendingRewards[address] = rd.pendingRewards[address].Add(amount)
	}
	
	return nil
}

// ClaimRewards 领取奖励
func (rd *RewardDistributor) ClaimRewards(address types.Address) decimal.Decimal {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	
	amount := rd.pendingRewards[address]
	rd.pendingRewards[address] = decimal.Zero
	return amount
}

// GetPendingRewards 获取待领取奖励
func (rd *RewardDistributor) GetPendingRewards(address types.Address) decimal.Decimal {
	rd.mu.RLock()
	defer rd.mu.RUnlock()
	
	return rd.pendingRewards[address]
}

// GetNodeScore 获取节点分数
func (rd *RewardDistributor) GetNodeScore(nodeID types.NodeID) (*NodeScore, bool) {
	rd.mu.RLock()
	defer rd.mu.RUnlock()
	
	score, ok := rd.nodeScores[nodeID]
	return score, ok
}

// ==================== 验证者奖励计算 ====================

// CalculateValidatorReward 计算验证者奖励
func CalculateValidatorReward(
	totalStake decimal.Decimal,
	validatorStake decimal.Decimal,
	blockReward decimal.Decimal,
) decimal.Decimal {
	if totalStake.IsZero() || validatorStake.IsZero() {
		return decimal.Zero
	}
	
	share := validatorStake.Div(totalStake)
	return blockReward.Mul(share)
}

// CalculateDelegatorReward 计算委托者奖励
func CalculateDelegatorReward(
	delegatorStake decimal.Decimal,
	validatorTotalStake decimal.Decimal,
	validatorReward decimal.Decimal,
	commission uint8,
) decimal.Decimal {
	if validatorTotalStake.IsZero() || delegatorStake.IsZero() {
		return decimal.Zero
	}
	
	// 验证者份额
	commissionFactor := 1.0 - (float64(commission) / 100.0)
	
	// 委托者份额 (扣除佣金后)
	delegatorShare := validatorReward.
		Mul(decimal.NewFromFloat(commissionFactor)).
		Mul(delegatorStake.Div(validatorTotalStake))
	
	return delegatorShare
}
