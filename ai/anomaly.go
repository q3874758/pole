package ai

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== AI 配置 ====================

// Config AI 模块配置
type Config struct {
	EnableAnomalyDetection bool          // 启用异常检测
	EnableGVSEnhancement  bool          // 启用 GVS 增强
	ModelUpdateInterval   time.Duration // 模型更新间隔
	MaxModelSize         int           // 最大模型大小 (MB)
	CPUBudgetPercent     int           // CPU 预算百分比
	MemoryBudgetMB       int           // 内存预算 (MB)
	InferenceTimeout     time.Duration // 推理超时
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		EnableAnomalyDetection: true,
		EnableGVSEnhancement:  true,
		ModelUpdateInterval:   7 * 24 * time.Hour, // 每周更新
		MaxModelSize:         100,                  // 100MB
		CPUBudgetPercent:     5,                    // 5%
		MemoryBudgetMB:       500,                   // 500MB
		InferenceTimeout:     10 * time.Millisecond, // 10ms
	}
}

// ==================== 异常检测 ====================

// AnomalyDetector 异常检测器
type AnomalyDetector struct {
	config       *Config
	dataCheck    *DataIntegrityChecker
	behaviorAnal *BehaviorAnalyzer
	networkAnal *NetworkAnalyzer
	mu           sync.RWMutex
	models       map[string]*AnomalyModel
}

// DataIntegrityChecker 数据完整性检查器 (第一层)
type DataIntegrityChecker struct {
	maxDataAge       time.Duration
	deviationThreshold float64
}

// BehaviorAnalyzer 行为分析器 (第二层)
type BehaviorAnalyzer struct {
	nodeProfiles    map[types.NodeID]*NodeProfile
	anomalyPatterns []AnomalyPattern
}

// NetworkAnalyzer 网络分析器 (第三层)
type NetworkAnalyzer struct {
	clusters   map[string][]types.NodeID
	geoAnomaly *GeoAnomalyDetector
}

// AnomalyModel 异常检测模型
type AnomalyModel struct {
	Name      string
	Type      ModelType
	Version   int
	Data      []byte
	TrainedAt time.Time
	Accuracy  float64
}

// ModelType 模型类型
type ModelType int

const (
	ModelTypeDataIntegrity ModelType = iota
	ModelTypeBehavior
	ModelTypeNetwork
	ModelTypeGVSScoring
)

// NodeProfile 节点画像
type NodeProfile struct {
	NodeID         types.NodeID
	CollectedGames map[types.GameID]*GameHistory
	ActivityTimes  []int64 // 小时分布
	Reputation     float64
	LastUpdate     int64
}

// GameHistory 游戏历史
type GameHistory struct {
	GameID       types.GameID
	DataPoints   []DataPoint
	AnomalyScore float64
	LastSubmit   int64
}

// DataPoint 数据点
type DataPoint struct {
	Timestamp     int64
	OnlinePlayers uint64
	PeakPlayers   uint64
	Tier          types.Tier
}

// AnomalyPattern 异常模式
type AnomalyPattern struct {
	Name        string
	Type        string
	Threshold   float64
	Severity    float64
	Description string
}

// GeoAnomalyDetector 地理位置异常检测
type GeoAnomalyDetector struct {
	knownLocations map[types.NodeID][]string
	vpnDetection   bool
}

// NewAnomalyDetector 创建异常检测器
func NewAnomalyDetector(config *Config) *AnomalyDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &AnomalyDetector{
		config:        config,
		dataCheck:    &DataIntegrityChecker{maxDataAge: 24 * time.Hour, deviationThreshold: 0.2},
		behaviorAnal: &BehaviorAnalyzer{nodeProfiles: make(map[types.NodeID]*NodeProfile)},
		networkAnal:  &NetworkAnalyzer{clusters: make(map[string][]types.NodeID)},
		models:       make(map[string]*AnomalyModel),
	}
}

// ==================== 第一层：数据完整性检测 ====================

