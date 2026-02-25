package consensus

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== 共识配置 ====================

// Config 共识配置
type Config struct {
	BlockTime       uint64        // 区块时间 (秒)
	ProposeTimeout  uint64        // 提议超时 (秒)
	VoteTimeout     uint64        // 投票超时 (秒)
	MinStake        types.TokenAmount // 最小质押
	MaxValidators   uint32        // 最大验证者数量
	BlocksPerEpoch  uint64        // 每纪元区块数
}

func DefaultConfig() *Config {
	return &Config{
		BlockTime:      3,
		ProposeTimeout: 2,
		VoteTimeout:    2,
		MinStake:       types.MustNewDecimal("10000"), // 10000 POLE
		MaxValidators:  21,
		BlocksPerEpoch: 14400, // ~12小时
	}
}

// ==================== 共识步骤 ====================

// Step 共识步骤
type Step int

const (
	StepPropose Step = iota
	StepVote
	StepCommit
	StepNewRound
)

func (s Step) String() string {
	switch s {
	case StepPropose:
		return "propose"
	case StepVote:
		return "vote"
	case StepCommit:
		return "commit"
	case StepNewRound:
		return "new_round"
	default:
		return "unknown"
	}
}

// ==================== 共识状态 ====================

// State 共识状态
type State struct {
	Height  types.BlockHeight `json:"height"`
	Round   uint64           `json:"round"`
	Step    Step             `json:"step"`
	Proposer types.Address   `json:"proposer"`
}

func NewState() *State {
	return &State{
		Height:  0,
		Round:   0,
		Step:    StepNewRound,
		Proposer: types.Address{},
	}
}

// ==================== 混合共识 ====================

// HybridConsensus 混合共识 (PoS + PoLE)
type HybridConsensus struct {
	config        *Config
	state         *State
	pos           *POSConsensus
	pole          *PoleConsensus
	validators   *ValidatorSet
	mu            sync.RWMutex
}

// NewHybridConsensus 创建混合共识
func NewHybridConsensus(config *Config) *HybridConsensus {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &HybridConsensus{
		config:      config,
		state:       NewState(),
		pos:         NewPOSConsensus(),
		pole:        NewPoleConsensus(),
		validators:  NewValidatorSet(),
	}
}

// GetState 获取共识状态
func (c *HybridConsensus) GetState() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return *c.state
}

// UpdateValidators 更新验证者集合
func (c *HybridConsensus) UpdateValidators(validators []types.Validator) {
	c.validators.Update(validators)
}

// IsActiveValidator 检查是否为活跃验证者
func (c *HybridConsensus) IsActiveValidator(addr types.Address) bool {
	return c.validators.IsActive(addr)
}

// CalculateProposer 计算提议者
func (c *HybridConsensus) CalculateProposer(height types.BlockHeight) (types.Address, error) {
	active := c.validators.GetActive()
	if len(active) == 0 {
		return types.Address{}, fmt.Errorf("no active validators")
	}

	// 根据高度和质押确定性地选择提议者
	seed := uint64(height)
	for _, v := range active {
		seed += uint64(len(v.Address.Bytes()))
	}
	
	proposerIndex := seed % uint64(len(active))
	return active[proposerIndex].Address, nil
}

// AdvanceStep 推进共识步骤
func (c *HybridConsensus) AdvanceStep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	switch c.state.Step {
	case StepNewRound:
		c.state.Step = StepPropose
	case StepPropose:
		c.state.Step = StepVote
	case StepVote:
		c.state.Step = StepCommit
	case StepCommit:
		c.state.Height++
		c.state.Round = 0
		c.state.Step = StepNewRound
	}
}

// ProcessBlock 处理区块
func (c *HybridConsensus) ProcessBlock(block *types.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 验证提议者
	proposer, err := c.CalculateProposer(block.Header.Height)
	if err != nil {
		return err
	}
	
	if proposer != block.Header.Proposer {
		return fmt.Errorf("invalid proposer")
	}
	
	// 记录区块
	c.pos.RecordBlock(block.Header.Height)
	
	// 更新状态
	c.state.Height = block.Header.Height
	c.state.Round++
	c.state.Step = StepNewRound
	c.state.Proposer = proposer
	
	return nil
}

