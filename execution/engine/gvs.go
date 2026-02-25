package engine

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== GVS 配置 ====================

// Config GVS 计算配置
type Config struct {
	BaseMultiplier    float64 // 基础乘数
	DecayHalfLife     float64 // 衰减半衰期 (小时)
	CoverageThreshold uint32  // 覆盖阈值
	MaxCoverageBonus  float64 // 最大覆盖加成
	WindowHours       uint32  // 滚动窗口 (小时)
}

func DefaultConfig() *Config {
	return &Config{
		BaseMultiplier:    1.0,
		DecayHalfLife:    24.0,  // 24小时
		CoverageThreshold: 5,
		MaxCoverageBonus: 1.5,
		WindowHours:      24,
	}
}

// ==================== GVS 记录 ====================

// GVSRecord GVS 记录
type GVSRecord struct {
	GameID   types.GameID `json:"game_id"`
	GVS      float64     `json:"gvs"`
	Updated  int64       `json:"updated"`
}

// ==================== GVS 计算器 ====================

// GvsCalculator GVS 计算器
type GvsCalculator struct {
	config   *Config
	history  map[types.GameID][]GVSRecord
	coverage map[types.GameID]uint32
	mu       sync.RWMutex
}

// NewGvsCalculator 创建 GVS 计算器
func NewGvsCalculator(config *Config) *GvsCalculator {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &GvsCalculator{
		config:   config,
		history:  make(map[types.GameID][]GVSRecord),
		coverage: make(map[types.GameID]uint32),
	}
}

// Calculate 计算单个游戏的 GVS
func (gc *GvsCalculator) Calculate(dataPoints []types.GameDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0.0
	}
	
	// 计算基础 GLV
	baseGLV := gc.calculateBaseGLV(dataPoints)
	
	// 获取层级系数
	tierCoefficient := gc.getTierCoefficient(dataPoints)
	
	// 时间衰减
	timeDecay := gc.calculateTimeDecay(dataPoints)
	
	// 覆盖加成
	coverageBonus := gc.calculateCoverageBonus(dataPoints[0].GameID)
	
	// 最终 GVS
	gvs := baseGLV * tierCoefficient * timeDecay * coverageBonus
	
	return math.Max(0.0, gvs)
}

// calculateBaseGLV 计算基础游戏列表价值
func (gc *GvsCalculator) calculateBaseGLV(dataPoints []types.GameDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0.0
	}
	
	n := float64(len(dataPoints))
	
	// 平均在线玩家数
	var avgOnline float64
	for _, dp := range dataPoints {
		avgOnline += float64(dp.OnlinePlayers)
	}
	avgOnline /= n
	
	// 峰值玩家
	peak := float64(dataPoints[0].PeakPlayers)
	for _, dp := range dataPoints {
		if float64(dp.PeakPlayers) > peak {
			peak = float64(dp.PeakPlayers)
		}
	}
	
	// GLV = ln(avg + 1) * sqrt(peak) * base_multiplier
	glv := math.Log(avgOnline+1) * math.Sqrt(peak+1) * gc.config.BaseMultiplier
	
	return glv
}

// getTierCoefficient 获取层级系数
func (gc *GvsCalculator) getTierCoefficient(dataPoints []types.GameDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 0.1
	}
	
	return dataPoints[len(dataPoints)-1].Tier.Weight()
}

// calculateTimeDecay 计算时间衰减
func (gc *GvsCalculator) calculateTimeDecay(dataPoints []types.GameDataPoint) float64 {
	if len(dataPoints) == 0 {
		return 1.0
	}
	
	// 找到最新数据点
	latest := dataPoints[0]
	for _, dp := range dataPoints {
		if dp.Timestamp > latest.Timestamp {
			latest = dp
		}
	}
	
	ageHours := float64(time.Now().Unix()-latest.Timestamp) / 3600.0
	
	// 指数衰减: 2^(-age/half_life)
	decay := math.Pow(2.0, -ageHours/gc.config.DecayHalfLife)
	
	// 最小 10% 权重
	return math.Max(0.1, decay)
}

