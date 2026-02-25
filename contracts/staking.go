package contracts

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"
)

// ==================== 质押配置 ====================

// StakingConfig 质押配置
type StakingConfig struct {
	MinStake              *big.Int       // 最小质押
	MaxValidators        uint32         // 最大验证者数
	UnbondingPeriod      int64          // 解绑周期（秒）
	MaxDelegatorsPerValidator uint32     // 每个验证者最大委托者数
	RewardDistributionPeriod int64       // 奖励分发周期（秒）
	CommissionMin         uint8          // 最小佣金比例
	CommissionMax         uint8          // 最大佣金比例
	SlashDoubleSign      *big.Int       // 双签惩罚比例
	SlashOffline         *big.Int       // 离线惩罚比例
}

// DefaultStakingConfig 默认质押配置
func DefaultStakingConfig() *StakingConfig {
	return &StakingConfig{
		MinStake:                  new(big.Int).Mul(big.NewInt(10000), pow10(18)), // 10000 POLE
		MaxValidators:             21,
		UnbondingPeriod:          21 * 24 * 3600, // 21天
		MaxDelegatorsPerValidator: 100,
		RewardDistributionPeriod:  86400, // 1天
		CommissionMin:            1,    // 1%
		CommissionMax:            50,   // 50%
		SlashDoubleSign:          big.NewInt(500),  // 5%
		SlashOffline:             big.NewInt(100),   // 1%
	}
}

// ==================== 验证者 ====================

// Validator 验证者
type Validator struct {
	Address         string
	PubKey          string
	Stake           *big.Int
	Delegated       *big.Int
	Commission      uint8
	Rewards         *big.Int
	Status          ValidatorStatus
	JailedUntil     int64
	MinSelfStake   *big.Int
	TotalDelegators uint32
	Signers        map[string]bool
	RegisteredAt   int64
	LastBlockSigned int64
}

// ValidatorStatus 验证者状态
type ValidatorStatus int

const (
	ValidatorStatusInactive ValidatorStatus = iota
	ValidatorStatusActive
	ValidatorStatusJailed
	ValidatorStatusUnbonding
)

// ==================== 委托 ====================

// Delegation 委托
type Delegation struct {
	Delegator  string
	Validator  string
	Amount     *big.Int
	Rewards    *big.Int
	StartTime  int64
	EndTime    int64
	IsActive   bool
}

// ==================== 质押合约 ====================

// StakingContract 质押合约
type StakingContract struct {
	config        *StakingConfig       `json:"-"`
	token         *TokenContract       `json:"-"`
	validators    map[string]*Validator
	delegations   map[string][]*Delegation // validator -> delegations
	delegatorMap  map[string]*Delegation   // delegator -> active delegation
	totalStaked   *big.Int
	epochRewards  map[string]*big.Int // epoch -> rewards
	mu            sync.RWMutex        `json:"-"`
}

// MarshalJSON 自定义序列化
func (sc *StakingContract) MarshalJSON() ([]byte, error) {
	type Alias StakingContract
	return json.Marshal(struct {
		Validators    map[string]*Validator     `json:"validators"`
		Delegations   map[string][]*Delegation  `json:"delegations"`
		DelegatorMap  map[string]*Delegation    `json:"delegator_map"`
		TotalStaked   string                     `json:"total_staked"`
		EpochRewards  map[string]string          `json:"epoch_rewards"`
		Alias
	}{
		Validators:   sc.validators,
		Delegations:  sc.delegations,
		DelegatorMap: sc.delegatorMap,
		TotalStaked:  sc.totalStaked.String(),
		EpochRewards: epochRewardsToString(sc.epochRewards),
		Alias:        Alias(*sc),
	})
}

func epochRewardsToString(m map[string]*big.Int) map[string]string {
	if m == nil {
		return nil
	}
	r := make(map[string]string)
	for k, v := range m {
		r[k] = v.String()
	}
	return r
}

func epochRewardsFromString(m map[string]string) map[string]*big.Int {
	if m == nil {
		return nil
	}
	r := make(map[string]*big.Int)
	for k, v := range m {
		r[k] = new(big.Int)
		r[k].SetString(v, 10)
	}
	return r
}