// CanProduceBlock 是否可以生产区块
func (c *HybridConsensus) CanProduceBlock() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.Step == StepPropose
}

// SubmitDataVote 提交数据投票
func (c *HybridConsensus) SubmitDataVote(voter types.Address, dataHash [32]byte, vote bool) error {
	return c.pole.SubmitVote(voter, dataHash, vote)
}

// GetDataVerificationResult 获取数据验证结果
func (c *HybridConsensus) GetDataVerificationResult(dataHash [32]byte) (bool, bool) {
	return c.pole.GetResult(dataHash)
}

// ==================== PoS 共识 ====================

// POSConsensus PoS 共识
type POSConsensus struct {
	lastHeight types.BlockHeight
	votes     map[types.Address][]Vote
	slashes   map[types.Address][]SlashRecord
	mu        sync.Mutex
}

// Vote 投票
type Vote struct {
	Height    types.BlockHeight `json:"height"`
	Round     uint64           `json:"round"`
	BlockHash [32]byte         `json:"block_hash"`
	VoteType  VoteType         `json:"vote_type"`
}

// VoteType 投票类型
type VoteType int

const (
	VoteTypePrevote VoteType = iota
	VoteTypePrecommit
)

// SlashRecord 削减记录
type SlashRecord struct {
	Height      types.BlockHeight `json:"height"`
	Reason      SlashReason       `json:"reason"`
	SlashAmount types.TokenAmount  `json:"slash_amount"`
}

// SlashReason 削减原因
type SlashReason int

const (
	SlashReasonDoubleSign SlashReason = iota
	SlashReasonUnavailable
	SlashReasonMalicious
)

// NewPOSConsensus 创建 PoS 共识
func NewPOSConsensus() *POSConsensus {
	return &POSConsensus{
		lastHeight: 0,
		votes:      make(map[types.Address][]Vote),
		slashes:    make(map[types.Address][]SlashRecord),
	}
}

// RecordBlock 记录区块
func (p *POSConsensus) RecordBlock(height types.BlockHeight) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if height <= p.lastHeight {
		return fmt.Errorf("invalid height")
	}
	p.lastHeight = height
	return nil
}

// RecordVote 记录投票
func (p *POSConsensus) RecordVote(validator types.Address, vote Vote) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.votes[validator] = append(p.votes[validator], vote)
	return nil
}

// Slash 削减验证者
func (p *POSConsensus) Slash(validator types.Address, record SlashRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.slashes[validator] = append(p.slashes[validator], record)
}

// GetSlashCount 获取削减次数
func (p *POSConsensus) GetSlashCount(validator types.Address) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	return len(p.slashes[validator])
}

// GetLastHeight 获取最后高度
func (p *POSConsensus) GetLastHeight() types.BlockHeight {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	return p.lastHeight
}

// ==================== PoLE 共识 ====================

// PoleConsensus PoLE 数据验证共识
type PoleConsensus struct {
	votes       map[[32]byte][]DataVote
	results     map[[32]byte]VerificationResult
	voteCounts  map[[32]byte](struct{ yes, no uint64 })
	quorum      float64
	threshold   float64
	mu          sync.Mutex
}

// DataVote 数据投票
type DataVote struct {
	Voter    types.NodeID `json:"voter"`
	DataHash [32]byte    `json:"data_hash"`
	Vote     bool         `json:"vote"`
	Stake    types.TokenAmount `json:"stake"`
	Time     int64        `json:"time"`
}

// VerificationResult 验证结果
type VerificationResult int

const (
	VerificationPending VerificationResult = iota
	VerificationApproved
	VerificationRejected
	VerificationInvalid
)

// NewPoleConsensus 创建 PoLE 共识
func NewPoleConsensus() *PoleConsensus {
	return &PoleConsensus{
		votes:      make(map[[32]byte][]DataVote),
		results:    make(map[[32]byte]VerificationResult),
		voteCounts: make(map[[32]byte](struct{ yes, no uint64 })),
		quorum:    0.67,  // 2/3 多数
		threshold: 0.67,  // 批准阈值
	}
}

