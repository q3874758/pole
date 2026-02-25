package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== 数据验证器 ====================

// DataVerifier 数据验证器
type DataVerifier struct {
	config        *VerifierConfig
	anomalyEngine *AnomalyEngine
	consensus     *DataConsensus
	quorum        *QuorumManager
	mu            sync.RWMutex
}

// VerifierConfig 验证器配置
type VerifierConfig struct {
	MinDataSources     int           // 最少数据源
	MaxDeviation       float64       // 最大偏差
	ConsensusThreshold float64       // 共识阈值
	VerificationTimeout time.Duration // 验证超时
	EnableCrossVerify  bool         // 启用交叉验证
	EnableZKP         bool          // 启用零知识证明
}

// DefaultVerifierConfig 默认验证配置
func DefaultVerifierConfig() *VerifierConfig {
	return &VerifierConfig{
		MinDataSources:      3,
		MaxDeviation:       0.30, // 30%
		ConsensusThreshold: 0.67, // 2/3
		VerificationTimeout: 30 * time.Second,
		EnableCrossVerify:  true,
		EnableZKP:        false,
	}
}

// VerificationResult 验证结果
type VerificationResult struct {
	Valid           bool          `json:"valid"`
	GameID         types.GameID  `json:"game_id"`
	Confidence     float64       `json:"confidence"`
	Deviation      float64       `json:"deviation"`
	DataHash       string        `json:"data_hash"`
	SourcesCount   int           `json:"sources_count"`
	ConsensusValue uint64       `json:"consensus_value"`
	Anomalies      []Anomaly     `json:"anomalies"`
	Timestamp      int64         `json:"timestamp"`
}

// Anomaly 异常
type Anomaly struct {
	Type     string  `json:"type"`
	Severity float64 `json:"severity"`
	Details  string `json:"details"`
}

// NewDataVerifier 创建数据验证器
func NewDataVerifier(config *VerifierConfig) *DataVerifier {
	if config == nil {
		config = DefaultVerifierConfig()
	}

	return &DataVerifier{
		config:        config,
		anomalyEngine: NewAnomalyEngine(),
		consensus:     NewDataConsensus(config.ConsensusThreshold),
		quorum:       NewQuorumManager(config.MinDataSources),
	}
}

// Verify 验证数据
func (dv *DataVerifier) Verify(
	gameID types.GameID,
	nodeData map[string]*types.GameDataPoint,
) (*VerificationResult, error) {
	dv.mu.Lock()
	defer dv.mu.Unlock()

	result := &VerificationResult{
		GameID:       gameID,
		Timestamp:    time.Now().Unix(),
		Anomalies:    make([]Anomaly, 0),
		Valid:        true,
		Confidence:  1.0,
		SourcesCount: len(nodeData),
	}

	if len(nodeData) == 0 {
		result.Valid = false
		result.Anomalies = append(result.Anomalies, Anomaly{
			Type:     "no_data",
			Severity: 1.0,
			Details:  "no data provided",
		})
		return result, nil
	}

	// 计算共识值
	consensusValue, err := dv.consensus.CalculateConsensus(nodeData)
	if err != nil {
		result.Valid = false
		result.Anomalies = append(result.Anomalies, Anomaly{
			Type:     "consensus_error",
			Severity: 1.0,
			Details:  err.Error(),
		})
		return result, err
	}
	result.ConsensusValue = consensusValue

	// 计算偏差
	deviation := dv.calculateDeviation(nodeData, consensusValue)
	result.Deviation = deviation

	// 检查偏差是否超过阈值
	if deviation > dv.config.MaxDeviation {
		result.Valid = false
		result.Confidence = 1 - deviation
		result.Anomalies = append(result.Anomalies, Anomaly{
			Type:     "high_deviation",
			Severity: deviation,
			Details:  fmt.Sprintf("deviation %.2f%% exceeds threshold %.2f%%", deviation*100, dv.config.MaxDeviation*100),
		})
	}

	// 异常检测
	anomalies := dv.anomalyEngine.Detect(nodeData)
	result.Anomalies = append(result.Anomalies, anomalies...)

	if len(anomalies) > 0 {
		result.Valid = false
		result.Confidence *= 0.5
	}

	// 交叉验证
	if dv.config.EnableCrossVerify && len(nodeData) >= dv.config.MinDataSources {
		crossResult := dv.crossVerify(nodeData)
		if !crossResult.Passed {
			result.Anomalies = append(result.Anomalies, crossResult.Anomalies...)
			result.Valid = false
		}
	}

	// 计算数据哈希
	result.DataHash = dv.hashData(gameID, consensusValue, result.Timestamp)

	return result, nil
}