// UnmarshalJSON 自定义反序列化
func (sc *StakingContract) UnmarshalJSON(data []byte) error {
	type Alias StakingContract
	aux := struct {
		Validators    map[string]*Validator    `json:"validators"`
		Delegations   map[string][]*Delegation `json:"delegations"`
		DelegatorMap  map[string]*Delegation   `json:"delegator_map"`
		TotalStaked   string                   `json:"total_staked"`
		EpochRewards  map[string]string       `json:"epoch_rewards"`
		*Alias
	}{
		Alias: (*Alias)(sc),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	sc.validators = aux.Validators
	sc.delegations = aux.Delegations
	sc.delegatorMap = aux.DelegatorMap
	sc.totalStaked = new(big.Int)
	sc.totalStaked.SetString(aux.TotalStaked, 10)
	sc.epochRewards = epochRewardsFromString(aux.EpochRewards)
	sc.config = DefaultStakingConfig()
	sc.token = nil // 需要外部重新设置
	return nil
}

// NewStakingContract 创建质押合约
func NewStakingContract(config *StakingConfig, token *TokenContract) *StakingContract {
	if config == nil {
		config = DefaultStakingConfig()
	}

	return &StakingContract{
		config:       config,
		token:        token,
		validators:   make(map[string]*Validator),
		delegations:   make(map[string][]*Delegation),
		delegatorMap: make(map[string]*Delegation),
		totalStaked:  big.NewInt(0),
		epochRewards: make(map[string]*big.Int),
	}
}

// ==================== 验证者操作 ====================

// RegisterValidator 注册验证者
func (sc *StakingContract) RegisterValidator(
	addr, pubKey string,
	commission uint8,
	selfStake *big.Int,
) error {
	// 检查质押金额
	if selfStake.Cmp(sc.config.MinStake) < 0 {
		return fmt.Errorf("insufficient stake: %s < %s", selfStake.String(), sc.config.MinStake.String())
	}

	// 检查佣金范围
	if commission < sc.config.CommissionMin || commission > sc.config.CommissionMax {
		return fmt.Errorf("commission out of range: %d", commission)
	}

	// 检查验证者数量
	if uint32(len(sc.validators)) >= sc.config.MaxValidators {
		return fmt.Errorf("max validators reached")
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	// 检查是否已注册
	if _, exists := sc.validators[addr]; exists {
		return fmt.Errorf("validator already registered")
	}

	// 质押从调用方转入合约（实际链上由 caller approve + transferFrom）
	// 此处仅记录，不执行转账

	// 创建验证者记录
	sc.validators[addr] = &Validator{
		Address:     addr,
		PubKey:     pubKey,
		Stake:      new(big.Int).Set(selfStake),
		Delegated:  big.NewInt(0),
		Commission: commission,
		Rewards:    big.NewInt(0),
		Status:     ValidatorStatusActive,
		JailedUntil: 0,
		MinSelfStake: selfStake,
		TotalDelegators: 0,
		Signers:   make(map[string]bool),
		RegisteredAt: time.Now().Unix(),
	}

	sc.totalStaked = new(big.Int).Add(sc.totalStaked, selfStake)

	return nil
}

// UnregisterValidator 注销验证者
func (sc *StakingContract) UnregisterValidator(addr string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	validator, exists := sc.validators[addr]
	if !exists {
		return fmt.Errorf("validator not found")
	}

	// 检查是否有委托
	if validator.Delegated.Cmp(big.NewInt(0)) > 0 {
		return fmt.Errorf("has active delegations")
	}

	// 设置为解绑状态
	validator.Status = ValidatorStatusUnbonding

	// 返还质押
	// 简化处理：直接转回（实际应该先锁定）
	sc.totalStaked = new(big.Int).Sub(sc.totalStaked, validator.Stake)

	delete(sc.validators, addr)

	return nil
}

// UpdateCommission 更新佣金
func (sc *StakingContract) UpdateCommission(addr string, commission uint8) error {
	if commission < sc.config.CommissionMin || commission > sc.config.CommissionMax {
		return fmt.Errorf("commission out of range")
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	validator, exists := sc.validators[addr]
	if !exists {
		return fmt.Errorf("validator not found")
	}

	validator.Commission = commission
	return nil
}

// ==================== 委托操作 ====================

// Delegate 委托
func (sc *StakingContract) Delegate(delegator, validatorAddr string, amount *big.Int) error {
	if amount.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	// 检查验证者是否存在
	validator, exists := sc.validators[validatorAddr]
	if !exists {
		return fmt.Errorf("validator not found")
	}

	// 检查验证者状态
	if validator.Status != ValidatorStatusActive {
		return fmt.Errorf("validator not active")
	}

	// 检查委托者数量
	if validator.TotalDelegators >= sc.config.MaxDelegatorsPerValidator {
		return fmt.Errorf("max delegators reached")
	}

	// 检查余额（简化：假设已经授权）
	balance := sc.token.BalanceOf(delegator)
	if balance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient balance")
	}

	// 执行转账（简化）
	// 实际应该先 Approve 再 TransferFrom

	// 创建委托记录
	delegation := &Delegation{
		Delegator: delegator,
		Validator: validatorAddr,
		Amount:   new(big.Int).Set(amount),
		Rewards:  big.NewInt(0),
		StartTime: time.Now().Unix(),
		EndTime:  0,
		IsActive: true,
	}

	sc.delegations[validatorAddr] = append(sc.delegations[validatorAddr], delegation)
	sc.delegatorMap[delegator] = delegation

	// 更新验证者委托总额
	validator.Delegated = new(big.Int).Add(validator.Delegated, amount)
	validator.TotalDelegators++

	// 更新总质押
	sc.totalStaked = new(big.Int).Add(sc.totalStaked, amount)

	return nil
}

// Undelegate 解除委托
func (sc *StakingContract) Undelegate(delegator string, amount *big.Int) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// 获取当前委托
	delegation, exists := sc.delegatorMap[delegator]
	if !exists || !delegation.IsActive {
		return fmt.Errorf("no active delegation")
	}

	// 检查委托金额
	if amount.Cmp(delegation.Amount) > 0 {
		return fmt.Errorf("exceeds delegation amount")
	}

	// 更新委托金额
	delegation.Amount = new(big.Int).Sub(delegation.Amount, amount)

	// 如果完全解除
	if delegation.Amount.Cmp(big.NewInt(0)) == 0 {
		delegation.IsActive = false
		delegation.EndTime = time.Now().Unix()

		// 从验证者移除
		validator, _ := sc.validators[delegation.Validator]
		if validator != nil {
			validator.TotalDelegators--
		}

		delete(sc.delegatorMap, delegator)
	}

	// 更新总质押
	sc.totalStaked = new(big.Int).Sub(sc.totalStaked, amount)

	return nil
}

// ==================== 惩罚 ====================

// Slash 惩罚验证者
func (sc *StakingContract) Slash(validatorAddr string, reason SlashReason) (*big.Int, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	validator, exists := sc.validators[validatorAddr]
	if !exists {
		return nil, fmt.Errorf("validator not found")
	}

	var slashPercent *big.Int
	switch reason {
	case SlashReasonDoubleSign:
		slashPercent = sc.config.SlashDoubleSign
	case SlashReasonOffline:
		slashPercent = sc.config.SlashOffline
	default:
		slashPercent = sc.config.SlashOffline
	}

	// 计算惩罚金额
	slashAmount := new(big.Int).Mul(validator.Stake, slashPercent)
	slashAmount = new(big.Int).Div(slashAmount, big.NewInt(10000))

	// 扣除质押
	validator.Stake = new(big.Int).Sub(validator.Stake, slashAmount)

	// 更新总质押
	sc.totalStaked = new(big.Int).Sub(sc.totalStaked, slashAmount)

	// 检查是否需要监禁
	if slashPercent.Cmp(sc.config.SlashDoubleSign) == 0 {
		validator.Status = ValidatorStatusJailed
		validator.JailedUntil = time.Now().Add(7 * 24 * time.Hour).Unix()
	}

	return slashAmount, nil
}

// Unjail 解除监禁
func (sc *StakingContract) Unjail(validatorAddr string) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	validator, exists := sc.validators[validatorAddr]
	if !exists {
		return fmt.Errorf("validator not found")
	}

	if validator.Status != ValidatorStatusJailed {
		return fmt.Errorf("validator not jailed")
	}

	if time.Now().Unix() < validator.JailedUntil {
		return fmt.Errorf("jail period not over")
	}

	validator.Status = ValidatorStatusActive
	validator.JailedUntil = 0

	return nil
}

