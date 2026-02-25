package security

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ProgressivePenaltyConfig 渐进式惩罚配置
type ProgressivePenaltyConfig struct {
	WarningThreshold   int             // 触发警告的连续违规次数
	MinorPenalty       decimal.Decimal // 轻微惩罚比例（首次违规）
	ModeratePenalty    decimal.Decimal // 中等惩罚比例（2-3 次违规）
	SeverePenalty     decimal.Decimal // 严重惩罚比例（4+ 次违规）
	MaxPenalty         decimal.Decimal // 最大惩罚比例上限
	ResetInterval      time.Duration   // 违规计数重置间隔
	GracePeriod        time.Duration   // 首次违规的宽限期（不计惩罚）
}

func DefaultProgressivePenaltyConfig() *ProgressivePenaltyConfig {
	return &ProgressivePenaltyConfig{
		WarningThreshold: 2,
		MinorPenalty:     decimal.NewFromFloat(0.01),   // 1%
		ModeratePenalty:  decimal.NewFromFloat(0.05),   // 5%
		SeverePenalty:   decimal.NewFromFloat(0.10),   // 10%
		MaxPenalty:      decimal.NewFromFloat(0.50),   // 50%
		ResetInterval:   24 * time.Hour,               // 24 小时无违规重置
		GracePeriod:      7 * 24 * time.Hour,           // 7 天宽限期
	}
}

// Violation 违规记录
type Violation struct {
	Timestamp time.Time       `json:"timestamp"` // 违规时间
	Reason    string          `json:"reason"`    // 违规原因
	Severity  string          `json:"severity"`  // 严重程度：minor/moderate/severe
	Penalty   decimal.Decimal `json:"penalty"`   // 本次惩罚比例
}

// ValidatorPenaltyState 验证者惩罚状态
type ValidatorPenaltyState struct {
	Address          string        `json:"address"`           // 验证者地址
	ViolationCount  int           `json:"violation_count"`  // 连续违规次数
	TotalPenalty    decimal.Decimal `json:"total_penalty"`  // 累计惩罚比例
	LastViolationAt time.Time     `json:"last_violation_at"` // 上次违规时间
	Violations      []Violation   `json:"violations"`       // 详细违规记录
	IsWarned        bool          `json:"is_warned"`        // 是否已警告
	IsJailed        bool          `json:"is_jailed"`        // 是否已监禁
	FirstSeenAt     time.Time     `json:"first_seen_at"`    // 首次出现时间
	mu              sync.RWMutex
}

// ProgressivePenaltyManager 渐进式惩罚管理器
type ProgressivePenaltyManager struct {
	config  *ProgressivePenaltyConfig
	states  map[string]*ValidatorPenaltyState
	mu      sync.RWMutex
}

// NewProgressivePenaltyManager 创建渐进式惩罚管理器
func NewProgressivePenaltyManager(cfg *ProgressivePenaltyConfig) *ProgressivePenaltyManager {
	if cfg == nil {
		cfg = DefaultProgressivePenaltyConfig()
	}
	return &ProgressivePenaltyManager{
		config: cfg,
		states: make(map[string]*ValidatorPenaltyState),
	}
}

