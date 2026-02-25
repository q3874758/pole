package engine

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== Tier 分层系统 ====================

// TierSystem Tier 分层系统
type TierSystem struct {
	tierConfigs map[types.Tier]*TierConfig
	mu          sync.RWMutex
}

// TierConfig Tier 配置
type TierConfig struct {
	Tier              types.Tier
	Name             string
	MinPlayers        uint64         // 最小在线玩家
	MaxPlayers        uint64         // 最大在线玩家
	Weight            float64        // GVS 权重
	RewardMultiplier  float64        // 奖励乘数
	VerificationLevel VerificationLevel // 验证级别
	APIFeatures       []string       // 支持的 API 特性
}

// VerificationLevel 验证级别
type VerificationLevel int

const (
	VerificationLevelHigh VerificationLevel = iota
	VerificationLevelMedium
	VerificationLevelLow
)

// NewTierSystem 创建分层系统
func NewTierSystem() *TierSystem {
	return &TierSystem{
		tierConfigs: map[types.Tier]*TierConfig{
			types.Tier1: {
				Tier:             types.Tier1,
				Name:             "核心验证层",
				MinPlayers:       1000,
				MaxPlayers:       100000000,
				Weight:           1.0,
				RewardMultiplier: 1.0,
				VerificationLevel: VerificationLevelHigh,
				APIFeatures:      []string{"realtime", "historical", "peak", "detailed"},
			},
			types.Tier2: {
				Tier:             types.Tier2,
				Name:             "增强验证层",
				MinPlayers:       100,
				MaxPlayers:       1000,
				Weight:           0.45,
				RewardMultiplier: 0.6,
				VerificationLevel: VerificationLevelMedium,
				APIFeatures:      []string{"realtime", "daily"},
			},
			types.Tier3: {
				Tier:             types.Tier3,
				Name:             "社区验证层",
				MinPlayers:       0,
				MaxPlayers:       100,
				Weight:           0.10,
				RewardMultiplier: 0.3,
				VerificationLevel: VerificationLevelLow,
				APIFeatures:      []string{"community"},
			},
		},
	}
}

// DetermineTier 确定游戏所属 Tier
func (ts *TierSystem) DetermineTier(onlinePlayers uint64) types.Tier {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	for tier, config := range ts.tierConfigs {
		if onlinePlayers >= config.MinPlayers && onlinePlayers < config.MaxPlayers {
			return tier
		}
	}

	// 默认 Tier3
	return types.Tier3
}

// GetTierConfig 获取 Tier 配置
func (ts *TierSystem) GetTierConfig(tier types.Tier) (*TierConfig, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	config, ok := ts.tierConfigs[tier]
	return config, ok
}

// UpdateTierWeight 更新 Tier 权重
func (ts *TierSystem) UpdateTierWeight(tier types.Tier, weight float64) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if config, ok := ts.tierConfigs[tier]; ok {
		config.Weight = weight
		return nil
	}
	return fmt.Errorf("tier not found")
}

// ==================== GVS 引擎 ====================

// GVSEngine GVS 引擎
type GVSEngine struct {
	calculator    *GvsCalculator
	tierSystem   *TierSystem
	updates      map[types.GameID]*GVSUpdate
	ranking      *GVSRanking
	mu           sync.RWMutex
}

// GVSUpdate GVS 更新
type GVSUpdate struct {
	GameID     types.GameID `json:"game_id"`
	GVS        float64     `json:"gvs"`
	PreviousGVS float64    `json:"previous_gvs"`
	Change     float64     `json:"change"`
	Trend      TrendDirection `json:"trend"`
	Tier       types.Tier   `json:"tier"`
	Timestamp  int64        `json:"timestamp"`
}

// TrendDirection 趋势方向
type TrendDirection int

const (
	TrendStable TrendDirection = iota
	TrendUp
	TrendDown
)

// GVSRanking GVS 排名
type GVSRanking struct {
	games     []GVSRank
	updatedAt int64
	mu        sync.RWMutex
}

// GVSRank GVS 排名
type GVSRank struct {
	GameID   types.GameID `json:"game_id"`
	GVS      float64     `json:"gvs"`
	Rank     int         `json:"rank"`
	Trend    TrendDirection `json:"trend"`
	Volume   uint64      `json:"volume"` // 交易量/活动量
}