// calculateDeviation 计算偏差
func (dv *DataVerifier) calculateDeviation(
	data map[string]*types.GameDataPoint,
	consensusValue uint64,
) float64 {
	if consensusValue == 0 {
		return 0
	}

	var totalDeviation float64
	count := 0

	for _, dp := range data {
		if dp.OnlinePlayers == 0 {
			continue
		}

		deviation := math.Abs(float64(dp.OnlinePlayers) - float64(consensusValue)) / float64(consensusValue)
		totalDeviation += deviation
		count++
	}

	if count == 0 {
		return 0
	}

	return totalDeviation / float64(count)
}

// crossVerify 交叉验证
func (dv *DataVerifier) crossVerify(
	data map[string]*types.GameDataPoint,
) *CrossVerifyResult {
	result := &CrossVerifyResult{
		Passed:    true,
		Anomalies: make([]Anomaly, 0),
	}

	// 获取所有数据源
	sources := make([]uint64, 0, len(data))
	for _, dp := range data {
		sources = append(sources, dp.OnlinePlayers)
	}

	// 计算中位数
	median := calculateMedian(sources)

	// 检查每个数据源与中位数的偏差
	for source, dp := range data {
		deviation := math.Abs(float64(dp.OnlinePlayers) - float64(median)) / float64(median)
		if deviation > dv.config.MaxDeviation*1.5 {
			result.Passed = false
			result.Anomalies = append(result.Anomalies, Anomaly{
				Type:     "cross_verify_failed",
				Severity: deviation,
				Details:  fmt.Sprintf("source %s deviation %.2f%%", source, deviation*100),
			})
		}
	}

	return result
}

// CrossVerifyResult 交叉验证结果
type CrossVerifyResult struct {
	Passed    bool
	Anomalies []Anomaly
}

// hashData 计算数据哈希
func (dv *DataVerifier) hashData(gameID types.GameID, value uint64, timestamp int64) string {
	data := fmt.Sprintf("%s:%d:%d", gameID, value, timestamp)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// ==================== 共识引擎 ====================

// DataConsensus 数据共识
type DataConsensus struct {
	threshold float64
	mu        sync.RWMutex
}

// NewDataConsensus 创建数据共识
func NewDataConsensus(threshold float64) *DataConsensus {
	return &DataConsensus{
		threshold: threshold,
	}
}

// CalculateConsensus 计算共识值
func (dc *DataConsensus) CalculateConsensus(
	data map[string]*types.GameDataPoint,
) (uint64, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("no data")
	}

	// 提取所有在线玩家数
	values := make([]uint64, 0, len(data))
	for _, dp := range data {
		values = append(values, dp.OnlinePlayers)
	}

	// 使用中位数作为共识值
	median := calculateMedian(values)

	return median, nil
}

// calculateMedian 计算中位数
func calculateMedian(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}

	// 排序
	sorted := make([]uint64, len(values))
	copy(sorted, values)
	quickSort(sorted)

	// 取中位数
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// quickSort 快速排序
func quickSort(arr []uint64) {
	if len(arr) <= 1 {
		return
	}

	pivot := arr[len(arr)/2]
	i, j := 0, len(arr)-1

	for i <= j {
		for i <= j && arr[i] < pivot {
			i++
		}
		for i <= j && arr[j] > pivot {
			j--
		}
		if i <= j {
			arr[i], arr[j] = arr[j], arr[i]
			i++
			j--
		}
	}

	if j > 0 {
		quickSort(arr[:j])
	}
	if i < len(arr)-1 {
		quickSort(arr[i:])
	}
}

// ==================== 异常检测引擎 ====================

// AnomalyEngine 异常检测引擎
type AnomalyEngine struct {
	models map[string]*AnomalyModel
	mu     sync.RWMutex
}

// AnomalyModel 异常模型
type AnomalyModel struct {
	Type      string
	Threshold float64
	Samples   []uint64
}

// NewAnomalyEngine 创建异常引擎
func NewAnomalyEngine() *AnomalyEngine {
	return &AnomalyEngine{
		models: make(map[string]*AnomalyModel),
	}
}

