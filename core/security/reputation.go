package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ReputationConfig 声誉配置
type ReputationConfig struct {
	InitialScore      decimal.Decimal // 初始声誉分数 (0-100)
	MaxScore          decimal.Decimal // 最高分数
	MinScore          decimal.Decimal // 最低分数（低于此值被踢）
	UptimeWeight      float64         // 在线时间权重
	AccuracyWeight    float64         // 数据准确性权重
	SlashWeight       float64         // 惩罚权重
	RecoveryRate      float64         // 声誉恢复速率（每小时）
	RecoveryInterval  time.Duration   // 恢复间隔
	PauseThreshold    decimal.Decimal // 触发警告的阈值
	EvictionThreshold decimal.Decimal // 驱逐阈值
}

func DefaultReputationConfig() *ReputationConfig {
	return &ReputationConfig{
		InitialScore:     decimal.NewFromFloat(80.0),
		MaxScore:        decimal.NewFromFloat(100.0),
		MinScore:        decimal.NewFromFloat(0.0),
		UptimeWeight:    0.3,
		AccuracyWeight:  0.5,
		SlashWeight:     0.2,
		RecoveryRate:    1.0,   // 每小时恢复 1 分
		RecoveryInterval: 1 * time.Hour,
		PauseThreshold:  decimal.NewFromFloat(50.0),
		EvictionThreshold: decimal.NewFromFloat(20.0),
	}
}

// ReputationEvent 声誉事件
type ReputationEvent struct {
	Type        string          `json:"type"`        // UptimeGood/UptimeBad/AccuracyGood/AccuracyBad/Slash
	Timestamp   int64           `json:"timestamp"`   // 时间戳
	DeltaScore  decimal.Decimal `json:"delta_score"` // 分数变化
	Reason      string          `json:"reason"`      // 原因描述
}

// Reputation 节点声誉
type Reputation struct {
	Address       string            `json:"address"`       // 节点地址
	Score         decimal.Decimal  `json:"score"`        // 当前声誉分数
	UptimeScore   decimal.Decimal  `json:"uptime_score"`  // 在线分数
	AccuracyScore decimal.Decimal  `json:"accuracy_score"` // 准确性分数
	SlashPenalty  decimal.Decimal  `json:"slash_penalty"` // 惩罚累积扣分
	LastUpdate    int64            `json:"last_update"`   // 最后更新时间
	Events        []ReputationEvent `json:"events"`       // 历史事件
	mu            sync.RWMutex
}

// ReputationManager 声誉管理器
type ReputationManager struct {
	config      *ReputationConfig
	reputations map[string]*Reputation
	mu          sync.RWMutex
}

// NewReputationManager 创建声誉管理器
func NewReputationManager(cfg *ReputationConfig) *ReputationManager {
	if cfg == nil {
		cfg = DefaultReputationConfig()
	}
	return &ReputationManager{
		config:      cfg,
		reputations: make(map[string]*Reputation),
	}
}

// RegisterNode 注册节点
func (rm *ReputationManager) RegisterNode(addr string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.reputations[addr]; !exists {
		rm.reputations[addr] = &Reputation{
			Address:       addr,
			Score:         rm.config.InitialScore,
			UptimeScore:   rm.config.InitialScore,
			AccuracyScore: rm.config.InitialScore,
			SlashPenalty:  decimal.Zero,
			LastUpdate:    time.Now().Unix(),
			Events:        make([]ReputationEvent, 0),
		}
	}
}

// RecordUptime 记录在线事件
func (rm *ReputationManager) RecordUptime(addr string, isGood bool) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	r, ok := rm.reputations[addr]
	if !ok {
		return fmt.Errorf("node not registered: %s", addr)
	}

	delta := decimal.NewFromFloat(-2.0)
	if isGood {
		delta = decimal.NewFromFloat(2.0)
	}

	r.UptimeScore = rm.applyDelta(r.UptimeScore, delta)
	r.LastUpdate = time.Now().Unix()
	reason := "bad"
	if isGood {
		reason = "good"
	}
	r.Events = append(r.Events, ReputationEvent{
		Type:        "Uptime",
		Timestamp:   r.LastUpdate,
		DeltaScore: delta,
		Reason:     reason,
	})

	rm.recalculate(addr)
	return nil
}

