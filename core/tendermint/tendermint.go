package tendermint

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"pole-core/core/types"
)

// ==================== Tendermint 共识配置 ====================

// Config Tendermint 配置
type Config struct {
	BlockTime         time.Duration // 区块时间 (默认3秒)
	ProposeTimeout    time.Duration // 提议超时
	PrevoteTimeout    time.Duration // 预投票超时
	PrecommitTimeout  time.Duration // 预提交超时
	MaxBlockSize     int           // 最大区块大小 (字节)
	MaxValidators    int           // 最大验证者数量
	QuorumRatio      float64       // 法定人数比例 (默认 2/3)
	FastFinality    bool          // 启用快速最终性
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		BlockTime:        3 * time.Second,
		ProposeTimeout:   2 * time.Second,
		PrevoteTimeout:   2 * time.Second,
		PrecommitTimeout: 2 * time.Second,
		MaxBlockSize:     10 * 1024 * 1024, // 10MB
		MaxValidators:    21,
		QuorumRatio:      0.67,
		FastFinality:     true,
	}
}

// ==================== 共识步骤 ====================

// RoundStep 共识步骤
type RoundStep int

const (
	RoundStepNewHeight RoundStep = iota
	RoundStepNewRound
	RoundStepPropose
	RoundStepPrevote
	RoundStepPrecommit
	RoundStepCommit
)

func (rs RoundStep) String() string {
	switch rs {
	case RoundStepNewHeight:
		return "NewHeight"
	case RoundStepNewRound:
		return "NewRound"
	case RoundStepPropose:
		return "Propose"
	case RoundStepPrevote:
		return "Prevote"
	case RoundStepPrecommit:
		return "Precommit"
	case RoundStepCommit:
		return "Commit"
	default:
		return "Unknown"
	}
}

// ==================== 投票类型 ====================

// VoteType 投票类型
type VoteType int

const (
	VoteTypeNil VoteType = iota
	VoteTypePrevote
	VoteTypePrecommit
)

func (vt VoteType) String() string {
	switch vt {
	case VoteTypeNil:
		return "nil"
	case VoteTypePrevote:
		return "prevote"
	case VoteTypePrecommit:
		return "precommit"
	default:
		return "unknown"
	}
}

// Vote 投票
type Vote struct {
	Type      VoteType      `json:"type"`
	Height    uint64        `json:"height"`
	Round     uint64        `json:"round"`
	Timestamp int64         `json:"timestamp"`
	BlockID   types.BlockID `json:"block_id"`
	Validator types.Address `json:"validator"`
	Signature []byte       `json:"signature"`
}

// NewVote 创建新投票
func NewVote(voteType VoteType, height, round uint64, blockID types.BlockID, validator types.Address) *Vote {
	return &Vote{
		Type:      voteType,
		Height:    height,
		Round:     round,
		Timestamp: time.Now().UnixNano(),
		BlockID:  blockID,
		Validator: validator,
	}
}

// ==================== 提案 ====================

// Proposal 区块提案
type Proposal struct {
	Height     uint64         `json:"height"`
	Round     uint64         `json:"round"`
	Timestamp int64          `json:"timestamp"`
	Block     *types.Block  `json:"block"`
	BlockID   types.BlockID `json:"block_id"`
	Proposer  types.Address `json:"proposer"`
	POLRound  int64         `json:"pol_round"` // Proof of Lock round
	Signature []byte        `json:"signature"`
}

// NewProposal 创建新提案
func NewProposal(height, round uint64, block *types.Block, proposer types.Address) *Proposal {
	blockID := types.NewBlockID(block)
	return &Proposal{
		Height:     height,
		Round:     round,
		Timestamp: time.Now().UnixNano(),
		Block:     block,
		BlockID:  blockID,
		Proposer: proposer,
	}
}

// ==================== 共识状态 ====================

// State 共识状态
type State struct {
	Height                uint64         // 当前高度
	Round                 uint64         // 当前轮次
	Step                  RoundStep      // 当前步骤
	StartTime             time.Time      // 当前步骤开始时间
	Proposer              types.Address  // 当前提议者
	Votes                 *VoteSet       // 投票集合
	LockedBlock           *types.Block   // 锁定区块
	LockedRound           int64          // 锁定轮次
	ValidBlock            *types.Block   // 有效区块
	ValidRound           int64          // 有效轮次
	Proposal             *Proposal      // 当前提案
	LastCommit           *Commit        // 上一个区块的提交
	LastValidators       *ValidatorSet  // 上一个高度验证者集合
	LastBlock            *types.Block  // 上一个区块
}