// CheckDataIntegrity 检查数据完整性
func (ad *AnomalyDetector) CheckDataIntegrity(data *types.GameDataPoint, source string) *AnomalyResult {
	result := &AnomalyResult{
		NodeID:      "",
		GameID:      data.GameID,
		Timestamp:   time.Now(),
		Severity:   0,
		Type:       "data_integrity",
		Description: "",
		Passed:     true,
	}

	// 检查时间戳
	now := time.Now().Unix()
	if data.Timestamp > now {
		result.Passed = false
		result.Severity = 1.0
		result.Description = "future timestamp detected"
		return result
	}

	if now-data.Timestamp > int64(ad.dataCheck.maxDataAge.Seconds()) {
		result.Passed = false
		result.Severity = 0.8
		result.Description = "data older than 24 hours"
		return result
	}

	// 检查数值合理性
	if data.OnlinePlayers > 100000000 { // 1亿上限
		result.Passed = false
		result.Severity = 1.0
		result.Description = "unrealistic player count"
		return result
	}

	// 峰值不应小于在线玩家
	if data.PeakPlayers < data.OnlinePlayers {
		result.Passed = false
		result.Severity = 0.9
		result.Description = "peak players less than online"
		return result
	}

	return result
}

// CrossValidate 交叉验证多个数据源
func (ad *AnomalyDetector) CrossValidate(gameID types.GameID, dataPoints []*types.GameDataPoint) *AnomalyResult {
	result := &AnomalyResult{
		GameID:     gameID,
		Timestamp:  time.Now(),
		Severity:   0,
		Type:       "cross_validation",
		Passed:     true,
	}

	if len(dataPoints) < 2 {
		result.Passed = false
		result.Severity = 0.5
		result.Description = "insufficient data points for validation"
		return result
	}

	// 计算平均值标准差和
	var sum float64
	values := make([]float64, len(dataPoints))
	for i, dp := range dataPoints {
		values[i] = float64(dp.OnlinePlayers)
		sum += values[i]
	}
	avg := sum / float64(len(values))

	var variance float64
	for _, v := range values {
		diff := v - avg
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(values)))

	// 检查偏差
	coefficientOfVariation := stdDev / avg
	if coefficientOfVariation > ad.dataCheck.deviationThreshold {
		result.Passed = false
		result.Severity = coefficientOfVariation
		result.Description = fmt.Sprintf("high deviation: %.2f%%", coefficientOfVariation*100)
	}

	return result
}

// ==================== 第二层：行为模式分析 ====================

// AnalyzeBehavior 分析节点行为
func (ad *AnomalyDetector) AnalyzeBehavior(nodeID types.NodeID, gameID types.GameID, data *types.GameDataPoint) *AnomalyResult {
	result := &AnomalyResult{
		NodeID:    nodeID,
		GameID:    gameID,
		Timestamp: time.Now(),
		Severity:  0,
		Type:      "behavior_analysis",
		Passed:   true,
	}

	// 更新节点画像
	profile := ad.getOrCreateProfile(nodeID)
	profile.addDataPoint(gameID, data)

	// 检测瞬间玩家数突变
	if profile.detectSuddenChange(gameID) {
		result.Passed = false
		result.Severity = 0.8
		result.Description = "sudden player count change detected"
		return result
	}

	// 检测规律性刷分
	if profile.detectRegularPattern(gameID) {
		result.Passed = false
		result.Severity = 0.9
		result.Description = "regular pattern detected (potential score manipulation)"
		return result
	}

	// 检测非人类操作特征
	if profile.detectNonHumanActivity() {
		result.Passed = false
		result.Severity = 0.7
		result.Description = "non-human activity pattern detected"
		return result
	}

	return result
}

// getOrCreateProfile 获取或创建节点画像
func (ad *AnomalyDetector) getOrCreateProfile(nodeID types.NodeID) *NodeProfile {
	ad.behaviorAnal.mu.Lock()
	defer ad.behaviorAnal.mu.Unlock()

	profile, ok := ad.behaviorAnal.nodeProfiles[nodeID]
	if !ok {
		profile = &NodeProfile{
			NodeID:         nodeID,
			CollectedGames: make(map[types.GameID]*GameHistory),
			Reputation:     100.0,
			LastUpdate:     time.Now().Unix(),
		}
		ad.behaviorAnal.nodeProfiles[nodeID] = profile
	}
	return profile
}

// detectSuddenChange 检测瞬间突变
func (np *NodeProfile) detectSuddenChange(gameID types.GameID) bool {
	history, ok := np.CollectedGames[gameID]
	if !ok || len(history.DataPoints) < 2 {
		return false
	}

	// 获取最近的两个数据点
	n := len(history.DataPoints)
	recent := history.DataPoints[n-1]
	previous := history.DataPoints[n-2]

	if previous.OnlinePlayers == 0 {
		return false
	}

	// 计算变化率
	changeRate := float64(recent.OnlinePlayers) / float64(previous.OnlinePlayers)
	
	// 超过 10 倍变化视为异常
	return changeRate > 10 || changeRate < 0.1
}