// calculateCoverageBonus 计算覆盖加成
func (gc *GvsCalculator) calculateCoverageBonus(gameID types.GameID) float64 {
	gc.mu.RLock()
	nodeCount := gc.coverage[gameID]
	gc.mu.RUnlock()
	
	if nodeCount >= gc.config.CoverageThreshold {
		ratio := float64(nodeCount) / float64(gc.config.CoverageThreshold)
		bonus := 1.0 + (ratio * 0.5)
		if bonus > gc.config.MaxCoverageBonus {
			bonus = gc.config.MaxCoverageBonus
		}
		return bonus
	}
	
	return 1.0
}

// UpdateCoverage 更新覆盖计数
func (gc *GvsCalculator) UpdateCoverage(gameID types.GameID) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	
	gc.coverage[gameID]++
}

// RecordGVS 记录 GVS 计算结果
func (gc *GvsCalculator) RecordGVS(gameID types.GameID, gvs float64) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	
	record := GVSRecord{
		GameID:  gameID,
		GVS:     gvs,
		Updated: time.Now().Unix(),
	}
	
	gc.history[gameID] = append(gc.history[gameID], record)
}

// GetHistory 获取历史 GVS
func (gc *GvsCalculator) GetHistory(gameID types.GameID) []GVSRecord {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	
	records := gc.history[gameID]
	result := make([]GVSRecord, len(records))
	copy(result, records)
	return result
}

// GetAverageGVS 获取时间窗口内的平均 GVS
func (gc *GvsCalculator) GetAverageGVS(gameID types.GameID, hours uint32) (float64, bool) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	
	records, ok := gc.history[gameID]
	if !ok || len(records) == 0 {
		return 0, false
	}
	
	cutoff := time.Now().Unix() - int64(hours*3600)
	
	var sum float64
	count := 0
	for _, r := range records {
		if r.Updated > cutoff {
			sum += r.GVS
			count++
		}
	}
	
	if count == 0 {
		return 0, false
	}
	
	return sum / float64(count), true
}

// CalculateTrend 计算趋势
func (gc *GvsCalculator) CalculateTrend(gameID types.GameID) (float64, bool) {
	gc.mu.RLock()
	records := gc.history[gameID]
	gc.mu.RUnlock()
	
	if len(records) < 2 {
		return 0, false
	}
	
	mid := len(records) / 2
	
	var recentSum, olderSum float64
	for _, r := range records[mid:] {
		recentSum += r.GVS
	}
	for _, r := range records[:mid] {
		olderSum += r.GVS
	}
	
	recentAvg := recentSum / float64(len(records)-mid)
	olderAvg := olderSum / float64(mid)
	
	if olderAvg == 0 {
		return 0, false
	}
	
	return (recentAvg - olderAvg) / olderAvg, true
}

// ==================== 批量计算器 ====================

// BatchGvsCalculator 批量 GVS 计算器
type BatchGvsCalculator struct {
	calculator *GvsCalculator
}

// NewBatchGvsCalculator 创建批量计算器
func NewBatchGvsCalculator(config *Config) *BatchGvsCalculator {
	return &BatchGvsCalculator{
		calculator: NewGvsCalculator(config),
	}
}

// CalculateBatch 批量计算 GVS
func (bc *BatchGvsCalculator) CalculateBatch(
	gameData map[types.GameID][]types.GameDataPoint,
) map[types.GameID]float64 {
	results := make(map[types.GameID]float64)
	
	for gameID, data := range gameData {
		gvs := bc.calculator.Calculate(data)
		bc.calculator.RecordGVS(gameID, gvs)
		results[gameID] = gvs
	}
	
	return results
}

// GetTopGames 获取排名前列的游戏
func (bc *BatchGvsCalculator) GetTopGames(limit int) []struct {
	GameID types.GameID
	GVS    float64
} {
	bc.calculator.mu.RLock()
	defer bc.calculator.mu.RUnlock()
	
	type gameGVS struct {
		gameID types.GameID
		gvs    float64
	}
	
	games := make([]gameGVS, 0, len(bc.calculator.history))
	
	for gameID, records := range bc.calculator.history {
		if len(records) > 0 {
			gvs := records[len(records)-1].GVS
			games = append(games, gameGVS{gameID: gameID, gvs: gvs})
		}
	}
	
	// 按 GVS 排序
	sort.Slice(games, func(i, j int) bool {
		return games[i].gvs > games[j].gvs
	})
	
	// 限制数量
	if limit > 0 && limit < len(games) {
		games = games[:limit]
	}
	
	result := make([]struct {
		GameID types.GameID
		GVS    float64
	}, len(games))
	
	for i, g := range games {
		result[i].GameID = g.gameID
		result[i].GVS = g.gvs
	}
	
	return result
}