// SubmitVote 提交投票
func (p *PoleConsensus) SubmitVote(voter types.Address, dataHash [32]byte, vote bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	voteData := DataVote{
		Voter:    types.NewNode(voter).ID,
		DataHash: dataHash,
		Vote:     vote,
		Time:     time.Now().Unix(),
	}
	
	p.votes[dataHash] = append(p.votes[dataHash], voteData)
	
	// 更新计数
	counts := p.voteCounts[dataHash]
	if vote {
		counts.yes++
	} else {
		counts.no++
	}
	p.voteCounts[dataHash] = counts
	
	// 检查是否可以确定结果
	p.checkAndSetResult(dataHash)
	
	return nil
}

// checkAndSetResult 检查并设置结果
func (p *PoleConsensus) checkAndSetResult(dataHash [32]byte) {
	votes := p.votes[dataHash]
	if len(votes) < 3 {
		return
	}
	
	totalVotes := float64(len(votes))
	yesVotes := 0
	for _, v := range votes {
		if v.Vote {
			yesVotes++
		}
	}
	
	yesRatio := float64(yesVotes) / totalVotes
	
	var result VerificationResult
	if yesRatio >= p.threshold {
		result = VerificationApproved
	} else if (1.0 - yesRatio) >= p.threshold {
		result = VerificationRejected
	} else {
		result = VerificationPending
	}
	
	p.results[dataHash] = result
}

// GetResult 获取验证结果
func (p *PoleConsensus) GetResult(dataHash [32]byte) (bool, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	result, ok := p.results[dataHash]
	if !ok {
		return false, false
	}
	
	switch result {
	case VerificationApproved:
		return true, true
	case VerificationRejected:
		return false, true
	default:
		return false, false
	}
}

// GetVoteCount 获取投票数
func (p *PoleConsensus) GetVoteCount(dataHash [32]byte) (uint64, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	counts := p.voteCounts[dataHash]
	return counts.yes, counts.no
}

// ==================== 验证者集合 ====================

// ValidatorSet 验证者集合
type ValidatorSet struct {
	validators      map[types.Address]types.Validator
	activeValidators []types.Address
	mu              sync.RWMutex
}

// NewValidatorSet 创建验证者集合
func NewValidatorSet() *ValidatorSet {
	return &ValidatorSet{
		validators:      make(map[types.Address]types.Validator),
		activeValidators: make([]types.Address, 0),
	}
}

// AddValidator 添加验证者
func (v *ValidatorSet) AddValidator(validator types.Validator) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.validators[validator.Address] = validator
	
	if validator.Status == types.ValidatorStatusActive {
		v.activeValidators = append(v.activeValidators, validator.Address)
	}
}

// RemoveValidator 移除验证者
func (v *ValidatorSet) RemoveValidator(addr types.Address) types.Validator {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	validator, ok := v.validators[addr]
	if ok {
		delete(v.validators, addr)
		// 从活跃列表中移除
		for i, a := range v.activeValidators {
			if a == addr {
				v.activeValidators = append(v.activeValidators[:i], v.activeValidators[i+1:]...)
				break
			}
		}
	}
	return validator
}

// Get 获取验证者
func (v *ValidatorSet) Get(addr types.Address) (types.Validator, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	validator, ok := v.validators[addr]
	return validator, ok
}

// IsActive 检查是否为活跃验证者
func (v *ValidatorSet) IsActive(addr types.Address) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	if validator, ok := v.validators[addr]; ok {
		return validator.Status == types.ValidatorStatusActive
	}
	return false
}

// GetActive 获取所有活跃验证者
func (v *ValidatorSet) GetActive() []types.Validator {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	result := make([]types.Validator, 0, len(v.activeValidators))
	for _, addr := range v.activeValidators {
		if v, ok := v.validators[addr]; ok {
			result = append(result, v)
		}
	}
	return result
}

// GetAll 获取所有验证者
func (v *ValidatorSet) GetAll() []types.Validator {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	result := make([]types.Validator, 0, len(v.validators))
	for _, v := range v.validators {
		result = append(result, v)
	}
	return result
}

// Update 更新验证者列表
func (v *ValidatorSet) Update(validators []types.Validator) {
	v.mu.Lock()
	defer v.mu.Unlock()
	
	v.validators = make(map[types.Address]types.Validator)
	v.activeValidators = make([]types.Address, 0)
	
	for _, validator := range validators {
		v.validators[validator.Address] = validator
		if validator.Status == types.ValidatorStatusActive {
			v.activeValidators = append(v.activeValidators, validator.Address)
		}
	}
}