// detectRegularPattern 检测规律性模式
func (np *NodeProfile) detectRegularPattern(gameID types.GameID) bool {
	history, ok := np.CollectedGames[gameID]
	if !ok || len(history.DataPoints) < 10 {
		return false
	}

	// 检查时间间隔是否规律
	var intervals []int64
	for i := 1; i < len(history.DataPoints); i++ {
		interval := history.DataPoints[i].Timestamp - history.DataPoints[i-1].Timestamp
		intervals = append(intervals, interval)
	}

	// 计算标准差
	var sum int64
	for _, iv := range intervals {
		sum += iv
	}
	avg := float64(sum) / float64(len(intervals))

	var variance float64
	for _, iv := range intervals {
		diff := float64(iv) - avg
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(intervals)))

	// 标准差小于平均值 5% 视为规律
	return stdDev < avg*0.05
}

// detectNonHumanActivity 检测非人类活动
func (np *NodeProfile) detectNonHumanActivity() bool {
	if len(np.ActivityTimes) < 24 {
		return false
	}

	// 检查是否 24 小时均匀分布（非人类特征）
	var count [24]int
	for _, t := range np.ActivityTimes {
		hour := time.Unix(t, 0).Hour()
		count[hour]++
	}

	// 计算方差
	var sum float64
	for _, c := range count {
		sum += float64(c)
	}
	avg := sum / 24

	var variance float64
	for _, c := range count {
		diff := float64(c) - avg
		variance += diff * diff
	}
	coefficientOfVariation := math.Sqrt(variance/24) / avg

	// 方差极小可能是机器人
	return coefficientOfVariation < 0.1
}

// addDataPoint 添加数据点
func (np *NodeProfile) addDataPoint(gameID types.GameID, data *types.GameDataPoint) {
	history, ok := np.CollectedGames[gameID]
	if !ok {
		history = &GameHistory{GameID: gameID}
		np.CollectedGames[gameID] = history
	}

	history.DataPoints = append(history.DataPoints, DataPoint{
		Timestamp:     data.Timestamp,
		OnlinePlayers: data.OnlinePlayers,
		PeakPlayers:   data.PeakPlayers,
		Tier:          data.Tier,
	})
	history.LastSubmit = time.Now().Unix()

	// 保留最近 1000 个点
	if len(history.DataPoints) > 1000 {
		history.DataPoints = history.DataPoints[len(history.DataPoints)-1000:]
	}

	np.LastUpdate = time.Now().Unix()
}

// ==================== 第三层：网络协同检测 ====================

// AnalyzeNetwork 网络协同分析
func (ad *AnomalyDetector) AnalyzeNetwork(nodeIDs []types.NodeID, dataPoints []*types.GameDataPoint) *AnomalyResult {
	result := &AnomalyResult{
		Timestamp: time.Now(),
		Severity:  0,
		Type:      "network_analysis",
		Passed:    true,
	}

	// 检测串通作弊
	if ad.detectCollusion(nodeIDs, dataPoints) {
		result.Passed = false
		result.Severity = 0.95
		result.Description = "potential collusion detected"
		return result
	}

	// 检测 Sybil 攻击
	if ad.detectSybilAttack(nodeIDs) {
		result.Passed = false
		result.Severity = 1.0
		result.Description = "Sybil attack detected"
		return result
	}

	return result
}

// detectCollusion 检测串通
func (ad *AnomalyDetector) detectCollusion(nodeIDs []types.NodeID, dataPoints []*types.GameDataPoint) bool {
	if len(nodeIDs) < 3 {
		return false
	}

	// 检查数据相似度
	var correlations int
	for i := 0; i < len(dataPoints); i++ {
		for j := i + 1; j < len(dataPoints); j++ {
			if dataPoints[i].OnlinePlayers == dataPoints[j].OnlinePlayers {
				correlations++
			}
		}
	}

	// 高度相似的数据可能表示串通
	totalPairs := len(dataPoints) * (len(dataPoints) - 1) / 2
	similarityRatio := float64(correlations) / float64(totalPairs)

	return similarityRatio > 0.8
}

// detectSybilAttack 检测 Sybil 攻击
func (ad *AnomalyDetector) detectSybilAttack(nodeIDs []types.NodeID) bool {
	// 检查同一硬件指纹下的多开
	// 简化：检查节点数量是否异常
	return len(nodeIDs) > 1000
}

// ==================== 异常结果 ====================

// AnomalyResult 异常检测结果
type AnomalyResult struct {
	NodeID      types.NodeID
	GameID      types.GameID
	Timestamp   time.Time
	Severity    float64
	Type        string
	Description string
	Passed      bool
	Metadata    map[string]interface{}
}

