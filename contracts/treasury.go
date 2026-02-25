package contracts

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ==================== 国库配置 ====================

// TreasuryConfig 国库配置
type TreasuryConfig struct {
	MinProposalAmount *big.Int // 最小提案金额
	ApprovalThreshold *big.Int // 通过阈值 (bps)
	VotingPeriod     int64    // 投票期 (秒)
	ExecutionPeriod  int64    // 执行期 (秒)
}

func DefaultTreasuryConfig() *TreasuryConfig {
	return &TreasuryConfig{
		MinProposalAmount: new(big.Int).Mul(big.NewInt(1000), pow10(18)), // 1000 POLE
		ApprovalThreshold: big.NewInt(5000), // 50%
		VotingPeriod:      7 * 24 * 3600, // 7 天
		ExecutionPeriod:   3 * 24 * 3600, // 3 天
	}
}

// ==================== 支出提案 ====================

// TreasuryProposal 国库支出提案
type TreasuryProposal struct {
	ID          uint64
	Proposer    string
	Recipient   string
	Amount      *big.Int
	Description string
	Status     string // Pending/Voting/Approved/Rejected/Executed
	VotesYes    *big.Int
	VotesNo     *big.Int
	CreatedAt   int64
	VotingEnd   int64
	ExecutedAt  int64
}

// ==================== 国库合约 ====================

// TreasuryContract 国库合约
type TreasuryContract struct {
	config      *TreasuryConfig
	balance    *big.Int
	proposals  map[uint64]*TreasuryProposal
	votes      map[uint64]map[string]bool // proposalID -> voter -> voted
	nextID     uint64
	mu         sync.RWMutex
}

// NewTreasuryContract 创建国库合约
func NewTreasuryContract(config *TreasuryConfig) *TreasuryContract {
	if config == nil {
		config = DefaultTreasuryConfig()
	}
	return &TreasuryContract{
		config:    config,
		balance:  big.NewInt(0),
		proposals: make(map[uint64]*TreasuryProposal),
		votes:     make(map[uint64]map[string]bool),
		nextID:    1,
	}
}

// Deposit 存款
func (tc *TreasuryContract) Deposit(from string, amount *big.Int) error {
	if amount.Sign() <= 0 {
		return fmt.Errorf("invalid amount")
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.balance = new(big.Int).Add(tc.balance, amount)
	return nil
}

// GetBalance 获取余额
func (tc *TreasuryContract) GetBalance() *big.Int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return new(big.Int).Set(tc.balance)
}

// CreateProposal 创建支出提案
func (tc *TreasuryContract) CreateProposal(proposer, recipient, description string, amount *big.Int) (uint64, error) {
	if amount.Cmp(tc.config.MinProposalAmount) < 0 {
		return 0, fmt.Errorf("amount below minimum: %s", tc.config.MinProposalAmount.String())
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.balance.Cmp(amount) < 0 {
		return 0, fmt.Errorf("insufficient treasury balance")
	}

	id := tc.nextID
	tc.nextID++

	now := time.Now().Unix()
	proposal := &TreasuryProposal{
		ID:          id,
		Proposer:    proposer,
		Recipient:   recipient,
		Amount:      new(big.Int).Set(amount),
		Description: description,
		Status:     "Voting",
		VotesYes:   big.NewInt(0),
		VotesNo:    big.NewInt(0),
		CreatedAt:   now,
		VotingEnd:   now + tc.config.VotingPeriod,
		ExecutedAt: 0,
	}

	tc.proposals[id] = proposal
	tc.votes[id] = make(map[string]bool)

	return id, nil
}

// Vote 投票
func (tc *TreasuryContract) Vote(proposalID uint64, voter string, support bool) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	proposal, ok := tc.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}

	if proposal.Status != "Voting" {
		return fmt.Errorf("proposal not in voting period")
	}

	now := time.Now().Unix()
	if now > proposal.VotingEnd {
		return fmt.Errorf("voting period ended")
	}

	// 检查是否已投票
	if tc.votes[proposalID][voter] {
		return fmt.Errorf("already voted")
	}

	tc.votes[proposalID][voter] = true

	if support {
		proposal.VotesYes = new(big.Int).Add(proposal.VotesYes, big.NewInt(1))
	} else {
		proposal.VotesNo = new(big.Int).Add(proposal.VotesNo, big.NewInt(1))
	}

	return nil
}

