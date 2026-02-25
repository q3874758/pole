package governance

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ==================== 委托投票配置 ====================

// DelegationConfig 委托配置
type DelegationConfig struct {
	MinDelegationAmount decimal.Decimal // 最小委托金额
	UnbondingPeriod    int64          // 解绑期 (秒)
	MaxDelegators      int             // 最大委托者数
}

func DefaultDelegationConfig() *DelegationConfig {
	minDel, _ := decimal.NewFromString("1000000000000000000") // 1 POLE
	return &DelegationConfig{
		MinDelegationAmount: minDel,
		UnbondingPeriod:    21 * 24 * 3600, // 21天
		MaxDelegators:       100,
	}
}

// ==================== 委托记录 ====================

// Delegation 委托记录
type Delegation struct {
	Delegator    string          `json:"delegator"`
	Recipient   string          `json:"recipient"` // 被委托的验证者或代表
	Amount      decimal.Decimal `json:"amount"`
	StartTime   int64           `json:"start_time"`
	IsActive    bool            `json:"is_active"`
}

// ==================== 委托投票管理器 ====================

// DelegationManager 委托投票管理器
type DelegationManager struct {
	config      *DelegationConfig
	delegations map[string]*Delegation // delegator -> delegation
	receivers   map[string][]string   // recipient -> delegators
	mu          sync.RWMutex
}

// NewDelegationManager 创建委托管理器
func NewDelegationManager(config *DelegationConfig) *DelegationManager {
	if config == nil {
		config = DefaultDelegationConfig()
	}
	return &DelegationManager{
		config:      config,
		delegations: make(map[string]*Delegation),
		receivers:   make(map[string][]string),
	}
}

// Delegate 委托投票权
func (dm *DelegationManager) Delegate(delegator, recipient string, amount decimal.Decimal) error {
	if amount.LessThan(dm.config.MinDelegationAmount) {
		return fmt.Errorf("amount below minimum: %s", dm.config.MinDelegationAmount.String())
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 检查接收者是否有效
	if _, exists := dm.receivers[recipient]; !exists && len(dm.receivers) > 0 {
		// 如果已有委托，检查新接收者是否在列表中
		validRecipient := false
		for r := range dm.receivers {
			if r == recipient {
				validRecipient = true
				break
			}
		}
		if !validRecipient && len(dm.receivers) >= dm.config.MaxDelegators {
			return fmt.Errorf("recipient not valid or max delegators reached")
		}
	}

	// 更新或创建委托
	if existing, exists := dm.delegations[delegator]; exists && existing.IsActive {
		// 增加现有委托
		existing.Amount = existing.Amount.Add(amount)
		existing.Recipient = recipient
	} else {
		// 新委托
		dm.delegations[delegator] = &Delegation{
			Delegator:  delegator,
			Recipient:  recipient,
			Amount:     amount,
			StartTime:  time.Now().Unix(),
			IsActive:   true,
		}
	}

	// 更新接收者列表
	if dm.receivers[recipient] == nil {
		dm.receivers[recipient] = []string{}
	}
	dm.receivers[recipient] = append(dm.receivers[recipient], delegator)

	return nil
}

// Undelegate 取消委托
func (dm *DelegationManager) Undelegate(delegator string) (decimal.Decimal, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	del, exists := dm.delegations[delegator]
	if !exists || !del.IsActive {
		return decimal.Zero, fmt.Errorf("no active delegation found")
	}

	amount := del.Amount
	del.IsActive = false

	// 从接收者列表中移除
	recipient := del.Recipient
	if dm.receivers[recipient] != nil {
		newList := []string{}
		for _, d := range dm.receivers[recipient] {
			if d != delegator {
				newList = append(newList, d)
			}
		}
		dm.receivers[recipient] = newList
	}

	return amount, nil
}

// GetDelegation 获取委托信息
func (dm *DelegationManager) GetDelegation(delegator string) (*Delegation, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	del, exists := dm.delegations[delegator]
	if !exists {
		return nil, false
	}
	return del, true
}

// GetDelegators 获取接收者的所有委托者
func (dm *DelegationManager) GetDelegators(recipient string) []string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	return dm.receivers[recipient]
}