// ==================== GVS 智能计算 ====================

// GVSScorer GVS 智能评分器
type GVSScorer struct {
	config       *Config
	baseScorer   *BaseScorer
	aiEnhancer   *AIEnhancer
	weights      *DynamicWeights
	mu           sync.RWMutex
}

// BaseScorer 基础评分器
type BaseScorer struct {
	baseWeights map[string]float64
}

// AIEnhancer AI 增强器
type AIEnhancer struct {
	retentionAnalyzer   *RetentionAnalyzer
	communityAnalyzer   *CommunityAnalyzer
	trendPredictor      *TrendPredictor
}

// DynamicWeights 动态权重
type DynamicWeights struct {
	RetentionWeight      float64
	ConversionWeight    float64
	CommunityWeight     float64
	ScarcityWeight      float64
	DecentralizationWeight float64
	LastUpdate          int64
}

// RetentionAnalyzer 留存率分析器
type RetentionAnalyzer struct {
	historicalData map[types.GameID]*RetentionData
}

// RetentionData 留存数据
type RetentionData struct {
	Day1  float64 // 1 日留存率
	Day7  float64 // 7 日留存率
	Day30 float64 // 30 日留存率
}

// CommunityAnalyzer 社区活跃度分析器
type CommunityAnalyzer struct {
	socialMetrics map[types.GameID]*SocialMetrics
}

// SocialMetrics 社交指标
type SocialMetrics struct {
	DiscordMembers int
	RedditSubs     int
	TwitterFollowers int
	ActivityScore  float64
}

// TrendPredictor 市场趋势预测器
type TrendPredictor struct {
	historicalScores map[types.GameID][]float64
}

// NewGVSScorer 创建 GVS 评分器
func NewGVSScorer(config *Config) *GVSScorer {
	if config == nil {
		config = DefaultConfig()
	}

	return &GVSScorer{
		config:     config,
		baseScorer: &BaseScorer{baseWeights: map[string]float64{
			"online":       0.4,
			"peak":         0.3,
			"average":      0.2,
			"tier":         0.1,
		}},
		aiEnhancer: &AIEnhancer{
			retentionAnalyzer:  &RetentionAnalyzer{historicalData: make(map[types.GameID]*RetentionData)},
			communityAnalyzer: &CommunityAnalyzer{socialMetrics: make(map[types.GameID]*SocialMetrics)},
			trendPredictor:    &TrendPredictor{historicalScores: make(map[types.GameID][]float64)},
		},
		weights: &DynamicWeights{
			RetentionWeight:         0.15,
			ConversionWeight:       0.20,
			CommunityWeight:        0.15,
			ScarcityWeight:         0.10,
			DecentralizationWeight: 0.10,
			LastUpdate:             time.Now().Unix(),
		},
	}
}

// CalculateGVS 计算 GVS
func (gs *GVSScorer) CalculateGVS(gameID types.GameID, dataPoints []types.GameDataPoint) float64 {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// 基础分数
	baseScore := gs.calculateBaseScore(dataPoints)

	// AI 增强分数
	aiScore := float64(0)
	if gs.config.EnableGVSEnhancement {
		aiScore = gs.calculateAIScore(gameID, dataPoints)
	}

	// 动态权重调整
	gs.adjustWeights()

	// 综合分数
	totalWeight := 0.7 + gs.weights.RetentionWeight + gs.weights.ConversionWeight + 
		gs.weights.CommunityWeight + gs.weights.ScarcityWeight + gs.weights.DecentralizationWeight

	gvs := (baseScore * 0.7) / totalWeight
	gvs += aiScore

	return gvs
}

// calculateBaseScore 计算基础分数
func (gs *GVSScorer) calculateBaseScore(dataPoints []types.GameDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0
	}

	var onlineScore, peakScore, avgScore float64
	var tierWeight float64

	for _, dp := range dataPoints {
		// 在线玩家分数 (归一化到 0-100)
		onlineScore += math.Log10(float64(dp.OnlinePlayers)+1) * 10

		// 峰值分数
		peakScore += math.Log10(float64(dp.PeakPlayers)+1) * 8

		// 层级权重
		tierWeight += float64(dp.Tier.Weight())
	}

	onlineScore /= float64(len(dataPoints))
	peakScore /= float64(len(dataPoints))
	tierWeight /= float64(len(dataPoints))

	// 加权计算
	baseScore := onlineScore*gs.baseScorer.baseWeights["online"] +
		peakScore*gs.baseScorer.baseWeights["peak"] +
		tierWeight*gs.baseScorer.baseWeights["tier"]

	return baseScore
}