// NewState 创建新状态
func NewState() *State {
	return &State{
		Height:         1,
		Round:          0,
		Step:           RoundStepNewHeight,
		StartTime:      time.Now(),
		Votes:          NewVoteSet(21),
		LockedRound:    -1,
		ValidRound:     -1,
		LastValidators: NewValidatorSet(),
	}
}

// ==================== 投票集合 ====================

// VoteSet 投票集合
type VoteSet struct {
	height    uint64
	round     uint64
	voteType  VoteType
	votes     map[types.Address]*Vote
	mu        sync.RWMutex
}

// NewVoteSet 创建新投票集合
func NewVoteSet(validatorCount int) *VoteSet {
	return &VoteSet{
		votes: make(map[types.Address]*Vote, validatorCount),
	}
}

// AddVote 添加投票
func (vs *VoteSet) AddVote(vote *Vote) bool {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vote.Height != vs.height || vote.Round != vs.round || vote.Type != vs.voteType {
		return false
	}

	vs.votes[vote.Validator] = vote
	return true
}

// HasVote 检查是否有投票
func (vs *VoteSet) HasVote(validator types.Address) bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	_, exists := vs.votes[validator]
	return exists
}

// GetVote 获取投票
func (vs *VoteSet) GetVote(validator types.Address) *Vote {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return vs.votes[validator]
}

// HasTwoThirdsMajority 检查是否有 2/3 多数
func (vs *VoteSet) HasTwoThirdsMajority(quorumRatio float64) bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	totalVotes := len(vs.votes)
	if totalVotes == 0 {
		return false
	}

	required := int(float64(totalVotes) * quorumRatio)
	return totalVotes >= required
}

// GetVotes 获取所有投票
func (vs *VoteSet) GetVotes() []*Vote {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	votes := make([]*Vote, 0, len(vs.votes))
	for _, vote := range vs.votes {
		votes = append(votes, vote)
	}
	return votes
}

// ==================== 提交 ====================

// Commit 区块提交
type Commit struct {
	Height     uint64            `json:"height"`
	Round      uint64            `json:"round"`
	BlockID    types.BlockID     `json:"block_id"`
	Signatures []CommitSignature `json:"signatures"`
	Timestamp  int64             `json:"timestamp"`
}

// CommitSignature 提交签名
type CommitSignature struct {
	Validator types.Address `json:"validator"`
	Signature []byte       `json:"signature"`
}

// NewCommit 创建新提交
func NewCommit(height, round uint64, blockID types.BlockID) *Commit {
	return &Commit{
		Height:     height,
		Round:      round,
		BlockID:    blockID,
		Signatures: make([]CommitSignature, 0),
		Timestamp:  time.Now().UnixNano(),
	}
}

// AddSignature 添加签名
func (c *Commit) AddSignature(validator types.Address, signature []byte) {
	c.Signatures = append(c.Signatures, CommitSignature{
		Validator: validator,
		Signature: signature,
	})
}

// HasEnoughSignatures 检查是否有足够签名
func (c *Commit) HasEnoughSignatures(quorumRatio float64) bool {
	totalValidators := len(c.Signatures)
	if totalValidators == 0 {
		return false
	}
	required := int(float64(totalValidators) * quorumRatio)
	return len(c.Signatures) >= required
}

// ==================== 验证者集合 ====================

// Validator 验证者信息
type Validator struct {
	Address      types.Address     `json:"address"`
	PubKey       []byte            `json:"pub_key"`
	VotingPower  int64             `json:"voting_power"` // 投票权重 (质押量)
	Stake        types.TokenAmount `json:"stake"`
	Commission   uint8             `json:"commission"`
	Status       ValidatorStatus   `json:"status"`
}

// ValidatorStatus 验证者状态
type ValidatorStatus int

const (
	ValidatorStatusActive ValidatorStatus = iota
	ValidatorStatusInactive
	ValidatorStatusJailed
	ValidatorStatusUnbonding
)

// ValidatorSet 验证者集合
type ValidatorSet struct {
	validators        []*Validator
	activeValidators  []*Validator
	totalVotingPower  int64
	proposerSelection *big.Int // 用于提议者选择
	mu                sync.RWMutex
}

