package contracts

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ==================== 治理合约配置 ====================

// GovernanceConfig 治理合约配置
type GovernanceConfig struct {
	MinProposalStake   *big.Int // 最小提案质押 (wei)
	MinVotingPeriod    int64    // 最小投票期（秒）
	QuorumThreshold    *big.Int // 法定人数阈值 (bps, 如 4000 = 40%)
	ApprovalThreshold  *big.Int // 批准阈值 (bps, 如 5000 = 50%)
	ExecutionDelay     int64    // 执行延迟（秒）
}

// DefaultGovernanceConfig 默认治理配置
func DefaultGovernanceConfig() *GovernanceConfig {
	return &GovernanceConfig{
		MinProposalStake:  new(big.Int).Mul(big.NewInt(10000), pow10(18)), // 10000 POLE
		MinVotingPeriod:   5 * 24 * 3600,                                  // 5 天
		QuorumThreshold:   big.NewInt(4000),                               // 40%
		ApprovalThreshold: big.NewInt(5000),                                // 50%
		ExecutionDelay:    2 * 24 * 3600,                                   // 2 天
	}
}

// ==================== 提案类型与状态 ====================

// GovProposalType 提案类型
type GovProposalType int

const (
	GovProposalTypeParameterChange GovProposalType = iota
	GovProposalTypeText
	GovProposalTypeTreasurySpend
	GovProposalTypeProtocolUpgrade
)

// GovProposalStatus 提案状态
type GovProposalStatus int

const (
	GovProposalStatusPending GovProposalStatus = iota
	GovProposalStatusVoting
	GovProposalStatusPassed
	GovProposalStatusRejected
	GovProposalStatusExecuted
	GovProposalStatusCancelled
)

// GovVoteOption 投票选项
type GovVoteOption int

const (
	GovVoteYes GovVoteOption = iota
	GovVoteNo
	GovVoteAbstain
)

// ==================== 治理提案与投票 ====================

// GovProposal 链上提案
type GovProposal struct {
	ID             uint64
	Proposer       string
	Title          string
	Description    string
	ProposalType   GovProposalType
	Data           []byte
	Status         GovProposalStatus
	CreatedAt      int64
	VotingStart    int64
	VotingEnd      int64
	ExecuteAt      int64
	YesVotes       *big.Int
	NoVotes        *big.Int
	AbstainVotes   *big.Int
	TotalVotingPower *big.Int
}

// GovVoteRecord 投票记录
type GovVoteRecord struct {
	ProposalID uint64
	Voter      string
	Option     GovVoteOption
	Weight     *big.Int
	Timestamp  int64
}

// ==================== 治理合约 ====================

// GovernanceContract 链上治理合约
type GovernanceContract struct {
	config         *GovernanceConfig          `json:"-"`
	proposals      map[uint64]*GovProposal
	votes          map[uint64]map[string]*GovVoteRecord
	nextProposalID uint64
	mu             sync.RWMutex `json:"-"`
}

// MarshalJSON 自定义序列化
func (gc *GovernanceContract) MarshalJSON() ([]byte, error) {
	type Alias GovernanceContract
	return json.Marshal(struct {
		Proposals      map[uint64]*GovProposal         `json:"proposals"`
		Votes          map[uint64]map[string]*GovVoteRecord `json:"votes"`
		NextProposalID uint64                            `json:"next_proposal_id"`
		Alias
	}{
		Proposals:      gc.proposals,
		Votes:          gc.votes,
		NextProposalID: gc.nextProposalID,
		Alias:          Alias(*gc),
	})
}