// GetTotalDelegatedTo 获取委托给某接收者的总金额
func (dm *DelegationManager) GetTotalDelegatedTo(recipient string) decimal.Decimal {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	total := decimal.Zero
	for _, del := range dm.delegations {
		if del.Recipient == recipient && del.IsActive {
			total = total.Add(del.Amount)
		}
	}
	return total
}

// GetVotingPower 获取某地址的投票权（包括自己质押 + 委托）
func (dm *DelegationManager) GetVotingPower(address string, selfStake decimal.Decimal) decimal.Decimal {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	// 自己的质押
	total := selfStake

	// 收到的委托
	if receivers, exists := dm.receivers[address]; exists {
		for _, del := range receivers {
			if d, ok := dm.delegations[del]; ok && d.IsActive {
				total = total.Add(d.Amount)
			}
		}
	}

	return total
}

// GetActiveDelegations 获取所有活跃委托
func (dm *DelegationManager) GetActiveDelegations() []*Delegation {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]*Delegation, 0)
	for _, d := range dm.delegations {
		if d.IsActive {
			result = append(result, d)
		}
	}
	return result
}

// ==================== 治理委托 (将投票权委托给其他账户) ====================

// GovernanceDelegation 治理委托
type GovernanceDelegation struct {
	From        string          // 委托者
	To          string          // 被委托者（代表）
	Amount      decimal.Decimal // 委托金额
	VotingPower decimal.Decimal // 委托的投票权
	StartTime   int64
	IsActive    bool
}

// GovernanceDelegations 治理委托管理器
type GovernanceDelegations struct {
	config     *DelegationConfig
	delegations map[string]*GovernanceDelegation
	delegators map[string][]string // to -> from list
	mu         sync.RWMutex
}

// NewGovernanceDelegations 创建治理委托管理器
func NewGovernanceDelegations(config *DelegationConfig) *GovernanceDelegations {
	if config == nil {
		config = DefaultDelegationConfig()
	}
	return &GovernanceDelegations{
		config:     config,
		delegations: make(map[string]*GovernanceDelegation),
		delegators: make(map[string][]string),
	}
}

// DelegateTo 委托投票权给代表
func (gd *GovernanceDelegations) DelegateTo(from, to string, amount decimal.Decimal) error {
	if amount.LessThan(gd.config.MinDelegationAmount) {
		return fmt.Errorf("amount below minimum")
	}

	gd.mu.Lock()
	defer gd.mu.Unlock()

	gd.delegations[from] = &GovernanceDelegation{
		From:        from,
		To:          to,
		Amount:      amount,
		VotingPower: amount, // 投票权等于委托金额
		StartTime:   time.Now().Unix(),
		IsActive:    true,
	}

	if gd.delegators[to] == nil {
		gd.delegators[to] = []string{}
	}
	gd.delegators[to] = append(gd.delegators[to], from)

	return nil
}

// RevokeDelegation 撤销委托
func (gd *GovernanceDelegations) RevokeDelegation(from string) error {
	gd.mu.Lock()
	defer gd.mu.Unlock()

	del, exists := gd.delegations[from]
	if !exists || !del.IsActive {
		return fmt.Errorf("no active delegation")
	}

	del.IsActive = false

	// 从列表移除
	to := del.To
	if gd.delegators[to] != nil {
		newList := []string{}
		for _, f := range gd.delegators[to] {
			if f != from {
				newList = append(newList, f)
			}
		}
		gd.delegators[to] = newList
	}

	return nil
}

// GetVotesFor 获取某代表的总票数
func (gd *GovernanceDelegations) GetVotesFor(representative string) decimal.Decimal {
	gd.mu.RLock()
	defer gd.mu.RUnlock()

	total := decimal.Zero
	for _, del := range gd.delegations {
		if del.To == representative && del.IsActive {
			total = total.Add(del.VotingPower)
		}
	}
	return total
}

// GetDelegate 获取委托信息
func (gd *GovernanceDelegations) GetDelegate(from string) (*GovernanceDelegation, bool) {
	gd.mu.RLock()
	defer gd.mu.RUnlock()

	del, exists := gd.delegations[from]
	return del, exists
}