// NewValidatorSet 创建验证者集合
func NewValidatorSet() *ValidatorSet {
	return &ValidatorSet{
		validators:        make([]*Validator, 0),
		activeValidators:  make([]*Validator, 0),
		totalVotingPower: 0,
		proposerSelection: big.NewInt(0),
	}
}

// Add 添加验证者
func (vs *ValidatorSet) Add(validator *Validator) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.validators = append(vs.validators, validator)
	if validator.Status == ValidatorStatusActive {
		vs.activeValidators = append(vs.activeValidators, validator)
		vs.totalVotingPower += validator.VotingPower
	}
}

// Remove 移除验证者
func (vs *ValidatorSet) Remove(addr types.Address) *Validator {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	for i, v := range vs.validators {
		if v.Address == addr {
			vs.validators = append(vs.validators[:i], vs.validators[i+1:]...)
			vs.recalcActive()
			return v
		}
	}
	return nil
}

// recalcActive 重新计算活跃验证者
func (vs *ValidatorSet) recalcActive() {
	vs.activeValidators = make([]*Validator, 0)
	vs.totalVotingPower = 0
	for _, v := range vs.validators {
		if v.Status == ValidatorStatusActive {
			vs.activeValidators = append(vs.activeValidators, v)
			vs.totalVotingPower += v.VotingPower
		}
	}
}

// GetProposer 获取提议者
func (vs *ValidatorSet) GetProposer(height, round uint64) *Validator {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if len(vs.activeValidators) == 0 {
		return nil
	}

	// 使用 VRF 确定性地选择提议者
	seed := height*1000 + round
	proposerIndex := int(seed) % len(vs.activeValidators)

	return vs.activeValidators[proposerIndex]
}

// GetActiveValidators 获取活跃验证者
func (vs *ValidatorSet) GetActiveValidators() []*Validator {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	result := make([]*Validator, len(vs.activeValidators))
	copy(result, vs.activeValidators)
	return result
}

// GetTotalVotingPower 获取总投票权重
func (vs *ValidatorSet) GetTotalVotingPower() int64 {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return vs.totalVotingPower
}

// HasTwoThirdsPower 检查是否有 2/3 投票权
func (vs *ValidatorSet) HasTwoThirdsPower(power int64) bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return power >= vs.totalVotingPower*2/3
}

// ==================== Tendermint 引擎 ====================

// Engine Tendermint 共识引擎
type Engine struct {
	config   *Config
	state    *State
	valSet   *ValidatorSet
	blockDB  BlockStore
	txPool   TxPool
	privKey  []byte // 验证者私钥
	mu       sync.RWMutex
	// 事件
	onPropose      func(*Proposal)
	onVote         func(*Vote)
	onBlockCommit  func(*types.Block)
	// 状态
	isRunning bool
	quitCh    chan struct{}
}

// NewEngine 创建 Tendermint 引擎
func NewEngine(config *Config, valSet *ValidatorSet) *Engine {
	if config == nil {
		config = DefaultConfig()
	}

	return &Engine{
		config:   config,
		state:    NewState(),
		valSet:   valSet,
		blockDB:  *NewBlockStore(),
		txPool:   *NewTxPool(),
		quitCh:   make(chan struct{}),
	}
}

// SetPrivKey 设置验证者私钥
func (e *Engine) SetPrivKey(privKey []byte) {
	e.privKey = privKey
}

// Start 启动共识
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isRunning {
		return fmt.Errorf("engine already running")
	}

	e.isRunning = true
	go e.runConsensus()

	return nil
}

// Stop 停止共识
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isRunning {
		return fmt.Errorf("engine not running")
	}

	e.isRunning = false
	close(e.quitCh)

	return nil
}

// IsRunning 检查是否运行
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.isRunning
}

// runConsensus 运行共识循环
func (e *Engine) runConsensus() {
	ticker := time.NewTicker(e.config.BlockTime)
	defer ticker.Stop()

	for {
		select {
		case <-e.quitCh:
			return
		case <-ticker.C:
			e.executeRound()
		}
	}
}