// NewGVSEngine 创建 GVS 引擎
func NewGVSEngine() *GVSEngine {
	return &GVSEngine{
		calculator:  NewGvsCalculator(nil),
		tierSystem: NewTierSystem(),
		updates:    make(map[types.GameID]*GVSUpdate),
		ranking:    &GVSRanking{},
	}
}

// ProcessDataPoint 处理数据点
func (ge *GVSEngine) ProcessDataPoint(dataPoint *types.GameDataPoint) (*GVSUpdate, error) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	// 确定 Tier
	tier := ge.tierSystem.DetermineTier(dataPoint.OnlinePlayers)

	// 更新覆盖计数
	ge.calculator.UpdateCoverage(dataPoint.GameID)

	// 计算 GVS
	dataPoints := []types.GameDataPoint{*dataPoint}
	gvs := ge.calculator.Calculate(dataPoints)

	// 获取之前的 GVS
	previousGVS := float64(0)
	if existing, ok := ge.updates[dataPoint.GameID]; ok {
		previousGVS = existing.GVS
	}

	// 计算变化
	change := gvs - previousGVS

	// 确定趋势
	var trend TrendDirection
	if change > 0.01 {
		trend = TrendUp
	} else if change < -0.01 {
		trend = TrendDown
	} else {
		trend = TrendStable
	}

	// 创建更新记录
	update := &GVSUpdate{
		GameID:      dataPoint.GameID,
		GVS:         gvs,
		PreviousGVS: previousGVS,
		Change:      change,
		Trend:       trend,
		Tier:        tier,
		Timestamp:   time.Now().Unix(),
	}

	ge.updates[dataPoint.GameID] = update

	// 记录到历史
	ge.calculator.RecordGVS(dataPoint.GameID, gvs)

	// 更新排名
	ge.updateRanking()

	return update, nil
}

// ProcessBatch 批量处理
func (ge *GVSEngine) ProcessBatch(dataPoints []types.GameDataPoint) map[types.GameID]*GVSUpdate {
	results := make(map[types.GameID]*GVSUpdate)

	// 按游戏分组
	gameData := make(map[types.GameID][]types.GameDataPoint)
	for _, dp := range dataPoints {
		gameData[dp.GameID] = append(gameData[dp.GameID], dp)
	}

	// 计算每个游戏的 GVS
	for gameID, dps := range gameData {
		update, err := ge.ProcessDataPoint(&dps[len(dps)-1]) // 使用最新数据点
		if err == nil {
			results[gameID] = update
		}
	}

	return results
}

// updateRanking 更新排名
func (ge *GVSEngine) updateRanking() {
	ge.ranking.mu.Lock()
	defer ge.ranking.mu.Unlock()

	// 收集所有游戏
	games := make([]GVSRank, 0, len(ge.updates))
	for gameID, update := range ge.updates {
		games = append(games, GVSRank{
			GameID: gameID,
			GVS:    update.GVS,
			Trend:  update.Trend,
		})
	}

	// 按 GVS 排序
	sort.Slice(games, func(i, j int) bool {
		return games[i].GVS > games[j].GVS
	})

	// 更新排名
	for i := range games {
		games[i].Rank = i + 1
	}

	ge.ranking.games = games
	ge.ranking.updatedAt = time.Now().Unix()
}

// GetRanking 获取排名
func (ge *GVSEngine) GetRanking(limit int) []GVSRank {
	ge.ranking.mu.RLock()
	defer ge.ranking.mu.RUnlock()

	if limit > 0 && limit < len(ge.ranking.games) {
		return ge.ranking.games[:limit]
	}

	result := make([]GVSRank, len(ge.ranking.games))
	copy(result, ge.ranking.games)
	return result
}

// GetGVS 获取游戏 GVS
func (ge *GVSEngine) GetGVS(gameID types.GameID) (*GVSUpdate, bool) {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	update, ok := ge.updates[gameID]
	return update, ok
}

// GetAllGVS 获取所有 GVS
func (ge *GVSEngine) GetAllGVS() map[types.GameID]float64 {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	result := make(map[types.GameID]float64)
	for gameID, update := range ge.updates {
		result[gameID] = update.GVS
	}
	return result
}

// GetTier 获取游戏 Tier
func (ge *GVSEngine) GetTier(gameID types.GameID) (types.Tier, bool) {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	if update, ok := ge.updates[gameID]; ok {
		return update.Tier, true
	}
	return types.Tier3, false
}