// Detect 检测异常
func (ae *AnomalyEngine) Detect(
	data map[string]*types.GameDataPoint,
) []Anomaly {
	anomalies := make([]Anomaly, 0)

	// 检测瞬间突变
	if anomaly := ae.detectSuddenChange(data); anomaly != nil {
		anomalies = append(anomalies, *anomaly)
	}

	// 检测规律性模式
	if anomaly := ae.detectRegularPattern(data); anomaly != nil {
		anomalies = append(anomalies, *anomaly)
	}

	// 检测极端值
	if anomaly := ae.detectOutliers(data); anomaly != nil {
		anomalies = append(anomalies, *anomaly)
	}

	// 检测时间异常
	if anomaly := ae.detectTimeAnomaly(data); anomaly != nil {
		anomalies = append(anomalies, *anomaly)
	}

	return anomalies
}

// detectSuddenChange 检测瞬间突变
func (ae *AnomalyEngine) detectSuddenChange(data map[string]*types.GameDataPoint) *Anomaly {
	values := make([]uint64, 0, len(data))
	for _, dp := range data {
		values = append(values, dp.OnlinePlayers)
	}

	if len(values) < 2 {
		return nil
	}

	// 检查最大变化率
	maxChange := 0.0
	for i := 1; i < len(values); i++ {
		if values[i-1] == 0 {
			continue
		}
		change := math.Abs(float64(values[i]) - float64(values[i-1])) / float64(values[i-1])
		if change > maxChange {
			maxChange = change
		}
	}

	// 超过 10 倍变化视为异常
	if maxChange > 10 {
		return &Anomaly{
			Type:     "sudden_change",
			Severity: maxChange / 10,
			Details:  fmt.Sprintf("max change %.2f%%", maxChange*100),
		}
	}

	return nil
}

// detectRegularPattern 检测规律性模式
func (ae *AnomalyEngine) detectRegularPattern(data map[string]*types.GameDataPoint) *Anomaly {
	// 检查时间间隔是否过于规律
	timestamps := make([]int64, 0, len(data))
	for _, dp := range data {
		timestamps = append(timestamps, dp.Timestamp)
	}

	if len(timestamps) < 3 {
		return nil
	}

	// 检查间隔方差
	var intervals []int64
	for i := 1; i < len(timestamps); i++ {
		intervals = append(intervals, timestamps[i]-timestamps[i-1])
	}

	avg := float64(0)
	for _, iv := range intervals {
		avg += float64(iv)
	}
	avg /= float64(len(intervals))

	if avg == 0 {
		return nil
	}

	variance := float64(0)
	for _, iv := range intervals {
		diff := float64(iv) - avg
		variance += diff * diff
	}
	variance /= float64(len(intervals))

	coefficientOfVariation := math.Sqrt(variance) / avg

	// 方差小于平均值 1% 视为规律（可能是机器人）
	if coefficientOfVariation < 0.01 {
		return &Anomaly{
			Type:     "regular_pattern",
			Severity: 1 - coefficientOfVariation,
			Details:  "suspiciously regular intervals",
		}
	}

	return nil
}

// detectOutliers 检测极端值
func (ae *AnomalyEngine) detectOutliers(data map[string]*types.GameDataPoint) *Anomaly {
	values := make([]uint64, 0, len(data))
	for _, dp := range data {
		values = append(values, dp.OnlinePlayers)
	}

	if len(values) < 3 {
		return nil
	}

	// 计算 IQR
	sorted := make([]uint64, len(values))
	copy(sorted, values)
	quickSort(sorted)

	q1 := sorted[len(sorted)/4]
	q3 := sorted[len(sorted)*3/4]
	iqr := float64(q3 - q1)

	if iqr == 0 {
		return nil
	}

	lowerBound := float64(q1) - 1.5*iqr
	upperBound := float64(q3) + 1.5*iqr

	outlierCount := 0
	for _, v := range values {
		if float64(v) < lowerBound || float64(v) > upperBound {
			outlierCount++
		}
	}

	if outlierCount > len(values)/2 {
		return &Anomaly{
			Type:     "outliers",
			Severity: float64(outlierCount) / float64(len(values)),
			Details:  fmt.Sprintf("%d outliers detected", outlierCount),
		}
	}

	return nil
}