// executeRound 执行一轮共识
func (e *Engine) executeRound() {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch e.state.Step {
	case RoundStepNewHeight:
		e.startNewHeight()

	case RoundStepNewRound:
		e.startNewRound()

	case RoundStepPropose:
		if e.state.Proposal != nil {
			e.state.Step = RoundStepPrevote
			e.state.StartTime = time.Now()
		}

	case RoundStepPrevote:
		// 等待 prevote 投票
		if e.state.Votes.HasTwoThirdsMajority(e.config.QuorumRatio) {
			e.state.Step = RoundStepPrecommit
			e.state.StartTime = time.Now()
		}

	case RoundStepPrecommit:
		// 等待 precommit 投票
		if e.state.Votes.HasTwoThirdsMajority(e.config.QuorumRatio) {
			e.commitBlock()
			e.state.Height++
			e.state.Round = 0
			e.state.Step = RoundStepNewHeight
			e.state.StartTime = time.Now()
		}

	case RoundStepCommit:
		e.state.Height++
		e.state.Round = 0
		e.state.Step = RoundStepNewHeight
		e.state.StartTime = time.Now()
	}
}

// startNewHeight 开始新区块高度
func (e *Engine) startNewHeight() {
	e.state.Height++
	e.state.Round = 0
	e.state.Step = RoundStepNewRound
	e.state.StartTime = time.Now()
	e.state.LastValidators = e.valSet
}

// startNewRound 开始新轮次
func (e *Engine) startNewRound() {
	e.state.Round++
	e.state.Step = RoundStepPropose
	e.state.StartTime = time.Now()

	// 选择提议者
	proposer := e.valSet.GetProposer(e.state.Height, e.state.Round)
	if proposer != nil {
		e.state.Proposer = proposer.Address
	}

	// 如果是当前验证者，创建提案
	if e.isCurrentValidator() && e.state.Proposal == nil {
		e.createProposal()
	}
}

// isCurrentValidator 检查是否为当前验证者
func (e *Engine) isCurrentValidator() bool {
	if e.state.Proposal == nil {
		return false
	}
	return e.state.Proposal.Proposer == e.state.Proposer
}

// createProposal 创建区块提案
func (e *Engine) createProposal() {
	// 从交易池获取交易
	txs := e.txPool.GetTxs(1000)

	// 创建区块
	block := e.createBlock(txs)

	// 创建提案
	proposal := NewProposal(e.state.Height, e.state.Round, block, e.state.Proposer)
	e.state.Proposal = proposal

	// 触发提案事件
	if e.onPropose != nil {
		e.onPropose(proposal)
	}
}

// createBlock 创建区块
func (e *Engine) createBlock(txs []types.Transaction) *types.Block {
	header := types.NewBlockHeader(
		types.BlockHeight(e.state.Height),
		e.state.LastBlock.Header.ParentHash,
		e.state.Proposer,
	)

	header.Timestamp = time.Now().Unix()

	block := types.NewBlock(*header)
	block.Transactions = txs

	// 计算区块哈希
	blockHash := e.calculateBlockHash(block)
	block.Header.ParentHash = blockHash

	return block
}

// calculateBlockHash 计算区块哈希
func (e *Engine) calculateBlockHash(block *types.Block) [32]byte {
	data, _ := json.Marshal(block)
	h := sha256.Sum256(data)
	return h
}

// commitBlock 提交区块
func (e *Engine) commitBlock() {
	if e.state.Proposal == nil || e.state.Proposal.Block == nil {
		return
	}

	block := e.state.Proposal.Block

	// 保存区块
	e.blockDB.SaveBlock(block)

	// 更新状态
	e.state.LastBlock = block
	e.state.LockedBlock = nil
	e.state.LockedRound = -1
	e.state.ValidBlock = nil
	e.state.ValidRound = -1

	// 创建提交
	commit := NewCommit(e.state.Height, e.state.Round, types.NewBlockID(block))
	e.state.LastCommit = commit

	// 触发区块提交事件
	if e.onBlockCommit != nil {
		e.onBlockCommit(block)
	}
}

// ==================== 投票处理 ====================

// SubmitVote 提交投票
func (e *Engine) SubmitVote(voteType VoteType, blockID types.BlockID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isCurrentValidator() {
		return fmt.Errorf("not current validator")
	}

	vote := NewVote(voteType, e.state.Height, e.state.Round, blockID, e.state.Proposer)

	// 签名
	if e.privKey != nil {
		vote.Signature = e.signVote(vote)
	}

	// 添加到投票集合
	e.state.Votes.AddVote(vote)

	// 触发投票事件
	if e.onVote != nil {
		e.onVote(vote)
	}

	return nil
}