// Count 获取验证者数量
func (v *ValidatorSet) Count() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	return len(v.validators)
}

// ActiveCount 获取活跃验证者数量
func (v *ValidatorSet) ActiveCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	return len(v.activeValidators)
}

// TotalStake 获取总质押
func (v *ValidatorSet) TotalStake() types.TokenAmount {
	v.mu.RLock()
	defer v.mu.RUnlock()
	
	var total types.TokenAmount
	for _, validator := range v.validators {
		total = total.Add(validator.Stake)
		total = total.Add(validator.Delegations)
	}
	return total
}

// ==================== 区块生产 ====================

// BlockProducer 区块生产者
type BlockProducer struct {
	height         types.BlockHeight
	lastBlockHash  [32]byte
	proposer       types.Address
	maxTxs         int
	maxDataPoints  int
	gasLimit       uint64
}

// NewBlockProducer 创建区块生产者
func NewBlockProducer(proposer types.Address) *BlockProducer {
	return &BlockProducer{
		height:        0,
		lastBlockHash: [32]byte{},
		proposer:      proposer,
		maxTxs:        1000,
		maxDataPoints: 5000,
		gasLimit:      50000000,
	}
}

// CreateBlock 创建区块
func (bp *BlockProducer) CreateBlock(
	txs []types.Transaction,
	gvsUpdates []types.GVSUpdate,
) (*types.Block, error) {
	if len(txs) > bp.maxTxs {
		return nil, fmt.Errorf("too many transactions")
	}
	if len(gvsUpdates) > bp.maxDataPoints {
		return nil, fmt.Errorf("too many data points")
	}
	
	bp.height++
	
	header := types.NewBlockHeader(bp.height, bp.lastBlockHash, bp.proposer)
	header.DataHash = bp.calculateDataHash(txs)
	header.GVSHash = bp.calculateGVSHash(gvsUpdates)
	header.ValidatorsHash = bp.calculateValidatorsHash()
	
	block := types.NewBlock(*header)
	block.Transactions = txs
	block.GVSUpdates = gvsUpdates
	
	bp.lastBlockHash = bp.calculateBlockHash(header)
	
	return block, nil
}

func (bp *BlockProducer) calculateBlockHash(header *types.BlockHeader) [32]byte {
	data := make([]byte, 0)
	data = append(data, uint64ToBytes(uint64(header.Height))...)
	data = append(data, header.ParentHash[:]...)
	data = append(data, uint64ToBytes(uint64(header.Timestamp))...)
	data = append(data, header.Proposer.Bytes()...)
	data = append(data, header.ValidatorsHash[:]...)
	data = append(data, header.DataHash[:]...)
	data = append(data, header.GVSHash[:]...)
	
	h := sha256.Sum256(data)
	return h
}

func (bp *BlockProducer) calculateDataHash(txs []types.Transaction) [32]byte {
	h := sha256.New()
	for _, tx := range txs {
		data, _ := json.Marshal(tx)
		h.Write(data)
	}
	result := h.Sum(nil)
	var hash [32]byte
	copy(hash[:], result)
	return hash
}

func (bp *BlockProducer) calculateGVSHash(updates []types.GVSUpdate) [32]byte {
	h := sha256.New()
	for _, u := range updates {
		data := fmt.Sprintf("%s:%.2f:%d:%d", u.GameID, u.GVS, u.Tier, u.Updated)
		h.Write([]byte(data))
	}
	result := h.Sum(nil)
	var hash [32]byte
	copy(hash[:], result)
	return hash
}

func (bp *BlockProducer) calculateValidatorsHash() [32]byte {
	// 简化版本
	return [32]byte{}
}

func (bp *BlockProducer) Height() types.BlockHeight {
	return bp.height
}

func (bp *BlockProducer) LastBlockHash() [32]byte {
	return bp.lastBlockHash
}

// ==================== 工具函数 ====================

func jsonMarshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

func sha256Sum(data []byte) [32]byte {
	h := sha256.Sum256(data)
	return h
}

// Uint64ToBytes 将 uint64 转换为字节数组
func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[7-i] = byte(v >> (i * 8))
	}
	return b
}