// RecordAccuracy 记录数据准确性事件
func (rm *ReputationManager) RecordAccuracy(addr string, deviation decimal.Decimal) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	r, ok := rm.reputations[addr]
	if !ok {
		return fmt.Errorf("node not registered: %s", addr)
	}

	// 偏差越小越好
	delta := decimal.Zero
	if deviation.LessThan(decimal.NewFromFloat(0.1)) {
		delta = decimal.NewFromFloat(3.0) // 优秀
	} else if deviation.LessThan(decimal.NewFromFloat(0.2)) {
		delta = decimal.NewFromFloat(1.0) // 良好
	} else if deviation.GreaterThan(decimal.NewFromFloat(0.5)) {
		delta = decimal.NewFromFloat(-5.0) // 严重偏差
	} else if deviation.GreaterThan(decimal.NewFromFloat(0.3)) {
		delta = decimal.NewFromFloat(-2.0) // 中等偏差
	}

	r.AccuracyScore = rm.applyDelta(r.AccuracyScore, delta)
	r.LastUpdate = time.Now().Unix()
	r.Events = append(r.Events, ReputationEvent{
		Type:        "Accuracy",
		Timestamp:   r.LastUpdate,
		DeltaScore:  delta,
		Reason:      deviation.String(),
	})

	rm.recalculate(addr)
	return nil
}

// RecordSlash 记录惩罚事件
func (rm *ReputationManager) RecordSlash(addr string, slashPercent decimal.Decimal) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	r, ok := rm.reputations[addr]
	if !ok {
		return fmt.Errorf("node not registered: %s", addr)
	}

	// 根据惩罚百分比扣分
	delta := slashPercent.Mul(decimal.NewFromFloat(-10)) // 例如 5% -> -50分
	r.SlashPenalty = r.SlashPenalty.Sub(delta)
	r.LastUpdate = time.Now().Unix()
	r.Events = append(r.Events, ReputationEvent{
		Type:        "Slash",
		Timestamp:   r.LastUpdate,
		DeltaScore:  delta,
		Reason:      slashPercent.String() + "%",
	})

	rm.recalculate(addr)
	return nil
}

// applyDelta 应用分数变化（带边界限制）
func (rm *ReputationManager) applyDelta(score, delta decimal.Decimal) decimal.Decimal {
	result := score.Add(delta)
	if result.GreaterThan(rm.config.MaxScore) {
		return rm.config.MaxScore
	}
	if result.LessThan(rm.config.MinScore) {
		return rm.config.MinScore
	}
	return result
}

// recalculate 重新计算总分
func (rm *ReputationManager) recalculate(addr string) {
	r := rm.reputations[addr]
	r.Score = r.UptimeScore.Mul(decimal.NewFromFloat(rm.config.UptimeWeight)).
		Add(r.AccuracyScore.Mul(decimal.NewFromFloat(rm.config.AccuracyWeight))).
		Add(r.SlashPenalty.Mul(decimal.NewFromFloat(rm.config.SlashWeight)))
}

// GetScore 获取声誉分数
func (rm *ReputationManager) GetScore(addr string) (decimal.Decimal, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	r, ok := rm.reputations[addr]
	if !ok {
		return decimal.Zero, false
	}
	return r.Score, true
}

// ShouldPause 是否应该警告
func (rm *ReputationManager) ShouldPause(addr string) bool {
	score, ok := rm.GetScore(addr)
	if !ok {
		return false
	}
	return score.LessThan(rm.config.PauseThreshold)
}

// ShouldEvict 是否应该驱逐
func (rm *ReputationManager) ShouldEvict(addr string) bool {
	score, ok := rm.GetScore(addr)
	if !ok {
		return true // 未注册的节点应该被驱逐
	}
	return score.LessThan(rm.config.EvictionThreshold)
}

// GetAllReputations 获取所有声誉
func (rm *ReputationManager) GetAllReputations() map[string]*Reputation {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make(map[string]*Reputation)
	for addr, r := range rm.reputations {
		result[addr] = r
	}
	return result
}

// SaveToFile 保存到文件
func (rm *ReputationManager) SaveToFile(path string) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	data, err := json.MarshalIndent(rm.reputations, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadFromFile 从文件加载
func (rm *ReputationManager) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在时忽略
		}
		return err
	}

	return json.Unmarshal(data, &rm.reputations)
}