// SlashReason 惩罚原因
type SlashReason int

const (
	SlashReasonDoubleSign SlashReason = iota
	SlashReasonOffline
	SlashReasonMalicious
)

// ==================== 奖励 ====================

// DistributeRewards 分发奖励
func (sc *StakingContract) DistributeRewards(validatorAddr string, reward *big.Int) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	validator, exists := sc.validators[validatorAddr]
	if !exists {
		return fmt.Errorf("validator not found")
	}

	// 计算验证者佣金
	commission := new(big.Int).Mul(reward, big.NewInt(int64(validator.Commission)))
	commission = new(big.Int).Div(commission, big.NewInt(100))

	// 验证者获得佣金
	validator.Rewards = new(big.Int).Add(validator.Rewards, commission)

	// 剩余奖励分配给委托者
	delegatorReward := new(big.Int).Sub(reward, commission)

	if validator.Delegated.Cmp(big.NewInt(0)) > 0 {
		// 按委托比例分配
		for _, del := range sc.delegations[validatorAddr] {
			if !del.IsActive {
				continue
			}

			share := new(big.Int).Mul(delegatorReward, del.Amount)
			share = new(big.Int).Div(share, validator.Delegated)

			del.Rewards = new(big.Int).Add(del.Rewards, share)
		}
	}

	return nil
}