// CalculateTrend 计算趋势
func (ge *GVSEngine) CalculateTrend(gameID types.GameID) (TrendDirection, float64, bool) {
	trend, ok := ge.calculator.CalculateTrend(gameID)
	if !ok {
		return TrendStable, 0, false
	}

	var direction TrendDirection
	if trend > 0.1 {
		direction = TrendUp
	} else if trend < -0.1 {
		direction = TrendDown
	} else {
		direction = TrendStable
	}

	return direction, trend, true
}

// GetTopGainers 获取涨幅最大的游戏
func (ge *GVSEngine) GetTopGainers(limit int) []GVSRank {
	ge.ranking.mu.RLock()
	defer ge.ranking.mu.RUnlock()

	gainers := make([]GVSRank, 0)
	for _, g := range ge.ranking.games {
		if g.Trend == TrendUp {
			gainers = append(gainers, g)
		}
	}

	if limit > 0 && limit < len(gainers) {
		return gainers[:limit]
	}
	return gainers
}

// GetTopLosers 获取跌幅最大的游戏
func (ge *GVSEngine) GetTopLosers(limit int) []GVSRank {
	ge.ranking.mu.RLock()
	defer ge.ranking.mu.RUnlock()

	losers := make([]GVSRank, 0)
	for _, g := range ge.ranking.games {
		if g.Trend == TrendDown {
			losers = append(losers, g)
		}
	}

	if limit > 0 && limit < len(losers) {
		return losers[:limit]
	}
	return losers
}

// GetByTier 获取指定 Tier 的游戏
func (ge *GVSEngine) GetByTier(tier types.Tier) []GVSRank {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	ge.ranking.mu.RLock()
	defer ge.ranking.mu.RUnlock()

	var results []GVSRank
	for _, g := range ge.ranking.games {
		if update, ok := ge.updates[g.GameID]; ok && update.Tier == tier {
			results = append(results, g)
		}
	}

	return results
}

// ==================== GVS 公式实现 ====================

// CalculateGVSFormula 实现 GVS 公式
// GVS = Base_GLV × Tier_Coefficient × Time_Decay × Coverage_Bonus
func CalculateGVSFormula(
	onlinePlayers uint64,
	peakPlayers uint64,
	averagePlayers uint64,
	tier types.Tier,
	dataAgeHours float64,
	nodeCount int,
) float64 {
	// 1. Base_GLV - 基础游戏列表价值
	baseGLV := calculateBaseGLV(onlinePlayers, peakPlayers, averagePlayers)

	// 2. Tier_Coefficient - 层级系数
	tierCoeff := float64(tier.Weight())

	// 3. Time_Decay - 时间衰减
	timeDecay := calculateTimeDecayFactor(dataAgeHours)

	// 4. Coverage_Bonus - 覆盖加成
	coverageBonus := calculateCoverageBonus(nodeCount)

	// 最终 GVS
	gvs := baseGLV * tierCoeff * timeDecay * coverageBonus

	return math.Max(0, gvs)
}

// calculateBaseGLV 计算基础 GLV
func calculateBaseGLV(online, peak, average uint64) float64 {
	// GLV = w1 * ln(online+1) + w2 * sqrt(peak) + w3 * log(average+1)
	onlineScore := math.Log(float64(online) + 1)
	peakScore := math.Sqrt(float64(peak) + 1)
	avgScore := math.Log(float64(average) + 1)

	// 权重
	w1, w2, w3 := 0.5, 0.3, 0.2

	glv := w1*onlineScore + w2*peakScore + w3*avgScore
	return glv
}

// calculateTimeDecayFactor 计算时间衰减因子
func calculateTimeDecayFactor(ageHours float64) float64 {
	// 24小时半衰期
	halfLife := 24.0
	decay := math.Pow(2, -ageHours/halfLife)
	return math.Max(0.1, decay) // 最小 10%
}

// calculateCoverageBonus 计算覆盖加成
func calculateCoverageBonus(nodeCount int) float64 {
	if nodeCount <= 0 {
		return 1.0
	}

	// 基准: 5个节点 = 1.0x, 10个节点 = 1.2x, 20个节点 = 1.5x, 50个节点 = 2.0x
	bonus := 1.0 + math.Log(float64(nodeCount)+1)*0.3
	return math.Min(bonus, 2.0) // 最大 2x
}

// ==================== GVS 统计 ====================

