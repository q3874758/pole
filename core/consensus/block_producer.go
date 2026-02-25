package consensus

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"pole-core/core/executor"
	"pole-core/core/state"
	"pole-core/core/types"
)

// BlockRunner 区块运行器（负责出块）
type BlockRunner struct {
	proposer     types.Address
	height       types.BlockHeight
	lastBlockHash [32]byte
	chainState   *state.ChainState
	executor     *executor.Executor
	config       *Config
	mu           sync.RWMutex
}

// NewBlockRunner 创建区块运行器
func NewBlockRunner(proposer types.Address, chainState *state.ChainState, exec *executor.Executor, config *Config) *BlockRunner {
	h := chainState.GetHeight()
	var lastHash [32]byte
	appHash := chainState.GetAppHash()
	copy(lastHash[:], appHash)
	return &BlockRunner{
		proposer:     proposer,
		height:       types.BlockHeight(h),
		lastBlockHash: lastHash,
		chainState:   chainState,
		executor:     exec,
		config:       config,
	}
}

// CreateBlock 创建新区块
func (bp *BlockRunner) CreateBlock(txs []types.Transaction) (*types.Block, error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.height++
	header := types.NewBlockHeader(bp.height, bp.lastBlockHash, bp.proposer)
	header.DataHash = bp.calculateDataHash(txs)
	header.ValidatorsHash = bp.calculateValidatorsHash()

	block := types.NewBlock(*header)
	block.Transactions = txs

	bp.lastBlockHash = bp.calculateBlockHash(header)

	return block, nil
}

// ExecuteBlock 执行区块中的所有交易
func (bp *BlockRunner) ExecuteBlock(block *types.Block) error {
	for i, tx := range block.Transactions {
		if err := bp.executor.ExecuteTx(&tx); err != nil {
			return fmt.Errorf("tx %d: %w", i, err)
		}
	}
	// 更新高度
	bp.chainState.IncrementHeight()
	// 计算新 AppHash（这里简化为区块哈希）
	bp.lastBlockHash = bp.calculateBlockHash(&block.Header)
	bp.chainState.SetAppHash(bp.lastBlockHash[:])
	// 保存状态
	return bp.chainState.SaveState()
}

// calculateBlockHash 计算区块哈希
func (bp *BlockRunner) calculateBlockHash(header *types.BlockHeader) [32]byte {
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

// calculateDataHash 计算交易数据哈希
func (bp *BlockRunner) calculateDataHash(txs []types.Transaction) [32]byte {
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

// calculateValidatorsHash 计算验证者哈希
func (bp *BlockRunner) calculateValidatorsHash() [32]byte {
	return [32]byte{}
}

// ==================== 共识循环 ====================

// ConsensusLoop 共识出块循环
type ConsensusLoop struct {
	producer    *BlockProducer
	consensus   *HybridConsensus
	chainState  *state.ChainState
	config      *Config
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	started     bool
	mu          sync.Mutex
}

// NewConsensusLoop 创建共识循环
func NewConsensusLoop(
	consensus *HybridConsensus,
	chainState *state.ChainState,
	exec *executor.Executor,
	config *Config,
) *ConsensusLoop {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConsensusLoop{
		consensus:  consensus,
		chainState: chainState,
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start 启动共识循环
func (cl *ConsensusLoop) Start() {
	cl.mu.Lock()
	if cl.started {
		cl.mu.Unlock()
		return
	}
	cl.started = true
	cl.mu.Unlock()

	cl.wg.Add(1)
	go cl.run()
}

// Stop 停止共识循环
func (cl *ConsensusLoop) Stop() {
	cl.cancel()
	cl.wg.Wait()
	cl.mu.Lock()
	cl.started = false
	cl.mu.Unlock()
}

// run 共识循环主函数
func (cl *ConsensusLoop) run() {
	defer cl.wg.Done()

	ticker := time.NewTicker(time.Duration(cl.config.BlockTime) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cl.ctx.Done():
			return
		case <-ticker.C:
			cl.produceBlock()
		}
	}
}

// produceBlock 生产区块
func (cl *ConsensusLoop) produceBlock() {
	// 获取当前高度
	height := cl.chainState.GetHeight()

	// 计算当前高度的提议者
	proposer, err := cl.consensus.CalculateProposer(types.BlockHeight(height + 1))
	if err != nil {
		// 没有活跃验证者，跳过
		return
	}

	// 简化：当前节点作为提议者（实际需要检查是否为当前节点）
	// 在真实实现中，这里需要检查本地是否为提议者
	_ = proposer

	// 创建区块运行器
	bp := NewBlockRunner(proposer, cl.chainState, nil, cl.config)

	// TODO: 获取待执行交易（从内存池）
	txs := []types.Transaction{}

	// 创建区块
	block, err := bp.CreateBlock(txs)
	if err != nil {
		fmt.Printf("[Consensus] 创建区块失败: %v\n", err)
		return
	}

	// 执行区块
	if err := bp.ExecuteBlock(block); err != nil {
		fmt.Printf("[Consensus] 执行区块失败: %v\n", err)
		return
	}

	// 广播区块（TODO: 通过 p2p 广播）
	fmt.Printf("[Consensus] 区块已生产: #%d, 交易数: %d\n", block.Header.Height, len(block.Transactions))
}

// IsRunning 检查是否正在运行
func (cl *ConsensusLoop) IsRunning() bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.started
}

// GetHeight 获取当前高度
func (cl *ConsensusLoop) GetHeight() uint64 {
	return cl.chainState.GetHeight()
}
