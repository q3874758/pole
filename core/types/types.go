package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ==================== 基础类型 ====================

// BlockHeight 区块高度
type BlockHeight uint64

// Epoch 纪元
type Epoch uint64

// TokenAmount 代币数量 (使用 decimal 处理高精度)
type TokenAmount decimal.Decimal

// GameID 游戏 ID
type GameID string

// NodeID 节点 ID (32字节)
type NodeID [32]byte

// Signature 签名
type Signature []byte

// Address 地址 (32字节)
type Address [32]byte

// ChainID 链 ID
type ChainID struct {
	Name    string `json:"name"`
	ChainID uint64 `json:"chain_id"`
}

func Mainnet() ChainID {
	return ChainID{Name: "pole-mainnet", ChainID: 1}
}

func Testnet() ChainID {
	return ChainID{Name: "pole-testnet", ChainID: 2}
}

// ==================== 区块 ====================

// BlockHeader 区块头
type BlockHeader struct {
	Height         BlockHeight `json:"height"`
	ParentHash     [32]byte   `json:"parent_hash"`
	Timestamp      int64      `json:"timestamp"`
	Proposer       Address    `json:"proposer"`
	ValidatorsHash [32]byte   `json:"validators_hash"`
	DataHash       [32]byte   `json:"data_hash"`
	GVSHash        [32]byte   `json:"gvs_hash"`
}

// NewBlockHeader 创建新区块头
func NewBlockHeader(height BlockHeight, parentHash [32]byte, proposer Address) *BlockHeader {
	return &BlockHeader{
		Height:     height,
		ParentHash: parentHash,
		Timestamp:  time.Now().Unix(),
		Proposer:   proposer,
	}
}

// Block 区块
type Block struct {
	Header       BlockHeader     `json:"header"`
	Transactions []Transaction  `json:"transactions"`
	GVSUpdates   []GVSUpdate    `json:"gvs_updates"`
}

// NewBlock 创建新区块
func NewBlock(header BlockHeader) *Block {
	return &Block{
		Header:       header,
		Transactions: make([]Transaction, 0),
		GVSUpdates:   make([]GVSUpdate, 0),
	}
}

// BlockID 区块ID
type BlockID [32]byte

// NewBlockID 创建区块ID
func NewBlockID(block *Block) BlockID {
	data, _ := json.Marshal(block)
	h := sha256.Sum256(data)
	return h
}

func (b BlockID) String() string {
	return hex.EncodeToString(b[:])
}

// ==================== 交易 ====================

// TransactionType 交易类型
type TransactionType int

const (
	TxTransfer TransactionType = iota
	TxStake
	TxUnstake
	TxSubmitData
	TxVote
	TxExecute
)

// Transaction 交易
type Transaction struct {
	Type       TransactionType `json:"type"`
	From       Address         `json:"from"`         // 交易发起者地址
	Nonce      uint64          `json:"nonce"`        // 防重放 nonce
	Fee        TokenAmount     `json:"fee"`          // 手续费
	Signature  Signature       `json:"signature"`    // 签名
	Transfer   *TransferTx     `json:"transfer,omitempty"`
	Stake      *StakeTx        `json:"stake,omitempty"`
	Unstake    *UnstakeTx      `json:"unstake,omitempty"`
	SubmitData *DataSubmitTx   `json:"submit_data,omitempty"`
	Vote       *VoteTx         `json:"vote,omitempty"`
	Execute    *ExecuteTx      `json:"execute,omitempty"`
}

// SignBytes 生成交易签名 payload
func (tx *Transaction) SignBytes() []byte {
	// 简化：使用 JSON 序列化（不含 signature 字段）
	data, _ := json.Marshal(struct {
		Type  TransactionType `json:"type"`
		From  Address        `json:"from"`
		Nonce uint64         `json:"nonce"`
		Fee   TokenAmount    `json:"fee"`
		Transfer  *TransferTx  `json:"transfer,omitempty"`
		Stake     *StakeTx    `json:"stake,omitempty"`
		Unstake   *UnstakeTx  `json:"unstake,omitempty"`
		Vote      *VoteTx     `json:"vote,omitempty"`
	}{
		Type:     tx.Type,
		From:     tx.From,
		Nonce:    tx.Nonce,
		Fee:      tx.Fee,
		Transfer: tx.Transfer,
		Stake:    tx.Stake,
		Unstake:  tx.Unstake,
		Vote:     tx.Vote,
	})
	return data
}