// GVSStats GVS 统计
type GVSStats struct {
	TotalGames     int               `json:"total_games"`
	TotalGVS       float64           `json:"total_gvs"`
	AverageGVS     float64           `json:"average_gvs"`
	TopGame        *GVSRank          `json:"top_game"`
	TierDistribution map[types.Tier]int `json:"tier_distribution"`
	UpdatedAt      int64             `json:"updated_at"`
}

// GetStats 获取统计信息
func (ge *GVSEngine) GetStats() GVSStats {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	ge.ranking.mu.RLock()
	defer ge.ranking.mu.RUnlock()

	var totalGVS float64
	tierDist := make(map[types.Tier]int)

	for _, update := range ge.updates {
		totalGVS += update.GVS
		tierDist[update.Tier]++
	}

	avgGVS := float64(0)
	if len(ge.updates) > 0 {
		avgGVS = totalGVS / float64(len(ge.updates))
	}

	var topGame *GVSRank
	if len(ge.ranking.games) > 0 {
		topGame = &ge.ranking.games[0]
	}

	return GVSStats{
		TotalGames:      len(ge.updates),
		TotalGVS:        totalGVS,
		AverageGVS:      avgGVS,
		TopGame:         topGame,
		TierDistribution: tierDist,
		UpdatedAt:       time.Now().Unix(),
	}
}

// ==================== GVS 奖励计算 ====================

// GVSRewardCalculator GVS 奖励计算器
type GVSRewardCalculator struct {
	totalRewardPool float64
	tierWeights    map[types.Tier]float64
}

// NewGVSRewardCalculator 创建奖励计算器
func NewGVSRewardCalculator(totalRewardPool float64) *GVSRewardCalculator {
	return &GVSRewardCalculator{
		totalRewardPool: totalRewardPool,
		tierWeights: map[types.Tier]float64{
			types.Tier1: 0.60, // 60% 给 Tier1
			types.Tier2: 0.30, // 30% 给 Tier2
			types.Tier3: 0.10, // 10% 给 Tier3
		},
	}
}

// CalculateReward 计算奖励
func (rc *GVSRewardCalculator) CalculateReward(gvs float64, tier types.Tier, totalTierGVS float64) float64 {
	if totalTierGVS == 0 {
		return 0
	}

	// 该 Tier 的奖励池
	tierPool := rc.totalRewardPool * rc.tierWeights[tier]

	// 按 GVS 占比分配
	share := gvs / totalTierGVS
	reward := tierPool * share

	return reward
}

// CalculateNodeReward 计算节点奖励
func (rc *GVSRewardCalculator) CalculateNodeReward(
	nodeGVS float64,
	nodeTier types.Tier,
	totalNetworkGVS float64,
	nodeStake float64,
	totalStake float64,
) float64 {
	// 基础奖励 = 总奖励 * (节点GVS / 网络总GVS)
	baseReward := rc.totalRewardPool * (nodeGVS / totalNetworkGVS)

	// 质押加权 = 基础奖励 * (节点质押 / 总质押)
	stakeMultiplier := 1.0
	if totalStake > 0 {
		stakeMultiplier = 1.0 + (nodeStake/totalStake)*0.5
	}

	return baseReward * stakeMultiplier
}

// ==================== GVS 快照 ====================

// GVSSnapshot GVS 快照
type GVSSnapshot struct {
	Height    types.BlockHeight `json:"height"`
	Timestamp int64             `json:"timestamp"`
	GVSMap    map[types.GameID]float64 `json:"gvs_map"`
}

// CreateSnapshot 创建快照
func (ge *GVSEngine) CreateSnapshot(height types.BlockHeight) *GVSSnapshot {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	gvsMap := make(map[types.GameID]float64)
	for gameID, update := range ge.updates {
		gvsMap[gameID] = update.GVS
	}

	return &GVSSnapshot{
		Height:    height,
		Timestamp: time.Now().Unix(),
		GVSMap:    gvsMap,
	}
}

// RestoreSnapshot 恢复快照
func (ge *GVSEngine) RestoreSnapshot(snapshot *GVSSnapshot) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	for gameID, gvs := range snapshot.GVSMap {
		ge.updates[gameID] = &GVSUpdate{
			GameID:   gameID,
			GVS:      gvs,
			Timestamp: snapshot.Timestamp,
		}
		ge.calculator.RecordGVS(gameID, gvs)
	}

	ge.updateRanking()
}