// signVote 签名投票
func (e *Engine) signVote(vote *Vote) []byte {
	data := fmt.Sprintf("%d%d%d%s", vote.Type, vote.Height, vote.Round, vote.BlockID.String())
	h := sha256.Sum256([]byte(data))
	return h[:]
}

// HandleProposal 处理提案
func (e *Engine) HandleProposal(proposal *Proposal) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 验证提案
	if proposal.Height != e.state.Height {
		return fmt.Errorf("invalid height")
	}

	if proposal.Round != e.state.Round {
		return fmt.Errorf("invalid round")
	}

	// 验证提议者
	proposer := e.valSet.GetProposer(proposal.Height, proposal.Round)
	if proposer == nil || proposer.Address != proposal.Proposer {
		return fmt.Errorf("invalid proposer")
	}

	// 保存提案
	e.state.Proposal = proposal

	// 如果有锁定区块，检查是否应该解锁
	if e.state.LockedBlock != nil {
		if bytes.Equal(e.state.LockedBlock.Header.ParentHash[:], proposal.BlockID[:]) {
			// 解锁
			e.state.LockedBlock = nil
			e.state.LockedRound = -1
		}
	}

	return nil
}

// HandleVote 处理投票
func (e *Engine) HandleVote(vote *Vote) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 验证投票
	if vote.Height != e.state.Height {
		return fmt.Errorf("invalid height")
	}

	// 添加到投票集合
	e.state.Votes.AddVote(vote)

	// 检查是否可以锁定区块
	if vote.Type == VoteTypePrevote && e.state.Proposal != nil {
		if vote.BlockID == e.state.Proposal.BlockID {
			e.state.LockedBlock = e.state.Proposal.Block
			e.state.LockedRound = int64(e.state.Round)
		}
	}

	return nil
}

// ==================== 快速最终性 ====================

// FastFinality 快速最终性确认
func (e *Engine) FastFinality() bool {
	if !e.config.FastFinality {
		return false
	}

	// 检查是否有足够的预提交
	return e.state.Votes.HasTwoThirdsMajority(e.config.QuorumRatio)
}

// GetFinalityStatus 获取最终性状态
func (e *Engine) GetFinalityStatus() FinalityStatus {
	if e.state.LastCommit == nil {
		return FinalityStatusUnknown
	}

	if e.state.LastCommit.HasEnoughSignatures(e.config.QuorumRatio) {
		return FinalityStatusFinalized
	}

	if e.FastFinality() {
		return FinalityStatusFast
	}

	return FinalityStatusPending
}

// FinalityStatus 最终性状态
type FinalityStatus int

const (
	FinalityStatusUnknown FinalityStatus = iota
	FinalityStatusPending
	FinalityStatusFast
	FinalityStatusFinalized
)

// ==================== 区块存储 ====================

// BlockStore 区块存储
type BlockStore struct {
	blocks map[uint64]*types.Block
	mu     sync.RWMutex
}

// NewBlockStore 创建区块存储
func NewBlockStore() *BlockStore {
	return &BlockStore{
		blocks: make(map[uint64]*types.Block),
	}
}

// SaveBlock 保存区块
func (bs *BlockStore) SaveBlock(block *types.Block) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.blocks[uint64(block.Header.Height)] = block
}

// GetBlock 获取区块
func (bs *BlockStore) GetBlock(height uint64) *types.Block {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return bs.blocks[height]
}

// GetLastBlock 获取最后一个区块
func (bs *BlockStore) GetLastBlock() *types.Block {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var lastBlock *types.Block
	for _, block := range bs.blocks {
		if lastBlock == nil || block.Header.Height > lastBlock.Header.Height {
			lastBlock = block
		}
	}
	return lastBlock
}

// ==================== 交易池 ====================

// TxPool 交易池
type TxPool struct {
	txs    []types.Transaction
	mu     sync.RWMutex
	maxTxs int
}

// NewTxPool 创建交易池
func NewTxPool() *TxPool {
	return &TxPool{
		txs:    make([]types.Transaction, 0),
		maxTxs: 10000,
	}
}

// AddTx 添加交易
func (tp *TxPool) AddTx(tx types.Transaction) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if len(tp.txs) >= tp.maxTxs {
		tp.txs = tp.txs[1:]
	}
	tp.txs = append(tp.txs, tx)
}

