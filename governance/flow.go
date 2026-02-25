package governance

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ==================== 治理阶段 ====================

// GovernancePhase 治理阶段
type GovernancePhase string

const (
	PhaseV1 GovernancePhase = "V1" // 主网上线后 12 个月：团队主导
	PhaseV2 GovernancePhase = "V2" // 12-24 个月：混合治理
	PhaseV3 GovernancePhase = "V3" // 24 个月后：完全 DAO
)

// ==================== 提案分类与阈值（白皮书 5.2）====================

// ProposalCategory 提案分类
type ProposalCategory string

const (
	CategoryMinorParams   ProposalCategory = "MinorParams"   // 轻微参数调整
	CategoryMediumGov     ProposalCategory = "MediumGov"     // 中等治理事项
	CategoryMajorProtocol ProposalCategory = "MajorProtocol" // 重大协议变更
	CategoryTreasurySpend ProposalCategory = "TreasurySpend" // 国库资金使用
)

// ProposalThreshold 提案阈值配置
type ProposalThreshold struct {
	MinStake        decimal.Decimal // 最低质押门槛 (POLE)
	ApprovalBps     int             // 投票通过阈值 (bps, 5000=50%)
	DepositPeriod   int64           // 公示期 (秒)
	VotingPeriod    int64           // 投票期 (秒)
	ExecutionPeriod int64           // 执行期 (秒)
}

// DefaultThresholds 默认阈值（白皮书表格）
func DefaultThresholds() map[ProposalCategory]ProposalThreshold {
	min10k, _ := decimal.NewFromString("10000000000000000000000")   // 10000 POLE
	min20k, _ := decimal.NewFromString("20000000000000000000000")   // 20000
	min50k, _ := decimal.NewFromString("50000000000000000000000")   // 50000
	min100k, _ := decimal.NewFromString("100000000000000000000000") // 100000
	return map[ProposalCategory]ProposalThreshold{
		CategoryMinorParams: {
			MinStake:        min10k,
			ApprovalBps:     5001, // >50%
			DepositPeriod:   5 * 24 * 3600,
			VotingPeriod:    5 * 24 * 3600,
			ExecutionPeriod: 7 * 24 * 3600,
		},
		CategoryMediumGov: {
			MinStake:        min50k,
			ApprovalBps:     6001, // >60%
			DepositPeriod:   7 * 24 * 3600,
			VotingPeriod:    5 * 24 * 3600,
			ExecutionPeriod: 14 * 24 * 3600,
		},
		CategoryMajorProtocol: {
			MinStake:        min100k,
			ApprovalBps:     7501, // >75%
			DepositPeriod:   14 * 24 * 3600,
			VotingPeriod:    7 * 24 * 3600,
			ExecutionPeriod: 14 * 24 * 3600,
		},
		CategoryTreasurySpend: {
			MinStake:        min20k,
			ApprovalBps:     6001, // >60%
			DepositPeriod:   7 * 24 * 3600,
			VotingPeriod:    5 * 24 * 3600,
			ExecutionPeriod: 7 * 24 * 3600,
		},
	}
}

// ==================== 治理流程 ====================

// ProposalStage 提案阶段
type ProposalStage string

const (
	StageDeposit   ProposalStage = "Deposit"   // 公示期
	StageVoting    ProposalStage = "Voting"   // 投票期
	StagePassed    ProposalStage = "Passed"   // 已通过
	StageRejected  ProposalStage = "Rejected" // 已拒绝
	StageExecution ProposalStage = "Execution" // 执行期
	StageExecuted  ProposalStage = "Executed"  // 已执行
)

// GovernanceFlow 治理流程
type GovernanceFlow struct {
	thresholds map[ProposalCategory]ProposalThreshold
	phase      GovernancePhase
	mu         sync.RWMutex
}

// NewGovernanceFlow 创建治理流程
func NewGovernanceFlow() *GovernanceFlow {
	return &GovernanceFlow{
		thresholds: DefaultThresholds(),
		phase:      PhaseV1,
	}
}

// GetThreshold 获取某类提案的阈值
func (gf *GovernanceFlow) GetThreshold(cat ProposalCategory) (ProposalThreshold, bool) {
	gf.mu.RLock()
	defer gf.mu.RUnlock()
	t, ok := gf.thresholds[cat]
	return t, ok
}

// SetPhase 设置治理阶段
func (gf *GovernanceFlow) SetPhase(phase GovernancePhase) {
	gf.mu.Lock()
	defer gf.mu.Unlock()
	gf.phase = phase
}

// GetPhase 获取当前阶段
func (gf *GovernanceFlow) GetPhase() GovernancePhase {
	gf.mu.RLock()
	defer gf.mu.RUnlock()
	return gf.phase
}

// ValidateProposal 验证提案是否满足门槛
func (gf *GovernanceFlow) ValidateProposal(cat ProposalCategory, stake decimal.Decimal) error {
	t, ok := gf.GetThreshold(cat)
	if !ok {
		return fmt.Errorf("unknown proposal category: %s", cat)
	}
	if stake.LessThan(t.MinStake) {
		return fmt.Errorf("stake below minimum: need %s", t.MinStake.String())
	}
	return nil
}

// CheckPassed 检查投票是否通过
func (gf *GovernanceFlow) CheckPassed(cat ProposalCategory, yesBps int) bool {
	t, ok := gf.GetThreshold(cat)
	if !ok {
		return false
	}
	return yesBps >= t.ApprovalBps
}

// GetDepositPeriod 获取公示期
func (gf *GovernanceFlow) GetDepositPeriod(cat ProposalCategory) int64 {
	t, ok := gf.GetThreshold(cat)
	if !ok {
		return 7 * 24 * 3600
	}
	return t.DepositPeriod
}

// GetVotingPeriod 获取投票期
func (gf *GovernanceFlow) GetVotingPeriod(cat ProposalCategory) int64 {
	t, ok := gf.GetThreshold(cat)
	if !ok {
		return 5 * 24 * 3600
	}
	return t.VotingPeriod
}

// GetExecutionPeriod 获取执行期
func (gf *GovernanceFlow) GetExecutionPeriod(cat ProposalCategory) int64 {
	t, ok := gf.GetThreshold(cat)
	if !ok {
		return 14 * 24 * 3600
	}
	return t.ExecutionPeriod
}

// AdvanceStage 推进阶段
func (gf *GovernanceFlow) AdvanceStage(current ProposalStage, cat ProposalCategory, createdAt int64) (ProposalStage, int64) {
	now := time.Now().Unix()
	t, _ := gf.GetThreshold(cat)

	switch current {
	case StageDeposit:
		if now >= createdAt+t.DepositPeriod {
			return StageVoting, createdAt + t.DepositPeriod
		}
	case StageVoting:
		if now >= createdAt+t.DepositPeriod+t.VotingPeriod {
			return StagePassed, 0 // 实际需根据投票结果
		}
	case StageExecution:
		if now >= createdAt+t.DepositPeriod+t.VotingPeriod+t.ExecutionPeriod {
			return StageExecuted, 0
		}
	}
	return current, 0
}