// RecordViolation 记录违规并计算惩罚
func (ppm *ProgressivePenaltyManager) RecordViolation(addr, reason, severity string) (*ValidatorPenaltyState, decimal.Decimal, error) {
	ppm.mu.Lock()
	defer ppm.mu.Unlock()

	state, ok := ppm.states[addr]
	if !ok {
		state = &ValidatorPenaltyState{
			Address:      addr,
			FirstSeenAt:  time.Now(),
			Violations:   make([]Violation, 0),
		}
		ppm.states[addr] = state
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	// 检查是否需要重置违规计数
	if state.LastViolationAt.Add(ppm.config.ResetInterval).Before(time.Now()) {
		state.ViolationCount = 0
		state.TotalPenalty = decimal.Zero
	}

	// 计算惩罚比例
	penalty := ppm.calculatePenalty(state.ViolationCount + 1)

	// 记录违规
	violation := Violation{
		Timestamp: time.Now(),
		Reason:    reason,
		Severity:  severity,
		Penalty:   penalty,
	}
	state.Violations = append(state.Violations, violation)

	// 更新状态
	state.ViolationCount++
	state.TotalPenalty = state.TotalPenalty.Add(penalty)
	state.LastViolationAt = time.Now()

	// 检查是否超过最大惩罚
	if state.TotalPenalty.GreaterThan(ppm.config.MaxPenalty) {
		state.TotalPenalty = ppm.config.MaxPenalty
	}

	// 检查是否需要警告或监禁
	state.IsWarned = state.ViolationCount >= ppm.config.WarningThreshold

	return state, penalty, nil
}

// calculatePenalty 根据违规次数计算惩罚比例
func (ppm *ProgressivePenaltyManager) calculatePenalty(count int) decimal.Decimal {
	switch {
	case count == 1:
		return ppm.config.MinorPenalty
	case count <= 3:
		return ppm.config.ModeratePenalty
	default:
		return ppm.config.SeverePenalty
	}
}

// ShouldJail 检查是否应该监禁
func (ppm *ProgressivePenaltyManager) ShouldJail(addr string) bool {
	ppm.mu.RLock()
	defer ppm.mu.RUnlock()

	state, ok := ppm.states[addr]
	if !ok {
		return false
	}

	state.mu.RLock()
	defer state.mu.RUnlock()

	// 严重违规 4 次以上应该监禁
	return state.ViolationCount >= 4
}

// GetState 获取验证者惩罚状态
func (ppm *ProgressivePenaltyManager) GetState(addr string) (*ValidatorPenaltyState, bool) {
	ppm.mu.RLock()
	defer ppm.mu.RUnlock()

	state, ok := ppm.states[addr]
	if !ok {
		return nil, false
	}

	state.mu.RLock()
	defer state.mu.RUnlock()

	return &ValidatorPenaltyState{
		Address:          state.Address,
		ViolationCount:  state.ViolationCount,
		TotalPenalty:    state.TotalPenalty,
		LastViolationAt: state.LastViolationAt,
		IsWarned:        state.IsWarned,
		IsJailed:        state.IsJailed,
		FirstSeenAt:     state.FirstSeenAt,
		// 不返回详细违规记录以避免锁竞争
	}, true
}

// GetPenalty 获取当前惩罚比例
func (ppm *ProgressivePenaltyManager) GetPenalty(addr string) decimal.Decimal {
	state, ok := ppm.GetState(addr)
	if !ok {
		return decimal.Zero
	}
	return state.TotalPenalty
}

// ClearViolation 清除违规记录（治理决定）
func (ppm *ProgressivePenaltyManager) ClearViolation(addr string) error {
	ppm.mu.Lock()
	defer ppm.mu.Unlock()

	state, ok := ppm.states[addr]
	if !ok {
		return fmt.Errorf("validator not found: %s", addr)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.ViolationCount = 0
	state.TotalPenalty = decimal.Zero
	state.IsWarned = false
	state.IsJailed = false

	return nil
}

// GetAllStates 获取所有验证者惩罚状态
func (ppm *ProgressivePenaltyManager) GetAllStates() map[string]*ValidatorPenaltyState {
	ppm.mu.RLock()
	defer ppm.mu.RUnlock()

	result := make(map[string]*ValidatorPenaltyState)
	for addr, state := range ppm.states {
		state.mu.RLock()
		result[addr] = &ValidatorPenaltyState{
			Address:          state.Address,
			ViolationCount:  state.ViolationCount,
			TotalPenalty:    state.TotalPenalty,
			LastViolationAt: state.LastViolationAt,
			IsWarned:        state.IsWarned,
			IsJailed:        state.IsJailed,
			FirstSeenAt:     state.FirstSeenAt,
		}
		state.mu.RUnlock()
	}
	return result
}