// GetTxs 获取交易
func (tp *TxPool) GetTxs(max int) []types.Transaction {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	if max > len(tp.txs) {
		max = len(tp.txs)
	}
	result := make([]types.Transaction, max)
	copy(result, tp.txs[:max])
	return result
}

// Size 获取交易池大小
func (tp *TxPool) Size() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	return len(tp.txs)
}

// ==================== 扩展模块：DPoS 委托 ====================

// Delegator 委托者
type Delegator struct {
	Address   types.Address     `json:"address"`
	Validator types.Address     `json:"validator"`
	Amount    types.TokenAmount `json:"amount"`
	StartTime int64             `json:"start_time"`
	EndTime   int64             `json:"end_time"`
}

// DelegationStore 委托存储
type DelegationStore struct {
	delegations map[types.Address][]Delegator
	mu          sync.RWMutex
}

// NewDelegationStore 创建委托存储
func NewDelegationStore() *DelegationStore {
	return &DelegationStore{
		delegations: make(map[types.Address][]Delegator),
	}
}

// Delegate 委托
func (ds *DelegationStore) Delegate(delegator, validator types.Address, amount types.TokenAmount, duration int64) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	delegation := Delegator{
		Address:   delegator,
		Validator: validator,
		Amount:    amount,
		StartTime: time.Now().Unix(),
		EndTime:   time.Now().Unix() + duration,
	}

	ds.delegations[delegator] = append(ds.delegations[delegator], delegation)
	return nil
}

// Undelegate 解除委托
func (ds *DelegationStore) Undelegate(delegator types.Address, amount types.TokenAmount) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	delegations := ds.delegations[delegator]
	for i, d := range delegations {
		dAmt := decimal.Decimal(d.Amount)
		aAmt := decimal.Decimal(amount)
		if dAmt.GreaterThanOrEqual(aAmt) {
			d.Amount = types.TokenAmount(dAmt.Sub(aAmt))
			delegations[i] = d
			break
		}
	}

	ds.delegations[delegator] = delegations
	return nil
}

// GetDelegations 获取委托
func (ds *DelegationStore) GetDelegations(delegator types.Address) []Delegator {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return ds.delegations[delegator]
}

// GetTotalDelegated 获取总委托量
func (ds *DelegationStore) GetTotalDelegated(validator types.Address) types.TokenAmount {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var total types.TokenAmount
	for _, delegations := range ds.delegations {
		for _, d := range delegations {
			if d.Validator == validator {
				total = total.Add(d.Amount)
			}
		}
	}
	return total
}

// ==================== 事件处理 ====================

// OnPropose 设置提案回调
func (e *Engine) OnPropose(handler func(*Proposal)) {
	e.onPropose = handler
}

// OnVote 设置投票回调
func (e *Engine) OnVote(handler func(*Vote)) {
	e.onVote = handler
}

// OnBlockCommit 设置区块提交回调
func (e *Engine) OnBlockCommit(handler func(*types.Block)) {
	e.onBlockCommit = handler
}

// ==================== 扩展：PoLE 数据验证集成 ====================

// PoLEDataVerifier PoLE 数据验证器
type PoLEDataVerifier struct {
	engine   *Engine
	minVotes int
	mu       sync.RWMutex
}

// NewPoLEDataVerifier 创建 PoLE 验证器
func NewPoLEDataVerifier(engine *Engine) *PoLEDataVerifier {
	return &PoLEDataVerifier{
		engine:   engine,
		minVotes: 3,
	}
}

// VerifyData 验证数据
func (pv *PoLEDataVerifier) VerifyData(dataHash [32]byte, votes []*Vote) (bool, bool) {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	if len(votes) < pv.minVotes {
		return false, false
	}

	yesVotes := 0
	for _, vote := range votes {
		if vote.BlockID.String() != "" {
			yesVotes++
		}
	}

	ratio := float64(yesVotes) / float64(len(votes))
	approved := ratio >= 0.67

	return approved, true
}

// ==================== 工具函数 ====================

// SignData 签名数据
func SignData(data []byte, privKey []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// VerifySignature 验证签名
func VerifySignature(data []byte, sig []byte, pubKey []byte) bool {
	expected := SignData(data, pubKey)
	return bytes.Equal(sig, expected)
}
