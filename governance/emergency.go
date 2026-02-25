package governance

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ==================== 紧急暂停机制（白皮书 6.3）====================

// PauseReason 暂停原因
type PauseReason string

const (
	PauseReasonSecurity   PauseReason = "Security"   // 安全漏洞
	PauseReasonAttack    PauseReason = "Attack"    // 网络攻击
	PauseReasonCriticalBug PauseReason = "CriticalBug" // 关键 bug
	PauseReasonGovernance PauseReason = "Governance" // 治理决策
)

// PauseScope 暂停范围
type PauseScope string

const (
	ScopeFull      PauseScope = "Full"       // 完全暂停
	ScopeTransfers PauseScope = "Transfers" // 仅暂停转账
	ScopeStaking   PauseScope = "Staking"   // 仅暂停质押
	ScopeGovernance PauseScope = "Governance" // 仅暂停治理
)

// EmergencyPause 紧急暂停状态
type EmergencyPause struct {
	IsPaused    bool        `json:"is_paused"`    // 是否暂停
	Scope       PauseScope  `json:"scope"`        // 暂停范围
	Reason      PauseReason `json:"reason"`      // 暂停原因
	PausedAt    int64       `json:"paused_at"`    // 暂停时间
	PausedBy    string      `json:"paused_by"`    // 暂停操作者
	ResumeAt    int64       `json:"resume_at"`   // 计划恢复时间
	ApprovedBy  string      `json:"approved_by"`  // 审批治理提案 ID（可选）
	ExtraData   string      `json:"extra_data"`   // 额外数据
}

// EmergencyPauseManager 紧急暂停管理器
type EmergencyPauseManager struct {
	mu          sync.RWMutex
	current     *EmergencyPause
	history     []*EmergencyPause
	config      *EmergencyPauseConfig
}

// EmergencyPauseConfig 紧急暂停配置
type EmergencyPauseConfig struct {
	MaxPauseDuration   time.Duration // 最大暂停时长
	MinGovernanceDelay time.Duration // 治理审批最短延迟
	AutoResumeEnabled  bool          // 是否支持自动恢复
	QuorumThreshold   decimal.Decimal // 暂停通过的治理门槛
}

func DefaultEmergencyPauseConfig() *EmergencyPauseConfig {
	return &EmergencyPauseConfig{
		MaxPauseDuration:   7 * 24 * time.Hour, // 最长 7 天
		MinGovernanceDelay: 1 * time.Hour,      // 至少 1 小时审批延迟
		AutoResumeEnabled:  true,
		QuorumThreshold:   decimal.NewFromFloat(0.67), // 67% 通过
	}
}

// NewEmergencyPauseManager 创建紧急暂停管理器
func NewEmergencyPauseManager(cfg *EmergencyPauseConfig) *EmergencyPauseManager {
	if cfg == nil {
		cfg = DefaultEmergencyPauseConfig()
	}
	return &EmergencyPauseManager{
		config:  cfg,
		current: nil,
		history: make([]*EmergencyPause, 0),
	}
}

// TriggerPause 触发紧急暂停（治理委员会或紧急按钮）
func (epm *EmergencyPauseManager) TriggerPause(scope PauseScope, reason PauseReason, pausedBy string, duration time.Duration) (*EmergencyPause, error) {
	epm.mu.Lock()
	defer epm.mu.Unlock()

	// 如果已经暂停，不能再次暂停（除非是扩大范围）
	if epm.current != nil && epm.current.IsPaused && scope == ScopeFull {
		return nil, fmt.Errorf("already paused")
	}

	now := time.Now()
	resumeAt := now.Add(duration).Unix()
	if duration > epm.config.MaxPauseDuration {
		resumeAt = now.Add(epm.config.MaxPauseDuration).Unix()
	}

	pause := &EmergencyPause{
		IsPaused:   true,
		Scope:      scope,
		Reason:     reason,
		PausedAt:   now.Unix(),
		PausedBy:   pausedBy,
		ResumeAt:   resumeAt,
		ExtraData:  "",
	}

	epm.current = pause
	epm.history = append(epm.history, pause)

	return pause, nil
}

// Resume 恢复网络（需要治理投票）
func (epm *EmergencyPauseManager) Resume(resumedBy string, approvedProposalID string) error {
	epm.mu.Lock()
	defer epm.mu.Unlock()

	if epm.current == nil || !epm.current.IsPaused {
		return fmt.Errorf("not paused")
	}

	epm.current.IsPaused = false
	epm.current.ResumeAt = time.Now().Unix()
	epm.current.ApprovedBy = approvedProposalID

	return nil
}

// CheckAutoResume 检查是否需要自动恢复
func (epm *EmergencyPauseManager) CheckAutoResume() (bool, string) {
	epm.mu.RLock()
	defer epm.mu.RUnlock()

	if epm.current == nil || !epm.current.IsPaused {
		return false, ""
	}

	if !epm.config.AutoResumeEnabled {
		return false, ""
	}

	now := time.Now().Unix()
	if now >= epm.current.ResumeAt {
		return true, "auto resume triggered"
	}

	return false, ""
}

// IsPaused 检查是否暂停
func (epm *EmergencyPauseManager) IsPaused() bool {
	epm.mu.RLock()
	defer epm.mu.RUnlock()

	if epm.current == nil {
		return false
	}

	// 检查是否超时自动恢复
	if epm.current.IsPaused && epm.current.ResumeAt > 0 {
		if time.Now().Unix() >= epm.current.ResumeAt && epm.config.AutoResumeEnabled {
			return false
		}
	}

	return epm.current.IsPaused
}

// IsActionAllowed 检查某操作是否被暂停
func (epm *EmergencyPauseManager) IsActionAllowed(action string) bool {
	epm.mu.RLock()
	defer epm.mu.RUnlock()

	if epm.current == nil || !epm.current.IsPaused {
		return true
	}

	switch epm.current.Scope {
	case ScopeFull:
		return false
	case ScopeTransfers:
		return action != "transfer" && action != "send"
	case ScopeStaking:
		return action != "stake" && action != "unstake" && action != "delegate"
	case ScopeGovernance:
		return action != "propose" && action != "vote"
	}

	return true
}

// GetCurrentPause 获取当前暂停状态
func (epm *EmergencyPauseManager) GetCurrentPause() *EmergencyPause {
	epm.mu.RLock()
	defer epm.mu.RUnlock()

	if epm.current == nil {
		return nil
	}

	return &EmergencyPause{
		IsPaused:   epm.current.IsPaused,
		Scope:      epm.current.Scope,
		Reason:     epm.current.Reason,
		PausedAt:   epm.current.PausedAt,
		PausedBy:   epm.current.PausedBy,
		ResumeAt:   epm.current.ResumeAt,
		ApprovedBy: epm.current.ApprovedBy,
		ExtraData:  epm.current.ExtraData,
	}
}

// GetHistory 获取暂停历史
func (epm *EmergencyPauseManager) GetHistory() []*EmergencyPause {
	epm.mu.RLock()
	defer epm.mu.RUnlock()

	result := make([]*EmergencyPause, len(epm.history))
	copy(result, epm.history)
	return result
}

// TimeUntilResume 返回距离恢复的时间
func (epm *EmergencyPauseManager) TimeUntilResume() time.Duration {
	epm.mu.RLock()
	defer epm.mu.RUnlock()

	if epm.current == nil || !epm.current.IsPaused || epm.current.ResumeAt == 0 {
		return 0
	}

	now := time.Now().Unix()
	if now >= epm.current.ResumeAt {
		return 0
	}

	return time.Duration(epm.current.ResumeAt-now) * time.Second
}