// UnmarshalJSON 自定义反序列化
func (gc *GovernanceContract) UnmarshalJSON(data []byte) error {
	type Alias GovernanceContract
	aux := struct {
		Proposals      map[uint64]*GovProposal              `json:"proposals"`
		Votes          map[uint64]map[string]*GovVoteRecord `json:"votes"`
		NextProposalID uint64                               `json:"next_proposal_id"`
		*Alias
	}{
		Alias: (*Alias)(gc),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	gc.proposals = aux.Proposals
	gc.votes = aux.Votes
	gc.nextProposalID = aux.NextProposalID
	gc.config = DefaultGovernanceConfig()
	return nil
}

// NewGovernanceContract 创建治理合约
func NewGovernanceContract(config *GovernanceConfig) *GovernanceContract {
	if config == nil {
		config = DefaultGovernanceConfig()
	}
	return &GovernanceContract{
		config:         config,
		proposals:      make(map[uint64]*GovProposal),
		votes:          make(map[uint64]map[string]*GovVoteRecord),
		nextProposalID: 1,
	}
}

// CreateProposal 创建提案（调用方需已满足最小质押）
func (gc *GovernanceContract) CreateProposal(
	proposer, title, description string,
	proposalType GovProposalType,
	data []byte,
) (uint64, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	id := gc.nextProposalID
	gc.nextProposalID++
	now := time.Now().Unix()

	p := &GovProposal{
		ID:               id,
		Proposer:         proposer,
		Title:            title,
		Description:      description,
		ProposalType:     proposalType,
		Data:             data,
		Status:           GovProposalStatusVoting,
		CreatedAt:        now,
		VotingStart:      now,
		VotingEnd:        now + gc.config.MinVotingPeriod,
		ExecuteAt:        0,
		YesVotes:         big.NewInt(0),
		NoVotes:          big.NewInt(0),
		AbstainVotes:     big.NewInt(0),
		TotalVotingPower: big.NewInt(0),
	}
	gc.proposals[id] = p
	gc.votes[id] = make(map[string]*GovVoteRecord)
	return id, nil
}

// CastVote 投票
func (gc *GovernanceContract) CastVote(
	proposalID uint64,
	voter string,
	option GovVoteOption,
	weight *big.Int,
) error {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	p, ok := gc.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}
	if p.Status != GovProposalStatusVoting {
		return fmt.Errorf("proposal not in voting")
	}
	now := time.Now().Unix()
	if now < p.VotingStart || now > p.VotingEnd {
		return fmt.Errorf("voting closed")
	}
	if _, voted := gc.votes[proposalID][voter]; voted {
		return fmt.Errorf("already voted")
	}

	switch option {
	case GovVoteYes:
		p.YesVotes = new(big.Int).Add(p.YesVotes, weight)
	case GovVoteNo:
		p.NoVotes = new(big.Int).Add(p.NoVotes, weight)
	case GovVoteAbstain:
		p.AbstainVotes = new(big.Int).Add(p.AbstainVotes, weight)
	}
	p.TotalVotingPower = new(big.Int).Add(p.TotalVotingPower, weight)
	gc.votes[proposalID][voter] = &GovVoteRecord{
		ProposalID: proposalID,
		Voter:      voter,
		Option:     option,
		Weight:     new(big.Int).Set(weight),
		Timestamp:  now,
	}
	return nil
}

// TallyProposal 统计提案结果并更新状态
func (gc *GovernanceContract) TallyProposal(proposalID uint64) (GovProposalStatus, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	p, ok := gc.proposals[proposalID]
	if !ok {
		return GovProposalStatusPending, fmt.Errorf("proposal not found")
	}
	if p.Status != GovProposalStatusVoting {
		return p.Status, nil
	}
	now := time.Now().Unix()
	if now <= p.VotingEnd {
		return GovProposalStatusVoting, fmt.Errorf("voting not ended")
	}

	total := new(big.Int).Add(p.YesVotes, p.NoVotes)
	total = total.Add(total, p.AbstainVotes)
	if total.Sign() == 0 {
		p.Status = GovProposalStatusRejected
		return GovProposalStatusRejected, nil
	}
	// 法定人数：TotalVotingPower 与链上总质押比较由调用方做，此处仅按票数比例
	yesBps := new(big.Int).Mul(p.YesVotes, big.NewInt(10000))
	yesBps = yesBps.Div(yesBps, total)
	if yesBps.Cmp(gc.config.ApprovalThreshold) >= 0 {
		p.Status = GovProposalStatusPassed
		p.ExecuteAt = now + gc.config.ExecutionDelay
		return GovProposalStatusPassed, nil
	}
	p.Status = GovProposalStatusRejected
	return GovProposalStatusRejected, nil
}

// MarkExecuted 标记提案已执行
func (gc *GovernanceContract) MarkExecuted(proposalID uint64) error {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	p, ok := gc.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}
	if p.Status != GovProposalStatusPassed {
		return fmt.Errorf("proposal not passed")
	}
	p.Status = GovProposalStatusExecuted
	return nil
}

// GetProposal 获取提案
func (gc *GovernanceContract) GetProposal(id uint64) (*GovProposal, bool) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	p, ok := gc.proposals[id]
	if !ok {
		return nil, false
	}
	// 返回副本避免外部修改
	cp := *p
	cp.YesVotes = new(big.Int).Set(p.YesVotes)
	cp.NoVotes = new(big.Int).Set(p.NoVotes)
	cp.AbstainVotes = new(big.Int).Set(p.AbstainVotes)
	cp.TotalVotingPower = new(big.Int).Set(p.TotalVotingPower)
	return &cp, true
}