// ClaimRewards 领取奖励
func (sc *StakingContract) ClaimRewards(addr string) (*big.Int, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	var totalReward *big.Int

	// 检查是否是验证者
	if validator, exists := sc.validators[addr]; exists {
		totalReward = new(big.Int).Set(validator.Rewards)
		validator.Rewards = big.NewInt(0)
	}

	// 检查是否是委托者
	if del, exists := sc.delegatorMap[addr]; exists && del.IsActive {
		if totalReward == nil {
			totalReward = big.NewInt(0)
		}
		totalReward = new(big.Int).Add(totalReward, del.Rewards)
		del.Rewards = big.NewInt(0)
	}

	if totalReward == nil || totalReward.Cmp(big.NewInt(0)) == 0 {
		return nil, fmt.Errorf("no rewards to claim")
	}

	return totalReward, nil
}

// ==================== 查询 ====================

// GetValidator 获取验证者
func (sc *StakingContract) GetValidator(addr string) (*Validator, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	v, ok := sc.validators[addr]
	return v, ok
}

// GetActiveValidators 获取活跃验证者
func (sc *StakingContract) GetActiveValidators() []*Validator {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	result := make([]*Validator, 0)
	for _, v := range sc.validators {
		if v.Status == ValidatorStatusActive {
			result = append(result, v)
		}
	}

	// 按质押量排序
	sort.Slice(result, func(i, j int) bool {
		totalI := new(big.Int).Add(result[i].Stake, result[i].Delegated)
		totalJ := new(big.Int).Add(result[j].Stake, result[j].Delegated)
		return totalI.Cmp(totalJ) > 0
	})

	return result
}

// GetDelegation 获取委托信息
func (sc *StakingContract) GetDelegation(delegator string) (*Delegation, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	del, ok := sc.delegatorMap[delegator]
	return del, ok
}

// GetTotalStaked 获取总质押
func (sc *StakingContract) GetTotalStaked() *big.Int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return new(big.Int).Set(sc.totalStaked)
}

// GetValidatorCount 获取验证者数量
func (sc *StakingContract) GetValidatorCount() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.validators)
}

// GetValidators 获取所有验证者
func (sc *StakingContract) GetValidators() []*Validator {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	result := make([]*Validator, 0, len(sc.validators))
	for _, v := range sc.validators {
		result = append(result, v)
	}
	return result
}

// GetStakingStats 获取质押统计
func (sc *StakingContract) GetStakingStats() StakingStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	activeCount := 0
	for _, v := range sc.validators {
		if v.Status == ValidatorStatusActive {
			activeCount++
		}
	}

	return StakingStats{
		TotalStaked:       new(big.Int).Set(sc.totalStaked),
		ValidatorCount:    uint32(len(sc.validators)),
		ActiveValidators:  uint32(activeCount),
		MaxValidators:   sc.config.MaxValidators,
		MinStake:        new(big.Int).Set(sc.config.MinStake),
	}
}

// StakingStats 质押统计
type StakingStats struct {
	TotalStaked       *big.Int
	ValidatorCount    uint32
	ActiveValidators  uint32
	MaxValidators    uint32
	MinStake         *big.Int
}
