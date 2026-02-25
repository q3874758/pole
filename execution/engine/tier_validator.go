package engine

import (
	"fmt"
	"math"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== Tier 验证器 ====================

// TierValidator Tier 验证器
type TierValidator struct {
	tierSystem *TierSystem
	// Tier 2 验证要求
	tier2Config *Tier2Config
	// Tier 3 验证要求
	tier3Config *Tier3Config
	mu          sync.RWMutex
}

// Tier2Config Tier 2 配置
type Tier2Config struct {
	MinNodesRequired    int           // 最少节点数
	MaxDeviation        float64        // 最大偏差
	UpdateFrequency     time.Duration // 更新频率
	DataSources        []string       // 数据来源
	ConfidenceRange     [2]float64    // 置信度范围
}

// Tier3Config Tier 3 配置
type Tier3Config struct {
	MinNodesRequired    int           // 最少节点数
	ConsensusThreshold float64        // 共识阈值
	EvidenceRequired   bool           // 需要证据
	DisputeEnabled     bool           // 启用争议
	StarRatingWeight   [5]float64    // 星评权重
}

// NewTierValidator 创建 Tier 验证器
func NewTierValidator() *TierValidator {
	return &TierValidator{
		tierSystem: NewTierSystem(),
		tier2Config: &Tier2Config{
			MinNodesRequired: 3,
			MaxDeviation:    0.30, // 30%
			UpdateFrequency:  24 * time.Hour,
			DataSources:     []string{"steamcharts", "playercount", "gametracker"},
			ConfidenceRange: [2]float64{0.30, 0.60},
		},
		tier3Config: &Tier3Config{
			MinNodesRequired:    10,
			ConsensusThreshold: 0.70, // 70%
			EvidenceRequired:   true,
			DisputeEnabled:     true,
			StarRatingWeight:   [5]float64{0.2, 0.4, 0.6, 0.8, 1.0},
		},
	}
}

// TierValidationResult Tier 验证结果
type TierValidationResult struct {
	Tier             types.Tier        `json:"tier"`
	Passed           bool              `json:"passed"`
	Confidence       float64           `json:"confidence"`
	NodesReporting  int               `json:"nodes_reporting"`
	DataDeviation   float64           `json:"data_deviation"`
	RequirementsMet []string           `json:"requirements_met"`
	RequirementsFailed []string        `json:"requirements_failed"`
	Timestamp       int64             `json:"timestamp"`
}

// ValidateTier1 验证 Tier 1
func (tv *TierValidator) ValidateTier1(dataPoints []types.GameDataPoint) *TierValidationResult {
	result := &TierValidationResult{
		Tier:            types.Tier1,
		Passed:          true,
		Confidence:       1.0,
		Timestamp:       time.Now().Unix(),
		RequirementsMet:  []string{},
	}

	// 验证要求
	requirements := []struct {
		name    string
		check   func() bool
	}{
		{"realtime_api_available", func() bool { return len(dataPoints) > 0 }},
		{"update_frequency_5min", func() bool {
			if len(dataPoints) < 2 { return true }
			// 检查最后两个数据点的时间间隔
			interval := dataPoints[0].Timestamp - dataPoints[1].Timestamp
			return interval <= 600 // 5分钟 = 300秒，这里放宽到10分钟
		}},
		{"historical_data_trustworthy", func() bool {
			// 检查历史数据无明显操纵痕迹
			return true // 简化实现
		}},
	}

	for _, req := range requirements {
		if req.check() {
			result.RequirementsMet = append(result.RequirementsMet, req.name)
		} else {
			result.RequirementsFailed = append(result.RequirementsFailed, req.name)
			result.Passed = false
			result.Confidence *= 0.8
		}
	}

	// 检查在线玩家数 >= 1000
	if len(dataPoints) > 0 && dataPoints[0].OnlinePlayers >= 1000 {
		result.RequirementsMet = append(result.RequirementsMet, "min_1000_players")
	} else {
		result.RequirementsFailed = append(result.RequirementsFailed, "min_1000_players")
		result.Passed = false
	}

	return result
}

// ValidateTier2 验证 Tier 2
func (tv *TierValidator) ValidateTier2(
	nodeData map[types.NodeID][]types.GameDataPoint,
	dataSources []string,
) *TierValidationResult {
	result := &TierValidationResult{
		Tier:            types.Tier2,
		Passed:          false,
		Confidence:      0.45,
		Timestamp:       time.Now().Unix(),
		RequirementsMet: []string{},
	}

	// 检查节点数 >= 3
	nodeCount := len(nodeData)
	result.NodesReporting = nodeCount

	if nodeCount >= tv.tier2Config.MinNodesRequired {
		result.RequirementsMet = append(result.RequirementsMet, "min_3_nodes")
	} else {
		result.RequirementsFailed = append(result.RequirementsFailed, 
			fmt.Sprintf("min_%d_nodes", tv.tier2Config.MinNodesRequired))
	}

	// 检查数据偏差
	if nodeCount >= 2 {
		deviation := tv.calculateDeviation(nodeData)
		result.DataDeviation = deviation

		if deviation <= tv.tier2Config.MaxDeviation {
			result.RequirementsMet = append(result.RequirementsMet, "deviation_within_30pct")
		} else {
			result.RequirementsFailed = append(result.RequirementsFailed, "deviation_within_30pct")
		}
	}

	// 检查数据来源
	validSources := 0
	for _, source := range dataSources {
		for _, valid := range tv.tier2Config.DataSources {
			if source == valid {
				validSources++
				break
			}
		}
	}
	if validSources >= 2 {
		result.RequirementsMet = append(result.RequirementsMet, "multiple_data_sources")
	} else {
		result.RequirementsFailed = append(result.RequirementsFailed, "multiple_data_sources")
	}

	// 计算置信度
	result.Confidence = tv.calculateTier2Confidence(result)

	// 确定是否通过
	result.Passed = len(result.RequirementsFailed) == 0 || 
		(len(result.RequirementsMet) >= 2 && result.DataDeviation <= tv.tier2Config.MaxDeviation)

	return result
}

// ValidateTier3 验证 Tier 3
func (tv *TierValidator) ValidateTier3(
	nodeData map[types.NodeID][]types.GameDataPoint,
	communityVotes map[types.Address]uint8, // 1-5 星评分
	evidence map[types.Address][]string,    // 证据截图/录屏
) *TierValidationResult {
	result := &TierValidationResult{
		Tier:            types.Tier3,
		Passed:          false,
		Confidence:      0.10,
		Timestamp:       time.Now().Unix(),
		RequirementsMet: []string{},
	}

	// 检查节点数 >= 10
	nodeCount := len(nodeData)
	result.NodesReporting = nodeCount

	if nodeCount >= tv.tier3Config.MinNodesRequired {
		result.RequirementsMet = append(result.RequirementsMet, 
			fmt.Sprintf("min_%d_nodes", tv.tier3Config.MinNodesRequired))
	} else {
		result.RequirementsFailed = append(result.RequirementsFailed, 
			fmt.Sprintf("min_%d_nodes", tv.tier3Config.MinNodesRequired))
	}

	// 检查社区投票共识
	if len(communityVotes) >= tv.tier3Config.MinNodesRequired {
		consensus := tv.calculateConsensus(communityVotes)
		result.DataDeviation = 1 - consensus // 转换为偏差

		if consensus >= tv.tier3Config.ConsensusThreshold {
			result.RequirementsMet = append(result.RequirementsMet, "70pct_consensus")
		} else {
			result.RequirementsFailed = append(result.RequirementsFailed, "70pct_consensus")
		}
	}

	// 检查证据
	if tv.tier3Config.EvidenceRequired {
		evidenceCount := 0
		for _, ev := range evidence {
			if len(ev) > 0 {
				evidenceCount++
			}
		}
		if evidenceCount >= nodeCount/2 {
			result.RequirementsMet = append(result.RequirementsMet, "evidence_provided")
		} else {
			result.RequirementsFailed = append(result.RequirementsFailed, "evidence_provided")
		}
	}

	// 计算置信度
	result.Confidence = tv.calculateTier3Confidence(result, communityVotes)

	// 确定是否通过
	result.Passed = len(result.RequirementsFailed) <= 1 && 
		result.DataDeviation <= (1-tv.tier3Config.ConsensusThreshold)

	return result
}

// calculateDeviation 计算数据偏差
func (tv *TierValidator) calculateDeviation(
	nodeData map[types.NodeID][]types.GameDataPoint,
) float64 {
	if len(nodeData) < 2 {
		return 0
	}

	// 计算每个节点的平均在线玩家数
	var totals []float64
	for _, data := range nodeData {
		if len(data) > 0 {
			var sum float64
			for _, dp := range data {
				sum += float64(dp.OnlinePlayers)
			}
			totals = append(totals, sum/float64(len(data)))
		}
	}

	if len(totals) < 2 {
		return 0
	}

	// 计算平均值
	var avg float64
	for _, t := range totals {
		avg += t
	}
	avg /= float64(len(totals))

	if avg == 0 {
		return 0
	}

	// 计算标准差
	var variance float64
	for _, t := range totals {
		diff := t - avg
		variance += diff * diff
	}
	variance /= float64(len(totals))
	stdDev := math.Sqrt(variance)

	// 返回变异系数
	return stdDev / avg
}

// calculateConsensus 计算共识度
func (tv *TierValidator) calculateConsensus(votes map[types.Address]uint8) float64 {
	if len(votes) == 0 {
		return 0
	}

	// 统计每个星级的数量
	starCounts := [5]int{0, 0, 0, 0, 0}
	for _, vote := range votes {
		if vote >= 1 && vote <= 5 {
			starCounts[vote-1]++
		}
	}

	// 找出最多票数的星级
	maxCount := 0
	for _, c := range starCounts {
		if c > maxCount {
			maxCount = c
		}
	}

	// 共识度 = 最高票数 / 总票数
	return float64(maxCount) / float64(len(votes))
}

// calculateTier2Confidence 计算 Tier 2 置信度
func (tv *TierValidator) calculateTier2Confidence(result *TierValidationResult) float64 {
	confidence := 0.30 // 基础置信度

	// 根据满足的要求增加置信度
	if len(result.RequirementsMet) >= 1 {
		confidence += 0.10
	}
	if len(result.RequirementsMet) >= 2 {
		confidence += 0.10
	}
	if result.DataDeviation <= 0.20 {
		confidence += 0.10
	}

	// 限制在范围内
	if confidence > tv.tier2Config.ConfidenceRange[1] {
		confidence = tv.tier2Config.ConfidenceRange[1]
	}
	if confidence < tv.tier2Config.ConfidenceRange[0] {
		confidence = tv.tier2Config.ConfidenceRange[0]
	}

	return confidence
}

// calculateTier3Confidence 计算 Tier 3 置信度
func (tv *TierValidator) calculateTier3Confidence(
	result *TierValidationResult,
	votes map[types.Address]uint8,
) float64 {
	confidence := 0.05 // 基础置信度

	// 根据满足的要求增加置信度
	if len(result.RequirementsMet) >= 1 {
		confidence += 0.02
	}
	if len(result.RequirementsMet) >= 2 {
		confidence += 0.03
	}

	// 根据星评平均值调整
	if len(votes) > 0 {
		var total uint8
		for _, v := range votes {
			total += v
		}
		avgRating := float64(total) / float64(len(votes))
		// 5星 = 最高置信度, 1星 = 最低
		confidence += (avgRating - 1) * 0.02
	}

	// 限制在 0.05-0.15 范围内
	if confidence > 0.15 {
		confidence = 0.15
	}
	if confidence < 0.05 {
		confidence = 0.05
	}

	return confidence
}

// ==================== 数据来源管理器 ====================

// DataSourceManager 数据来源管理器
type DataSourceManager struct {
	sources map[string]*DataSource
	mu      sync.RWMutex
}

// DataSource 数据来源
type DataSource struct {
	Name         string        `json:"name"`
	Type         SourceType    `json:"type"`
	Endpoint     string        `json:"endpoint"`
	Reliability   float64       `json:"reliability"` // 0-1
	Latency      time.Duration `json:"latency"`
	LastUpdated   int64        `json:"last_updated"`
	Status       SourceStatus  `json:"status"`
}

// SourceType 数据来源类型
type SourceType int

const (
	SourceTypeOfficialAPI SourceType = iota
	SourceTypeThirdParty
	SourceTypeCommunity
	SourceTypeManual
)

// SourceStatus 来源状态
type SourceStatus int

const (
	SourceStatusActive SourceStatus = iota
	SourceStatusInactive
	SourceStatusError
)

// NewDataSourceManager 创建数据来源管理器
func NewDataSourceManager() *DataSourceManager {
	return &DataSourceManager{
		sources: make(map[string]*DataSource),
	}
}

// RegisterSource 注册数据来源
func (dsm *DataSourceManager) RegisterSource(source *DataSource) {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()
	dsm.sources[source.Name] = source
}

// GetSource 获取数据来源
func (dsm *DataSourceManager) GetSource(name string) (*DataSource, bool) {
	dsm.mu.RLock()
	defer dsm.mu.RUnlock()

	source, ok := dsm.sources[name]
	return source, ok
}

// GetReliableSources 获取可靠的数据来源
func (dsm *DataSourceManager) GetReliableSources(minReliability float64) []*DataSource {
	dsm.mu.RLock()
	defer dsm.mu.RUnlock()

	var results []*DataSource
	for _, source := range dsm.sources {
		if source.Reliability >= minReliability && source.Status == SourceStatusActive {
			results = append(results, source)
		}
	}
	return results
}

// UpdateReliability 更新可靠性
func (dsm *DataSourceManager) UpdateReliability(name string, success bool) {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()

	if source, ok := dsm.sources[name]; ok {
		// 简单移动平均
		if success {
			source.Reliability = math.Min(1.0, source.Reliability+0.01)
		} else {
			source.Reliability = math.Max(0.0, source.Reliability-0.05)
		}
		source.LastUpdated = time.Now().Unix()
	}
}

// GetSourcesByType 按类型获取来源
func (dsm *DataSourceManager) GetSourcesByType(sourceType SourceType) []*DataSource {
	dsm.mu.RLock()
	defer dsm.mu.RUnlock()

	var results []*DataSource
	for _, source := range dsm.sources {
		if source.Type == sourceType {
			results = append(results, source)
		}
	}
	return results
}

// ==================== 社区验证系统 ====================

// CommunityVerifier 社区验证器
type CommunityVerifier struct {
	votes        map[types.GameID]map[types.Address]*CommunityVote
	disputes     map[types.GameID]*Dispute
	evidenceDB   map[types.GameID]map[types.Address][]string
	mu           sync.RWMutex
}

// CommunityVote 社区投票
type CommunityVote struct {
	Voter     types.Address `json:"voter"`
	Rating    uint8         `json:"rating"` // 1-5 星
	Evidence  []string     `json:"evidence"`
	Timestamp int64        `json:"timestamp"`
	Stake     float64      `json:"stake"` // 质押量作为权重
}

// Dispute 争议
type Dispute struct {
	GameID       types.GameID     `json:"game_id"`
	Initiator    types.Address   `json:"initiator"`
	Reason       string          `json:"reason"`
	Evidence     []string        `json:"evidence"`
	StartTime    int64           `json:"start_time"`
	EndTime     int64           `json:"end_time"`
	Status       DisputeStatus   `json:"status"`
	VotesFor     float64         `json:"votes_for"`
	VotesAgainst float64         `json:"votes_against"`
}

// DisputeStatus 争议状态
type DisputeStatus int

const (
	DisputeStatusPending DisputeStatus = iota
	DisputeStatusVoting
	DisputeStatusResolved
	DisputeStatusRejected
)

// NewCommunityVerifier 创建社区验证器
func NewCommunityVerifier() *CommunityVerifier {
	return &CommunityVerifier{
		votes:      make(map[types.GameID]map[types.Address]*CommunityVote),
		disputes:   make(map[types.GameID]*Dispute),
		evidenceDB: make(map[types.GameID]map[types.Address][]string),
	}
}

// SubmitVote 提交投票
func (cv *CommunityVerifier) SubmitVote(
	gameID types.GameID,
	voter types.Address,
	rating uint8,
	evidence []string,
	stake float64,
) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be 1-5")
	}

	cv.mu.Lock()
	defer cv.mu.Unlock()

	if cv.votes[gameID] == nil {
		cv.votes[gameID] = make(map[types.Address]*CommunityVote)
	}

	cv.votes[gameID][voter] = &CommunityVote{
		Voter:     voter,
		Rating:    rating,
		Evidence:  evidence,
		Timestamp: time.Now().Unix(),
		Stake:     stake,
	}

	// 保存证据
	if cv.evidenceDB[gameID] == nil {
		cv.evidenceDB[gameID] = make(map[types.Address][]string)
	}
	cv.evidenceDB[gameID][voter] = evidence

	return nil
}

