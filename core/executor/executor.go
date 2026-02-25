package executor

import (
	"context"
	"fmt"
	"math/big"

	"github.com/shopspring/decimal"
	"pole-core/contracts"
	"pole-core/core/crypto"
	"pole-core/core/state"
	"pole-core/core/types"
)

// TokenAmountToBigInt 将 TokenAmount 转换为 *big.Int
func TokenAmountToBigInt(amount types.TokenAmount) *big.Int {
	d := decimal.Decimal(amount)
	return d.BigInt()
}

// BigIntToTokenAmount 将 *big.Int 转换为 TokenAmount
func BigIntToTokenAmount(v *big.Int) types.TokenAmount {
	return types.TokenAmount(decimal.NewFromBigInt(v, 0))
}

// Executor 交易执行器（将交易路由到合约调用）
type Executor struct {
	chainState *state.ChainState
	signer     *crypto.Signer
}

// NewExecutor 创建执行器
func NewExecutor(cs *state.ChainState) *Executor {
	return &Executor{
		chainState: cs,
		signer:     crypto.NewSigner(),
	}
}

// ExecuteTx 执行交易并返回结果
func (e *Executor) ExecuteTx(tx *types.Transaction) error {
	// 验证签名（跳过数据提交交易）
	if tx.Type != types.TxSubmitData {
		if err := e.verifySignature(tx); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	switch tx.Type {
	case types.TxTransfer:
		return e.executeTransfer(tx.Transfer)
	case types.TxStake:
		return e.executeStake(tx.Stake)
	case types.TxUnstake:
		return e.executeUnstake(tx.Unstake)
	case types.TxVote:
		return e.executeVote(tx.Vote)
	case types.TxSubmitData:
		// 数据提交在共识层处理，此处返回成功
		return nil
	case types.TxExecute:
		return e.executeContract(tx.Execute)
	default:
		return fmt.Errorf("unknown transaction type: %v", tx.Type)
	}
}

// verifySignature 验证交易签名
func (e *Executor) verifySignature(tx *types.Transaction) error {
	if tx == nil {
		return fmt.Errorf("nil transaction")
	}
	// 验证签名存在
	if len(tx.Signature) == 0 {
		return fmt.Errorf("missing signature")
	}
	// 使用签名器验证
	return e.signer.Verify(tx)
}

// SetSigner 设置签名器（用于测试或自定义密钥方案）
func (e *Executor) SetSigner(s *crypto.Signer) {
	e.signer = s
}

// executeTransfer 执行转账交易
func (e *Executor) executeTransfer(tx *types.TransferTx) error {
	if tx == nil {
		return fmt.Errorf("nil transfer tx")
	}
	amount := TokenAmountToBigInt(tx.Amount)
	// 验证金额
	if amount.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("invalid amount")
	}
	// 调用代币合约
	ctx := context.Background()
	err := e.chainState.GetToken().Transfer(ctx, tx.From.String(), tx.To.String(), amount)
	if err != nil {
		return fmt.Errorf("transfer: %w", err)
	}
	return nil
}

// executeStake 执行质押交易
func (e *Executor) executeStake(tx *types.StakeTx) error {
	if tx == nil {
		return fmt.Errorf("nil stake tx")
	}
	amount := TokenAmountToBigInt(tx.Amount)
	if amount.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("invalid stake amount")
	}
	// 先转账到质押合约（需要先 approve，这里简化直接调用 delegate）
	// 实际实现中需要：from 调用 token.approve(staking_contract, amount)，然后 staking.delegate
	// 这里简化为直接委托
	err := e.chainState.GetStaking().Delegate(tx.Delegator.String(), tx.Validator.String(), amount)
	if err != nil {
		return fmt.Errorf("delegate: %w", err)
	}
	return nil
}

// executeUnstake 执行解除质押交易
func (e *Executor) executeUnstake(tx *types.UnstakeTx) error {
	if tx == nil {
		return fmt.Errorf("nil unstake tx")
	}
	amount := TokenAmountToBigInt(tx.Amount)
	if amount.Cmp(big.NewInt(0)) <= 0 {
		return fmt.Errorf("invalid unstake amount")
	}
	err := e.chainState.GetStaking().Undelegate(tx.Delegator.String(), amount)
	if err != nil {
		return fmt.Errorf("undelegate: %w", err)
	}
	return nil
}

// executeVote 执行投票交易
func (e *Executor) executeVote(tx *types.VoteTx) error {
	if tx == nil {
		return fmt.Errorf("nil vote tx")
	}
	// 转换投票选项
	var opt contracts.GovVoteOption
	switch tx.VoteOption {
	case 0:
		opt = contracts.GovVoteAbstain
	case 1:
		opt = contracts.GovVoteYes
	case 2:
		opt = contracts.GovVoteNo
	default:
		return fmt.Errorf("invalid vote option: %d", tx.VoteOption)
	}
	weight := TokenAmountToBigInt(tx.Weight)
	err := e.chainState.GetGovernance().CastVote(tx.ProposalID, tx.Voter.String(), opt, weight)
	if err != nil {
		return fmt.Errorf("cast vote: %w", err)
	}
	return nil
}

// executeContract 执行合约调用
func (e *Executor) executeContract(tx *types.ExecuteTx) error {
	if tx == nil {
		return fmt.Errorf("nil execute tx")
	}
	// 根据 contract_id 路由到不同合约
	// 这里简化：假设 contract_id 0=Token, 1=Staking, 2=Governance
	// 实际实现需要更复杂的路由逻辑
	switch tx.ContractID {
	case 0:
		// Token 合约的直接调用（扩展用）
		return fmt.Errorf("contract 0 not implemented")
	case 1:
		// Staking 合约的直接调用（扩展用）
		return fmt.Errorf("contract 1 not implemented")
	case 2:
		// Governance 合约的直接调用（扩展用）
		return fmt.Errorf("contract 2 not implemented")
	default:
		return fmt.Errorf("unknown contract: %d", tx.ContractID)
	}
}

// ExecuteBlock 执行一个区块的所有交易
func (e *Executor) ExecuteBlock(block *types.Block) error {
	for i, tx := range block.Transactions {
		if err := e.ExecuteTx(&tx); err != nil {
			return fmt.Errorf("tx %d: %w", i, err)
		}
	}
	// 更新区块高度
	e.chainState.IncrementHeight()
	// 保存状态
	return e.chainState.SaveState()
}

// DeliverTx 验证并交付交易（不改变状态，仅验证）
func (e *Executor) DeliverTx(tx *types.Transaction) error {
	// 基础验证：检查交易是否可解析
	if tx == nil {
		return fmt.Errorf("nil transaction")
	}
	// 根据类型验证
	switch tx.Type {
	case types.TxTransfer:
		if tx.Transfer == nil {
			return fmt.Errorf("nil transfer data")
		}
	case types.TxStake:
		if tx.Stake == nil {
			return fmt.Errorf("nil stake data")
		}
	case types.TxVote:
		if tx.Vote == nil {
			return fmt.Errorf("nil vote data")
		}
	}
	return nil
}

// Query 查询链状态（只读）
func (e *Executor) Query(addr string) *big.Int {
	return e.chainState.GetBalance(addr)
}
