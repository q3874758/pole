package governance

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"pole-core/core/types"
)

// ==================== 治理配置 ====================

// Config 治理配置
type Config struct {
	MinProposalStake   decimal.Decimal // 最小提案质押
	MinVotingPeriod    int64           // 最小投票期 (秒)
	QuorumThreshold    float64         // 法定人数阈值
	ApprovalThreshold   float64         // 批准阈值
	ExecutionDelay      int64           // 执行延迟 (秒)
}

func DefaultConfig() *Config {
	minStake, _ := decimal.NewFromString("10000000000000000000") // 10000 POLE
	return &Config{
		MinProposalStake:  minStake,
		MinVotingPeriod:   5 * 24 * 3600,  // 5 天
		QuorumThreshold:    0.40,          // 40%
		ApprovalThreshold:  0.50,          // 50%
		ExecutionDelay:     2 * 24 * 3600, // 2 天
	}
}

// ==================== 提案类型 ====================

// ProposalType 提案类型
type ProposalType int

const (
	ProposalTypeParameterChange ProposalType = iota
	ProposalTypeTextProposal
	ProposalTypeTreasurySpend
	ProposalTypeProtocolUpgrade
)

func (pt ProposalType) String() string {
	switch pt {
	case ProposalTypeParameterChange:
		return "ParameterChange"
	case ProposalTypeTextProposal:
		return "TextProposal"
	case ProposalTypeTreasurySpend:
		return "TreasurySpend"
	case ProposalTypeProtocolUpgrade:
		return "ProtocolUpgrade"
	default:
		return "Unknown"
	}
}

// ==================== 提案状态 ====================

// ProposalStatus 提案状态
type ProposalStatus int

const (
	ProposalStatusPending ProposalStatus = iota
	ProposalStatusVoting
	ProposalStatusPassed
	ProposalStatusRejected
	ProposalStatusExecuted
	ProposalStatusCancelled
)