// TransferTx 转账交易
type TransferTx struct {
	From  Address    `json:"from"`
	To    Address    `json:"to"`
	Amount TokenAmount `json:"amount"`
	Fee    TokenAmount `json:"fee"`
}

// StakeTx 质押交易
type StakeTx struct {
	Delegator  Address    `json:"delegator"`
	Validator  Address    `json:"validator"`
	Amount     TokenAmount `json:"amount"`
}

// UnstakeTx 解除质押交易
type UnstakeTx struct {
	Delegator  Address    `json:"delegator"`
	Validator  Address    `json:"validator"`
	Amount     TokenAmount `json:"amount"`
}

// DataSubmitTx 数据提交交易
type DataSubmitTx struct {
	NodeID    NodeID          `json:"node_id"`
	GameData  []GameDataPoint `json:"game_data"`
	Signature Signature       `json:"signature"`
}

// VoteTx 投票交易
type VoteTx struct {
	Voter      Address    `json:"voter"`
	ProposalID uint64     `json:"proposal_id"`
	VoteOption uint8      `json:"vote_option"` // 0: abstain, 1: yes, 2: no
	Weight     TokenAmount `json:"weight"`
}

// ExecuteTx 执行交易
type ExecuteTx struct {
	From       Address    `json:"from"`
	ContractID uint64     `json:"contract_id"`
	Payload    []byte     `json:"payload"`
	Fee        TokenAmount `json:"fee"`
}

// ==================== 游戏数据 ====================

// GameDataPoint 游戏数据点
type GameDataPoint struct {
	GameID        GameID `json:"game_id"`
	OnlinePlayers uint64 `json:"online_players"`
	PeakPlayers  uint64 `json:"peak_players"`
	Timestamp    int64  `json:"timestamp"`
	Tier         Tier   `json:"tier"`
}

// Tier 游戏层级
type Tier int

const (
	Tier1 Tier = iota + 1
	Tier2
	Tier3
)

// Weight 获取层级权重
func (t Tier) Weight() float64 {
	switch t {
	case Tier1:
		return 1.0
	case Tier2:
		return 0.45
	case Tier3:
		return 0.10
	default:
		return 0.0
	}
}

func (t Tier) String() string {
	switch t {
	case Tier1:
		return "Tier1"
	case Tier2:
		return "Tier2"
	case Tier3:
		return "Tier3"
	default:
		return "Unknown"
	}
}

// MarshalJSON 自定义 JSON 序列化
func (t Tier) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// ==================== GVS ====================

// GVSUpdate GVS 更新
type GVSUpdate struct {
	GameID   GameID   `json:"game_id"`
	GVS      float64  `json:"gvs"`
	Tier     Tier     `json:"tier"`
	Updated  int64    `json:"updated"`
}

// GVS 游戏价值评分
type GVS struct {
	GameID  GameID   `json:"game_id"`
	Score   float64  `json:"score"`
	Tier    Tier     `json:"tier"`
	Updated int64    `json:"updated"`
}

// ==================== 验证者 ====================

// Validator 验证者
type Validator struct {
	Address     Address     `json:"address"`
	PublicKey   []byte     `json:"public_key"`
	Stake       TokenAmount `json:"stake"`
	Delegations TokenAmount `json:"delegations"`
	Commission  uint8      `json:"commission"`
	Status      ValidatorStatus `json:"status"`
	Moniker     string     `json:"moniker"`
}

// ValidatorStatus 验证者状态
type ValidatorStatus int

const (
	ValidatorStatusActive ValidatorStatus = iota
	ValidatorStatusInactive
	ValidatorStatusJailed
	ValidatorStatusUnbonding
)