// detectTimeAnomaly 检测时间异常
func (ae *AnomalyEngine) detectTimeAnomaly(data map[string]*types.GameDataPoint) *Anomaly {
	now := time.Now().Unix()

	for source, dp := range data {
		// 检查未来时间
		if dp.Timestamp > now {
			return &Anomaly{
				Type:     "future_timestamp",
				Severity: 1.0,
				Details:  fmt.Sprintf("source %s has future timestamp", source),
			}
		}

		// 检查超过 24 小时的数据
		if now-dp.Timestamp > 86400 {
			return &Anomaly{
				Type:     "stale_data",
				Severity: 0.8,
				Details:  fmt.Sprintf("source %s data older than 24h", source),
			}
		}
	}

	return nil
}

// ==================== Quorum 管理器 ====================

// QuorumManager Quorum 管理器
type QuorumManager struct {
	minSources int
	mu         sync.RWMutex
}

// NewQuorumManager 创建 Quorum 管理器
func NewQuorumManager(minSources int) *QuorumManager {
	return &QuorumManager{
		minSources: minSources,
	}
}

// CheckQuorum 检查 Quorum
func (qm *QuorumManager) CheckQuorum(sources int) bool {
	return sources >= qm.minSources
}

// ==================== 零知识证明验证 ====================

// ZKVerifier 零知识证明验证器
type ZKVerifier struct {
	enabled bool
}

// NewZKVerifier 创建 ZK 验证器
func NewZKVerifier() *ZKVerifier {
	return &ZKVerifier{
		enabled: false, // 默认关闭
	}
}

// VerifyProof 验证证明
func (zk *ZKVerifier) VerifyProof(proof []byte, publicInput []byte) bool {
	if !zk.enabled {
		return true // 如果未启用，直接返回 true
	}

	// 简化实现
	return len(proof) > 0
}

// GenerateProof 生成证明
func (zk *ZKVerifier) GenerateProof(data []byte, witness []byte) ([]byte, error) {
	if !zk.enabled {
		return []byte{}, nil
	}

	// 简化实现
	return data, nil
}

// ==================== 批量验证 ====================

// BatchVerifier 批量验证器
type BatchVerifier struct {
	verifier *DataVerifier
}

// NewBatchVerifier 创建批量验证器
func NewBatchVerifier(config *VerifierConfig) *BatchVerifier {
	return &BatchVerifier{
		verifier: NewDataVerifier(config),
	}
}

// VerifyBatch 批量验证
func (bv *BatchVerifier) VerifyBatch(
	data map[types.GameID]map[string]*types.GameDataPoint,
) map[types.GameID]*VerificationResult {
	results := make(map[types.GameID]*VerificationResult)

	for gameID, nodeData := range data {
		result, err := bv.verifier.Verify(gameID, nodeData)
		if err != nil {
			results[gameID] = &VerificationResult{
				Valid:     false,
				GameID:    gameID,
				Timestamp: time.Now().Unix(),
				Anomalies: []Anomaly{{
					Type:     "verification_error",
					Severity: 1.0,
					Details:  err.Error(),
				}},
			}
		} else {
			results[gameID] = result
		}
	}

	return results
}

// GetValidData 获取有效数据
func (bv *BatchVerifier) GetValidData(
	results map[types.GameID]*VerificationResult,
) map[types.GameID]uint64 {
	validData := make(map[types.GameID]uint64)

	for gameID, result := range results {
		if result.Valid {
			validData[gameID] = result.ConsensusValue
		}
	}

	return validData
}

// ==================== 验证报告 ====================

// VerificationReport 验证报告
type VerificationReport struct {
	TotalChecks   int                     `json:"total_checks"`
	PassedChecks int                     `json:"passed_checks"`
	FailedChecks int                     `json:"failed_checks"`
	Results      map[string]bool          `json:"results"`
	Anomalies    []Anomaly               `json:"anomalies"`
	Timestamp    int64                   `json:"timestamp"`
}

// GenerateReport 生成报告
func (bv *BatchVerifier) GenerateReport(
	results map[types.GameID]*VerificationResult,
) *VerificationReport {
	report := &VerificationReport{
		TotalChecks: len(results),
		Results:     make(map[string]bool),
		Timestamp:   time.Now().Unix(),
		Anomalies:   make([]Anomaly, 0),
	}

	for gameID, result := range results {
		report.Results[string(gameID)] = result.Valid
		if result.Valid {
			report.PassedChecks++
		} else {
			report.FailedChecks++
			report.Anomalies = append(report.Anomalies, result.Anomalies...)
		}
	}

	return report
}

// ToJSON 转换为 JSON
func (vr *VerificationReport) ToJSON() ([]byte, error) {
	return json.Marshal(vr)
}