// calculateAIScore 计算 AI 增强分数
func (gs *GVSScorer) calculateAIScore(gameID types.GameID, dataPoints []types.GameDataPoint) float64 {
	var aiScore float64

	// 留存率分数
	retentionScore := gs.aiEnhancer.retentionAnalyzer.analyzeRetention(gameID)
	aiScore += retentionScore * gs.weights.RetentionWeight

	// 社区活跃度分数
	communityScore := gs.aiEnhancer.communityAnalyzer.analyzeCommunity(gameID)
	aiScore += communityScore * gs.weights.CommunityWeight

	// 市场趋势预测
	trendScore := gs.aiEnhancer.trendPredictor.predictTrend(gameID, dataPoints)
	aiScore += trendScore * 0.2

	return aiScore
}

// adjustWeights 调整动态权重
func (gs *GVSScorer) adjustWeights() {
	// 根据市场环境自动调整
	// 简化实现
	now := time.Now().Unix()
	if now-gs.weights.LastUpdate > 86400 { // 每天更新
		// 根据网络规模调整
		gs.weights.DecentralizationWeight = 0.10
		
		gs.weights.LastUpdate = now
	}
}

// analyzeRetention 分析留存率
func (ra *RetentionAnalyzer) analyzeRetention(gameID types.GameID) float64 {
	data, ok := ra.historicalData[gameID]
	if !ok {
		// 模拟数据
		return 0.5 + rand.Float64()*0.3
	}

	// 综合留存率分数
	score := data.Day1*0.5 + data.Day7*0.3 + data.Day30*0.2
	return score
}

// analyzeCommunity 分析社区活跃度
func (ca *CommunityAnalyzer) analyzeCommunity(gameID types.GameID) float64 {
	metrics, ok := ca.socialMetrics[gameID]
	if !ok {
		// 模拟数据
		return 0.3 + rand.Float64()*0.4
	}

	// 归一化分数
	score := math.Log10(float64(metrics.DiscordMembers+1))*0.3 +
		math.Log10(float64(metrics.RedditSubs+1))*0.3 +
		math.Log10(float64(metrics.TwitterFollowers+1))*0.2 +
		metrics.ActivityScore*0.2

	return math.Min(score/10, 1.0)
}

// predictTrend 预测趋势
func (tp *TrendPredictor) predictTrend(gameID types.GameID, dataPoints []types.GameDataPoint) float64 {
	// 简化的趋势预测
	if len(dataPoints) < 2 {
		return 0.5
	}

	// 计算斜率
	n := float64(len(dataPoints))
	var sumX, sumY, sumXY, sumX2 float64
	for i, dp := range dataPoints {
		x := float64(i)
		y := float64(dp.OnlinePlayers)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	
	// 转换为 0-1 分数
	avgPlayers := sumY / n
	if avgPlayers == 0 {
		return 0.5
	}

	trendScore := (slope / avgPlayers) * 10 // 缩放因子
	trendScore = (trendScore + 1) / 2       // 转换到 0-1

	return math.Max(0, math.Min(1, trendScore))
}

// UpdateWeights 更新权重
func (gs *GVSScorer) UpdateWeights(weights *DynamicWeights) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.weights = weights
}

// GetWeights 获取当前权重
func (gs *GVSScorer) GetWeights() *DynamicWeights {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.weights
}

// ==================== 工具函数 ====================

// CalculateGLV 计算 GLV (Game List Value)
func CalculateGLV(online, peak, average uint64, tier types.Tier) float64 {
	// GLV = log(online + 1) * w1 + log(peak + 1) * w2 + log(average + 1) * w3
	onlineScore := math.Log10(float64(online) + 1)
	peakScore := math.Log10(float64(peak) + 1)
	avgScore := math.Log10(float64(average) + 1)

	glv := onlineScore*0.5 + peakScore*0.3 + avgScore*0.2
	glv *= float64(tier.Weight())

	return glv
}

// ApplyTimeDecay 应用时间衰减
func ApplyTimeDecay(baseScore float64, ageDays int) float64 {
	decayFactor := math.Pow(0.5, float64(ageDays)/30.0)
	return baseScore * decayFactor
}

// CalculateCoverageBonus 计算覆盖加成
func CalculateCoverageBonus(nodeCount int) float64 {
	// 节点越多，加成越高，上限 20%
	bonus := math.Log10(float64(nodeCount)+1) * 0.1
	return math.Min(bonus, 0.2)
}