func (s ValidatorStatus) String() string {
	switch s {
	case ValidatorStatusActive:
		return "active"
	case ValidatorStatusInactive:
		return "inactive"
	case ValidatorStatusJailed:
		return "jailed"
	case ValidatorStatusUnbonding:
		return "unbonding"
	default:
		return "unknown"
	}
}

// ==================== 节点 ====================

// Node 节点
type Node struct {
	ID              NodeID     `json:"id"`
	Address         Address    `json:"address"`
	Stake           TokenAmount `json:"stake"`
	Reputation      float64    `json:"reputation"`
	CollectedGames []GameID   `json:"collected_games"`
	Uptime          float64    `json:"uptime"`
	LastHeartbeat  int64      `json:"last_heartbeat"`
}

// NewNode 创建新节点
func NewNode(address Address) *Node {
	var nodeID NodeID
	uuid := uuid.New()
	// 使用 UUID 的前16字节填充 NodeID
	copy(nodeID[:], uuid[:])
	
	return &Node{
		ID:              nodeID,
		Address:         address,
		Stake:           TokenAmount{},
		Reputation:      100.0,
		CollectedGames:  make([]GameID, 0),
		Uptime:          100.0,
		LastHeartbeat:   time.Now().Unix(),
	}
}

// ==================== 网络状态 ====================

// NetworkState 网络状态
type NetworkState struct {
	Height          BlockHeight            `json:"height"`
	Epoch           Epoch                  `json:"epoch"`
	TotalStake      TokenAmount            `json:"total_stake"`
	ActiveValidators uint32                 `json:"active_validators"`
	TotalNodes      uint32                 `json:"total_nodes"`
	GVS             map[GameID]float64     `json:"gvs"`
}

// NewNetworkState 创建网络状态
func NewNetworkState() *NetworkState {
	return &NetworkState{
		Height:     0,
		Epoch:      0,
		TotalStake: TokenAmount{},
		GVS:        make(map[GameID]float64),
	}
}

// ==================== 地址工具 ====================

// FromPublicKey 从公钥生成地址
func FromPublicKey(pk []byte) Address {
	var addr Address
	// 简单使用 SHA256
	h := sha256Hash(pk)
	copy(addr[:], h[:32])
	return addr
}

// String 地址转字符串
func (a Address) String() string {
	return hex.EncodeToString(a[:])
}

// Bytes 获取字节
func (a Address) Bytes() []byte {
	return a[:]
}

// ==================== 工具函数 ====================

func sha256Hash(data []byte) [32]byte {
	h := sha256.New()
	h.Write(data)
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

func sha256Sum(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

// MustNewDecimal 创建 decimal
func MustNewDecimal(s string) TokenAmount {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return TokenAmount(d)
}

// NewDecimalFromInt 从整数创建
func NewDecimalFromInt(i int64) TokenAmount {
	return TokenAmount(decimal.NewFromInt(i))
}

// Add 加法
func (a TokenAmount) Add(b TokenAmount) TokenAmount {
	return TokenAmount(a.Decimal().Add(b.Decimal()))
}

// Sub 减法
func (a TokenAmount) Sub(b TokenAmount) TokenAmount {
	return TokenAmount(a.Decimal().Sub(b.Decimal()))
}

// Mul 乘法
func (a TokenAmount) Mul(b TokenAmount) TokenAmount {
	return TokenAmount(a.Decimal().Mul(b.Decimal()))
}

// Div 除法
func (a TokenAmount) Div(b TokenAmount) TokenAmount {
	return TokenAmount(a.Decimal().Div(b.Decimal()))
}

// Decimal 获取底层 decimal
func (a TokenAmount) Decimal() decimal.Decimal {
	return decimal.Decimal(a)
}

// IsZero 是否为零
func (a TokenAmount) IsZero() bool {
	return a.Decimal().IsZero()
}

// LessThan 是否小于
func (a TokenAmount) LessThan(b TokenAmount) bool {
	return a.Decimal().LessThan(b.Decimal())
}

// GreaterThan 是否大于
func (a TokenAmount) GreaterThan(b TokenAmount) bool {
	return a.Decimal().GreaterThan(b.Decimal())
}