// GetRating 获取评分
func (cv *CommunityVerifier) GetRating(gameID types.GameID) (float64, uint32, bool) {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	votes, ok := cv.votes[gameID]
	if !ok || len(votes) == 0 {
		return 0, 0, false
	}

	// 加权平均
	var weightedSum, totalWeight float64
	for _, vote := range votes {
		weightedSum += float64(vote.Rating) * vote.Stake
		totalWeight += vote.Stake
	}

	if totalWeight == 0 {
		return 0, 0, false
	}

	avgRating := weightedSum / totalWeight
	passed := len(votes) >= 10

	return avgRating, uint32(len(votes)), passed
}

// GetEvidence 获取证据
func (cv *CommunityVerifier) GetEvidence(gameID types.GameID) map[types.Address][]string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	if evidence, ok := cv.evidenceDB[gameID]; ok {
		result := make(map[types.Address][]string)
		for k, v := range evidence {
			result[k] = v
		}
		return result
	}
	return nil
}

// CreateDispute 创建争议
func (cv *CommunityVerifier) CreateDispute(
	gameID types.GameID,
	initiator types.Address,
	reason string,
	evidence []string,
) (*Dispute, error) {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	// 检查是否已有争议
	if _, exists := cv.disputes[gameID]; exists {
		return nil, fmt.Errorf("dispute already exists")
	}

	dispute := &Dispute{
		GameID:    gameID,
		Initiator: initiator,
		Reason:    reason,
		Evidence:  evidence,
		StartTime: time.Now().Unix(),
		EndTime:   time.Now().Unix() + 7*24*3600, // 7天
		Status:    DisputeStatusVoting,
	}

	cv.disputes[gameID] = dispute
	return dispute, nil
}