func (ps ProposalStatus) String() string {
	switch ps {
	case ProposalStatusPending:
		return "Pending"
	case ProposalStatusVoting:
		return "Voting"
	case ProposalStatusPassed:
		return "Passed"
	case ProposalStatusRejected:
		return "Rejected"
	case ProposalStatusExecuted:
		return "Executed"
	case ProposalStatusCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// ==================== 投票选项 ====================

// VoteOption 投票选项
type VoteOption int

const (
	VoteOptionYes VoteOption = iota
	VoteOptionNo
	VoteOptionAbstain
)

func (vo VoteOption) String() string {
	switch vo {
	case VoteOptionYes:
		return "Yes"
	case VoteOptionNo:
		return "No"
	case VoteOptionAbstain:
		return "Abstain"
	default:
		return "Unknown"
	}
}

// ==================== 提案 ====================

// Proposal 提案
type Proposal struct {
	ID                uint64           `json:"id"`
	Proposer         types.Address   `json:"proposer"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	ProposalType     ProposalType     `json:"proposal_type"`
	Data             []byte           `json:"data"`
	Status           ProposalStatus   `json:"status"`
	CreatedAt        int64            `json:"created_at"`
	VotingStart      int64            `json:"voting_start"`
	VotingEnd        int64            `json:"voting_end"`
	ExecuteAt       int64            `json:"execute_at"`
	YesVotes        decimal.Decimal  `json:"yes_votes"`
	NoVotes         decimal.Decimal  `json:"no_votes"`
	AbstainVotes    decimal.Decimal  `json:"abstain_votes"`
	TotalVotingPower decimal.Decimal `json:"total_voting_power"`
}

// ==================== 投票记录 ====================

// VoteRecord 投票记录
type VoteRecord struct {
	ProposalID  uint64        `json:"proposal_id"`
	Voter       types.Address `json:"voter"`
	VoteOption  VoteOption   `json:"vote_option"`
	Weight      decimal.Decimal `json:"weight"`
	Timestamp   int64         `json:"timestamp"`
}

// ==================== 治理参数 ====================

// Params 治理参数
type Params struct {
	InflationRate          float64         `json:"inflation_rate"`
	DecayFactor            float64         `json:"decay_factor"`
	TxBurnPercent          float64         `json:"tx_burn_percent"`
	RewardBurnThreshold    decimal.Decimal `json:"reward_burn_threshold"`
	RewardBurnPercent      float64         `json:"reward_burn_percent"`
	MinValidatorStake      decimal.Decimal `json:"min_validator_stake"`
	MaxValidators         uint32          `json:"max_validators"`
	EpochLength            uint64          `json:"epoch_length"`
	UnbondingPeriod        int64           `json:"unbonding_period"`
	DataDeviationThreshold float64         `json:"data_deviation_threshold"`
}

func DefaultParams() *Params {
	threshold, _ := decimal.NewFromString("10000000000000000000") // 10000 POLE
	minStake, _ := decimal.NewFromString("10000000000000000000")   // 10000 POLE
	return &Params{
		InflationRate:          0.20,
		DecayFactor:            0.5,
		TxBurnPercent:           0.25,
		RewardBurnThreshold:     threshold,
		RewardBurnPercent:       0.10,
		MinValidatorStake:       minStake,
		MaxValidators:           21,
		EpochLength:             14400,
		UnbondingPeriod:         21 * 24 * 3600, // 21 天
		DataDeviationThreshold:   0.20,
	}
}

// ==================== 治理 ====================

// Governance 治理
type Governance struct {
	config        *Config
	params        *Params
	proposals     map[uint64]*Proposal
	votes         map[uint64]map[types.Address]*VoteRecord
	nextProposalID uint64
	mu            sync.RWMutex
}

// NewGovernance 创建治理
func NewGovernance(config *Config) *Governance {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &Governance{
		config:          config,
		params:          DefaultParams(),
		proposals:       make(map[uint64]*Proposal),
		votes:           make(map[uint64]map[types.Address]*VoteRecord),
		nextProposalID:  1,
	}
}

// CreateProposal 创建提案
func (g *Governance) CreateProposal(
	proposer types.Address,
	title, description string,
	proposalType ProposalType,
	data []byte,
) (uint64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// 检查质押 (简化版本)
	stake := decimal.Zero // 实际应检查实际质押
	if stake.LessThan(g.config.MinProposalStake) {
		return 0, fmt.Errorf("insufficient stake")
	}
	
	id := g.nextProposalID
	g.nextProposalID++
	
	now := time.Now().Unix()
	
	proposal := &Proposal{
		ID:                id,
		Proposer:         proposer,
		Title:            title,
		Description:      description,
		ProposalType:     proposalType,
		Data:             data,
		Status:           ProposalStatusPending,
		CreatedAt:        now,
		VotingStart:       now,
		VotingEnd:        now + g.config.MinVotingPeriod,
		ExecuteAt:        0,
		YesVotes:         decimal.Zero,
		NoVotes:          decimal.Zero,
		AbstainVotes:     decimal.Zero,
		TotalVotingPower: decimal.Zero,
	}
	
	g.proposals[id] = proposal
	g.votes[id] = make(map[types.Address]*VoteRecord)
	
	return id, nil
}

// CastVote 投票
func (g *Governance) CastVote(
	proposalID uint64,
	voter types.Address,
	voteOption VoteOption,
	weight decimal.Decimal,
) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	proposal, ok := g.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}
	
	// 检查投票期
	now := time.Now().Unix()
	if now < proposal.VotingStart || now > proposal.VotingEnd {
		return fmt.Errorf("voting closed")
	}
	
	// 更新投票计数
	switch voteOption {
	case VoteOptionYes:
		proposal.YesVotes = proposal.YesVotes.Add(weight)
	case VoteOptionNo:
		proposal.NoVotes = proposal.NoVotes.Add(weight)
	case VoteOptionAbstain:
		proposal.AbstainVotes = proposal.AbstainVotes.Add(weight)
	}
	
	proposal.TotalVotingPower = proposal.TotalVotingPower.Add(weight)
	
	// 记录投票
	g.votes[proposalID][voter] = &VoteRecord{
		ProposalID:  proposalID,
		Voter:       voter,
		VoteOption:  voteOption,
		Weight:      weight,
		Timestamp:   now,
	}
	
	return nil
}

// TallyVotes 统计投票
func (g *Governance) TallyVotes(proposalID uint64) (ProposalStatus, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	proposal, ok := g.proposals[proposalID]
	if !ok {
		return ProposalStatusPending, fmt.Errorf("proposal not found")
	}
	
	// 检查投票期是否结束
	now := time.Now().Unix()
	if now < proposal.VotingEnd {
		return ProposalStatusPending, fmt.Errorf("voting not ended")
	}
	
	// 统计结果
	total := proposal.YesVotes.Add(proposal.NoVotes).Add(proposal.AbstainVotes)
	
	// 检查法定人数
	quorum := 0.0
	if !proposal.TotalVotingPower.IsZero() {
		quorum, _ = total.Div(proposal.TotalVotingPower).Float64()
	}
	
	if quorum < g.config.QuorumThreshold {
		proposal.Status = ProposalStatusRejected
		return ProposalStatusRejected, nil
	}
	
	// 检查批准
	yesRatio, _ := proposal.YesVotes.Div(total).Float64()
	if yesRatio >= g.config.ApprovalThreshold {
		proposal.Status = ProposalStatusPassed
		proposal.ExecuteAt = now + g.config.ExecutionDelay
	} else {
		proposal.Status = ProposalStatusRejected
	}
	
	return proposal.Status, nil
}

// ExecuteProposal 执行提案
func (g *Governance) ExecuteProposal(proposalID uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	proposal, ok := g.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}
	
	if proposal.Status != ProposalStatusPassed {
		return fmt.Errorf("proposal not passed")
	}
	
	// 检查执行延迟
	now := time.Now().Unix()
	if now < proposal.ExecuteAt {
		return fmt.Errorf("execution not ready")
	}
	
	// 执行提案 (实际会修改参数)
	proposal.Status = ProposalStatusExecuted
	
	return nil
}

// GetProposal 获取提案
func (g *Governance) GetProposal(proposalID uint64) (*Proposal, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	proposal, ok := g.proposals[proposalID]
	return proposal, ok
}

// GetProposals 获取所有提案
func (g *Governance) GetProposals() []*Proposal {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	result := make([]*Proposal, 0, len(g.proposals))
	for _, p := range g.proposals {
		result = append(result, p)
	}
	
	return result
}

// GetParams 获取治理参数
func (g *Governance) GetParams() *Params {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	return g.params
}

// UpdateParams 更新治理参数
func (g *Governance) UpdateParams(params *Params) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.params = params
}
