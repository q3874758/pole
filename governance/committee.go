package governance

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ==================== 治理委员会配置 ====================

// CommitteeConfig 治理委员会配置
type CommitteeConfig struct {
	MaxMembers           int             // 最大委员数
	MinStake             decimal.Decimal // 最低质押要求
	ElectionInterval     int64           // 选举周期 (秒)
	TermLength           int64           // 任期长度 (秒)
	QuorumPercentage    decimal.Decimal // 法定人数比例
}

func DefaultCommitteeConfig() *CommitteeConfig {
	minStake, _ := decimal.NewFromString("1000000000000000000000") // 10000 POLE
	return &CommitteeConfig{
		MaxMembers:        7,
		MinStake:          minStake,
		ElectionInterval:   90 * 24 * 3600, // 90天
		TermLength:        365 * 24 * 3600,  // 1年
		QuorumPercentage:  decimal.NewFromFloat(0.4), // 40%
	}
}

// ==================== 委员会成员 ====================

// CommitteeMember 委员会成员
type CommitteeMember struct {
	Address      string          `json:"address"`
	Stake        decimal.Decimal `json:"stake"`
	VoteCount    decimal.Decimal `json:"vote_count"`
	JoinedAt     int64          `json:"joined_at"`
	TermEndAt    int64          `json:"term_end_at"`
	IsActive     bool           `json:"is_active"`
}

// ==================== 治理委员会 ====================

// GovernanceCommittee 治理委员会
type GovernanceCommittee struct {
	config      *CommitteeConfig
	members     map[string]*CommitteeMember
	candidates map[string]*CommitteeMember
	votes      map[string]map[string]decimal.Decimal // candidate -> voter -> amount
	mu         sync.RWMutex
}

// NewGovernanceCommittee 创建治理委员会
func NewGovernanceCommittee(config *CommitteeConfig) *GovernanceCommittee {
	if config == nil {
		config = DefaultCommitteeConfig()
	}
	return &GovernanceCommittee{
		config:      config,
		members:     make(map[string]*CommitteeMember),
		candidates:  make(map[string]*CommitteeMember),
		votes:       make(map[string]map[string]decimal.Decimal),
	}
}

// RegisterCandidate 注册候选人
func (gc *GovernanceCommittee) RegisterCandidate(address string, stake decimal.Decimal) error {
	if stake.LessThan(gc.config.MinStake) {
		return fmt.Errorf("stake below minimum: %s", gc.config.MinStake.String())
	}

	gc.mu.Lock()
	defer gc.mu.Unlock()

	if _, exists := gc.members[address]; exists {
		return fmt.Errorf("already a committee member")
	}
	if _, exists := gc.candidates[address]; exists {
		return fmt.Errorf("already a candidate")
	}

	gc.candidates[address] = &CommitteeMember{
		Address:   address,
		Stake:     stake,
		VoteCount: decimal.Zero,
		JoinedAt:  0,
		TermEndAt: 0,
		IsActive:  false,
	}

	return nil
}

// Vote 投票给候选人
func (gc *GovernanceCommittee) Vote(candidate, voter string, amount decimal.Decimal) error {
	if amount.IsNegative() || amount.IsZero() {
		return fmt.Errorf("invalid vote amount")
	}

	gc.mu.Lock()
	defer gc.mu.Unlock()

	// 检查候选人是否存在
	if _, exists := gc.candidates[candidate]; !exists {
		return fmt.Errorf("candidate not found")
	}

	// 记录投票
	if gc.votes[candidate] == nil {
		gc.votes[candidate] = make(map[string]decimal.Decimal)
	}
	gc.votes[candidate][voter] = amount

	// 更新候选人票数
	gc.candidates[candidate].VoteCount = gc.candidates[candidate].VoteCount.Add(amount)

	return nil
}

// Unvote 取消投票
func (gc *GovernanceCommittee) Unvote(candidate, voter string) decimal.Decimal {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	if gc.votes[candidate] == nil {
		return decimal.Zero
	}

	amount := gc.votes[candidate][voter]
	delete(gc.votes[candidate], voter)

	if c, exists := gc.candidates[candidate]; exists {
		c.VoteCount = c.VoteCount.Sub(amount)
	}

	return amount
}

// ElectCommittee 选举委员会
func (gc *GovernanceCommittee) ElectCommittee() error {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	// 按票数排序候选人
	type candidateWithVotes struct {
		address string
		votes   decimal.Decimal
	}

	candidates := make([]candidateWithVotes, 0, len(gc.candidates))
	for addr, c := range gc.candidates {
		candidates = append(candidates, candidateWithVotes{addr, c.VoteCount})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].votes.GreaterThan(candidates[j].votes)
	})

	// 选出前 N 名
	newMembers := make(map[string]*CommitteeMember)
	now := time.Now().Unix()

	for i := 0; i < len(candidates) && i < gc.config.MaxMembers; i++ {
		addr := candidates[i].address
		c := gc.candidates[addr]

		newMembers[addr] = &CommitteeMember{
			Address:   addr,
			Stake:     c.Stake,
			VoteCount: c.VoteCount,
			JoinedAt:  now,
			TermEndAt: now + gc.config.TermLength,
			IsActive:  true,
		}
	}

	// 更新成员
	gc.members = newMembers

	// 清理候选人
	gc.candidates = make(map[string]*CommitteeMember)
	gc.votes = make(map[string]map[string]decimal.Decimal)

	return nil
}

// GetMembers 获取当前委员会成员
func (gc *GovernanceCommittee) GetMembers() []*CommitteeMember {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	members := make([]*CommitteeMember, 0, len(gc.members))
	for _, m := range gc.members {
		members = append(members, m)
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].VoteCount.GreaterThan(members[j].VoteCount)
	})

	return members
}

// GetCandidates 获取候选人列表
func (gc *GovernanceCommittee) GetCandidates() []*CommitteeMember {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	candidates := make([]*CommitteeMember, 0, len(gc.candidates))
	for _, c := range gc.candidates {
		candidates = append(candidates, c)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].VoteCount.GreaterThan(candidates[j].VoteCount)
	})

	return candidates
}

// IsMember 检查是否为委员会成员
func (gc *GovernanceCommittee) IsMember(address string) bool {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	if m, exists := gc.members[address]; exists && m.IsActive {
		// 检查任期是否到期
		if time.Now().Unix() > m.TermEndAt {
			return false
		}
		return true
	}
	return false
}

// RemoveMember 移除成员 (通过投票)
func (gc *GovernanceCommittee) RemoveMember(address string, votes decimal.Decimal) error {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	m, exists := gc.members[address]
	if !exists {
		return fmt.Errorf("not a committee member")
	}

	// 检查是否达到弹劾阈值
	totalVotes := m.VoteCount
	if votes.GreaterThanOrEqual(totalVotes.Mul(decimal.NewFromFloat(2))) {
		delete(gc.members, address)
		return nil
	}

	return fmt.Errorf("insufficient votes for removal")
}