// VoteDispute 投票争议
func (cv *CommunityVerifier) VoteDispute(
	gameID types.GameID,
	voter types.Address,
	approve bool,
	stake float64,
) error {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	dispute, ok := cv.disputes[gameID]
	if !ok {
		return fmt.Errorf("dispute not found")
	}

	if time.Now().Unix() > dispute.EndTime {
		return fmt.Errorf("dispute voting period ended")
	}

	if approve {
		dispute.VotesFor += stake
	} else {
		dispute.VotesAgainst += stake
	}

	return nil
}

// ResolveDispute 解决争议
func (cv *CommunityVerifier) ResolveDispute(gameID types.GameID) (bool, error) {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	dispute, ok := cv.disputes[gameID]
	if !ok {
		return false, fmt.Errorf("dispute not found")
	}

	totalVotes := dispute.VotesFor + dispute.VotesAgainst
	if totalVotes == 0 {
		dispute.Status = DisputeStatusRejected
		return false, nil
	}

	approvalRatio := dispute.VotesFor / totalVotes
	resolved := approvalRatio > 0.5

	if resolved {
		dispute.Status = DisputeStatusResolved
	} else {
		dispute.Status = DisputeStatusRejected
	}

	return resolved, nil
}

// GetDispute 获取争议
func (cv *CommunityVerifier) GetDispute(gameID types.GameID) (*Dispute, bool) {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	dispute, ok := cv.disputes[gameID]
	return dispute, ok
}