// Tally 统计投票
func (tc *TreasuryContract) Tally(proposalID uint64) (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	proposal, ok := tc.proposals[proposalID]
	if !ok {
		return "", fmt.Errorf("proposal not found")
	}

	now := time.Now().Unix()
	if now < proposal.VotingEnd {
		return "", fmt.Errorf("voting not ended")
	}

	total := new(big.Int).Add(proposal.VotesYes, proposal.VotesNo)
	if total.Sign() == 0 {
		proposal.Status = "Rejected"
		return "Rejected", nil
	}

	// 计算通过率 (bps)
	yesBps := new(big.Int).Mul(proposal.VotesYes, big.NewInt(10000))
	yesBps = yesBps.Div(yesBps, total)

	if yesBps.Cmp(tc.config.ApprovalThreshold) >= 0 {
		proposal.Status = "Approved"
		return "Approved", nil
	}

	proposal.Status = "Rejected"
	return "Rejected", nil
}

// Execute 执行提案
func (tc *TreasuryContract) Execute(proposalID uint64) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	proposal, ok := tc.proposals[proposalID]
	if !ok {
		return fmt.Errorf("proposal not found")
	}

	if proposal.Status != "Approved" {
		return fmt.Errorf("proposal not approved")
	}

	// 检查余额
	if tc.balance.Cmp(proposal.Amount) < 0 {
		return fmt.Errorf("insufficient balance")
	}

	// 执行转账
	tc.balance = new(big.Int).Sub(tc.balance, proposal.Amount)
	// TODO: 实际转账到 recipient

	proposal.Status = "Executed"
	proposal.ExecutedAt = time.Now().Unix()

	return nil
}

// GetProposal 获取提案
func (tc *TreasuryContract) GetProposal(id uint64) (*TreasuryProposal, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	p, ok := tc.proposals[id]
	if !ok {
		return nil, false
	}
	return &TreasuryProposal{
		ID:          p.ID,
		Proposer:    p.Proposer,
		Recipient:   p.Recipient,
		Amount:      new(big.Int).Set(p.Amount),
		Description: p.Description,
		Status:     p.Status,
		VotesYes:   new(big.Int).Set(p.VotesYes),
		VotesNo:    new(big.Int).Set(p.VotesNo),
		CreatedAt:   p.CreatedAt,
		VotingEnd:   p.VotingEnd,
		ExecutedAt:  p.ExecutedAt,
	}, true
}

// GetProposals 获取所有提案
func (tc *TreasuryContract) GetProposals() []*TreasuryProposal {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	result := make([]*TreasuryProposal, 0, len(tc.proposals))
	for _, p := range tc.proposals {
		result = append(result, p)
	}
	return result
}

// ==================== JSON 序列化 ====================

func (tc *TreasuryContract) MarshalJSON() ([]byte, error) {
	type Alias TreasuryContract
	return json.Marshal(struct {
		Balance  string                  `json:"balance"`
		Proposals map[string]*TreasuryProposal `json:"proposals"`
		Alias
	}{
		Balance:  tc.balance.String(),
		Proposals: convertProposals(tc.proposals),
		Alias:     Alias(*tc),
	})
}

func convertProposals(m map[uint64]*TreasuryProposal) map[string]*TreasuryProposal {
	r := make(map[string]*TreasuryProposal)
	for k, v := range m {
		r[fmt.Sprintf("%d", k)] = v
	}
	return r
}

// UnmarshalJSON 反序列化（用于链状态加载）
func (tc *TreasuryContract) UnmarshalJSON(data []byte) error {
	aux := &struct {
		Balance   string                    `json:"balance"`
		Proposals map[string]*TreasuryProposal `json:"proposals"`
	}{}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	tc.balance = new(big.Int)
	tc.balance.SetString(aux.Balance, 10)
	tc.proposals = make(map[uint64]*TreasuryProposal)
	tc.votes = make(map[uint64]map[string]bool)
	var maxID uint64
	for k, p := range aux.Proposals {
		id := uint64(0)
		fmt.Sscanf(k, "%d", &id)
		tc.proposals[id] = p
		if p != nil {
			tc.votes[id] = make(map[string]bool)
		}
		if id > maxID {
			maxID = id
		}
	}
	tc.nextID = maxID + 1
	return nil
}