// ==================== 异常检测 ====================

// AnomalyDetector 异常检测器
type AnomalyDetector struct {
	zscoreThreshold float64
	minSamples      int
	windowSize      int
}

// NewAnomalyDetector 创建异常检测器
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{
		zscoreThreshold: 3.0,
		minSamples:      10,
		windowSize:      5,
	}
}

// DetectZScore 使用 Z-score 方法检测异常
func (ad *AnomalyDetector) DetectZScore(data []uint64) []int {
	if len(data) < ad.minSamples {
		return nil
	}
	
	// 计算平均值
	var sum float64
	for _, v := range data {
		sum += float64(v)
	}
	mean := sum / float64(len(data))
	
	// 计算标准差
	var variance float64
	for _, v := range data {
		diff := float64(v) - mean
		variance += diff * diff
	}
	variance /= float64(len(data))
	stdDev := math.Sqrt(variance)
	
	if stdDev == 0 {
		return nil
	}
	
	// 查找异常值
	anomalies := make([]int, 0)
	for i, v := range data {
		zscore := math.Abs(float64(v)-mean) / stdDev
		if zscore > ad.zscoreThreshold {
			anomalies = append(anomalies, i)
		}
	}
	
	return anomalies
}

// DetectSuddenChanges 检测突变
func (ad *AnomalyDetector) DetectSuddenChanges(data []uint64, threshold float64) [][3]uint64 {
	if len(data) < 2 {
		return nil
	}
	
	anomalies := make([][3]uint64, 0)
	
	for i := 1; i < len(data); i++ {
		prev := float64(data[i-1])
		curr := float64(data[i])
		
		if prev == 0 {
			continue
		}
		
		change := math.Abs(curr-prev) / prev
		if change > threshold {
			anomalies = append(anomalies, [3]uint64{uint64(i-1), data[i-1], data[i]})
		}
	}
	
	return anomalies
}

// ValidateNodeData 验证节点数据
func (ad *AnomalyDetector) ValidateNodeData(
	nodeData []types.GameDataPoint,
	networkData []types.GameDataPoint,
) ValidationResult {
	if len(nodeData) == 0 {
		return ValidationResult{
			IsValid:  false,
			Anomalies: []string{"No data submitted"},
			Deviation: 1.0,
		}
	}
	
	anomalies := make([]string, 0)
	
	// 计算节点平均值
	var nodeAvg float64
	for _, dp := range nodeData {
		nodeAvg += float64(dp.OnlinePlayers)
	}
	nodeAvg /= float64(len(nodeData))
	
	// 计算网络平均值
	var networkAvg float64
	for _, dp := range networkData {
		networkAvg += float64(dp.OnlinePlayers)
	}
	networkAvg /= float64(len(networkData))
	
	deviation := 0.0
	if networkAvg > 0 {
		deviation = math.Abs(nodeAvg-networkAvg) / networkAvg
	}
	
	// 检测异常
	playerCounts := make([]uint64, len(nodeData))
	for i, dp := range nodeData {
		playerCounts[i] = dp.OnlinePlayers
	}
	
	outliers := ad.DetectZScore(playerCounts)
	if len(outliers) > 0 {
		anomalies = append(anomalies, fmt.Sprintf("%d outliers detected", len(outliers)))
	}
	
	suddenChanges := ad.DetectSuddenChanges(playerCounts, 0.5)
	if len(suddenChanges) > 0 {
		anomalies = append(anomalies, fmt.Sprintf("%d sudden changes detected", len(suddenChanges)))
	}
	
	if deviation > 0.5 {
		anomalies = append(anomalies, fmt.Sprintf("High deviation from network: %.1f%%", deviation*100))
	}
	
	return ValidationResult{
		IsValid:   len(anomalies) == 0,
		Anomalies: anomalies,
		Deviation: deviation,
	}
}

// ValidationResult 验证结果
type ValidationResult struct {
	IsValid   bool
	Anomalies []string
	Deviation float64
}
