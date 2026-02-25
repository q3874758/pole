package economy

import (
	"math/big"

	"github.com/shopspring/decimal"
	"pole-core/contracts"
)

// 链上 POLE 精度（与 TokenContract.Decimals 一致）
const treasuryDecimals = 18

var ten18 = decimal.NewFromInt(1e18)

// TransferToTreasuryFunc 将代币从手续费池转入国库账户的钩子（由链层注入）
type TransferToTreasuryFunc func(amountWei *big.Int) error

// TreasuryAdapter 将 contracts.TreasuryContract 适配为 economy.TreasuryManager
// 链上使用 *big.Int（最小单位），economy 使用 decimal（人类可读），此处做转换
// 若设置 TransferToTreasury，则 Deposit 时先执行代币转账再更新合约余额
type TreasuryAdapter struct {
	tc         *contracts.TreasuryContract
	transferFn TransferToTreasuryFunc
}

// NewTreasuryAdapter 创建国库适配器
func NewTreasuryAdapter(tc *contracts.TreasuryContract) *TreasuryAdapter {
	return &TreasuryAdapter{tc: tc}
}

// WithTransferToTreasury 设置“转入国库账户”钩子（链整合时注入：从 feePool 转到 treasury 地址）
func (a *TreasuryAdapter) WithTransferToTreasury(fn TransferToTreasuryFunc) *TreasuryAdapter {
	a.transferFn = fn
	return a
}

// Deposit 将 amount（人类可读 POLE）存入链上国库
// 若已设置 TransferToTreasury，会先执行代币转账再更新合约余额
func (a *TreasuryAdapter) Deposit(from string, amount decimal.Decimal) error {
	if amount.IsZero() || amount.IsNegative() {
		return nil
	}
	wei := amount.Mul(ten18).Truncate(0).BigInt()
	if wei.Sign() <= 0 {
		return nil
	}
	if a.transferFn != nil {
		if err := a.transferFn(wei); err != nil {
			return err
		}
	}
	return a.tc.Deposit(from, wei)
}

// GetBalance 返回链上国库余额（人类可读 POLE）
func (a *TreasuryAdapter) GetBalance() decimal.Decimal {
	b := a.tc.GetBalance()
	if b == nil || b.Sign() == 0 {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(b, -int32(treasuryDecimals))
}
