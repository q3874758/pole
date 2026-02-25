package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// VestingPoolAddress 锁仓池地址（持有待释放代币）
const VestingPoolAddress = "pole1vestingpool00000000000000000000"

// SecondsPerMonth 按 30 天计算
const SecondsPerMonth = 30 * 24 * 3600

// VestingSchedule 单笔线性释放计划（白皮书：锁定期后按月线性释放）
type VestingSchedule struct {
	Beneficiary   string   `json:"beneficiary"`
	Total         *big.Int `json:"total"`
	LockUntilUnix int64    `json:"lock_until_unix"` // 锁仓结束时间（Unix 秒）
	VestingMonths int      `json:"vesting_months"`  // 线性释放月数
	Claimed       *big.Int `json:"claimed"`         // 已领取数量
}

// VestingContract 团队/投资人线性释放合约
type VestingContract struct {
	token    *TokenContract
	poolAddr string
	schedules map[string]*VestingSchedule
	mu       sync.RWMutex
}

// NewVestingContract 创建锁仓合约，poolAddr 需已在 token 中有余额
func NewVestingContract(token *TokenContract, poolAddr string) *VestingContract {
	if poolAddr == "" {
		poolAddr = VestingPoolAddress
	}
	return &VestingContract{
		token:    token,
		poolAddr: poolAddr,
		schedules: make(map[string]*VestingSchedule),
	}
}

// AddSchedule 添加释放计划（创世或治理调用）
func (vc *VestingContract) AddSchedule(beneficiary string, total *big.Int, lockUntilUnix int64, vestingMonths int) error {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if total.Sign() <= 0 || vestingMonths <= 0 {
		return fmt.Errorf("invalid total or vesting months")
	}
	if _, exists := vc.schedules[beneficiary]; exists {
		return fmt.Errorf("vesting schedule already exists for %s", beneficiary)
	}

	vc.schedules[beneficiary] = &VestingSchedule{
		Beneficiary:   beneficiary,
		Total:         new(big.Int).Set(total),
		LockUntilUnix: lockUntilUnix,
		VestingMonths: vestingMonths,
		Claimed:       big.NewInt(0),
	}
	return nil
}

// unlockedAmount 计算到当前时刻已解锁总量（按整月）
func (vc *VestingContract) unlockedAmount(s *VestingSchedule, nowUnix int64) *big.Int {
	if nowUnix < s.LockUntilUnix {
		return big.NewInt(0)
	}
	elapsed := nowUnix - s.LockUntilUnix
	months := int64(elapsed / int64(SecondsPerMonth))
	if months <= 0 {
		return big.NewInt(0)
	}
	if months >= int64(s.VestingMonths) {
		return new(big.Int).Set(s.Total)
	}
	// 已解锁 = Total * months / VestingMonths
	unlocked := new(big.Int).Mul(s.Total, big.NewInt(months))
	unlocked.Div(unlocked, big.NewInt(int64(s.VestingMonths)))
	return unlocked
}

// Claim 领取已解锁部分：从池子转至受益人
func (vc *VestingContract) Claim(beneficiary string) (*big.Int, error) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	s, ok := vc.schedules[beneficiary]
	if !ok {
		return nil, fmt.Errorf("no vesting schedule for %s", beneficiary)
	}

	now := time.Now().Unix()
	unlocked := vc.unlockedAmount(s, now)
	claimable := new(big.Int).Sub(unlocked, s.Claimed)
	if claimable.Sign() <= 0 {
		return nil, fmt.Errorf("no vesting amount to claim")
	}

	ctx := context.Background()
	// 从池子转给受益人（池子地址需有足够余额）
	if err := vc.token.Transfer(ctx, vc.poolAddr, beneficiary, claimable); err != nil {
		return nil, fmt.Errorf("transfer from vesting pool: %w", err)
	}

	s.Claimed = new(big.Int).Add(s.Claimed, claimable)
	return claimable, nil
}

// GetInfo 查询某地址的锁仓信息
func (vc *VestingContract) GetInfo(beneficiary string) (total, claimed, claimable *big.Int, lockUntil int64, vestingMonths int, ok bool) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	s, exists := vc.schedules[beneficiary]
	if !exists {
		return nil, nil, nil, 0, 0, false
	}

	now := time.Now().Unix()
	unlocked := vc.unlockedAmount(s, now)
	claimable = new(big.Int).Sub(unlocked, s.Claimed)
	if claimable.Sign() < 0 {
		claimable = big.NewInt(0)
	}

	return new(big.Int).Set(s.Total), new(big.Int).Set(s.Claimed), claimable, s.LockUntilUnix, s.VestingMonths, true
}

// MarshalJSON 序列化（供 state 持久化）
func (vc *VestingContract) MarshalJSON() ([]byte, error) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	type enc struct {
		PoolAddr  string             `json:"pool_addr"`
		Schedules map[string]*VestingSchedule `json:"schedules"`
	}
	return json.Marshal(enc{PoolAddr: vc.poolAddr, Schedules: vc.schedules})
}

// UnmarshalJSON 反序列化
func (vc *VestingContract) UnmarshalJSON(data []byte) error {
	var enc struct {
		PoolAddr  string             `json:"pool_addr"`
		Schedules map[string]*VestingSchedule `json:"schedules"`
	}
	if err := json.Unmarshal(data, &enc); err != nil {
		return err
	}
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.poolAddr = enc.PoolAddr
	if enc.Schedules != nil {
		vc.schedules = enc.Schedules
	} else {
		vc.schedules = make(map[string]*VestingSchedule)
	}
	return nil
}

// VestingSchedule MarshalJSON Amount
func (s VestingSchedule) MarshalJSON() ([]byte, error) {
	type Alias VestingSchedule
	return json.Marshal(struct {
		Total  string `json:"total"`
		Claimed string `json:"claimed"`
		Alias
	}{
		Total:   s.Total.String(),
		Claimed: s.Claimed.String(),
		Alias:   (Alias)(s),
	})
}

// UnmarshalJSON VestingSchedule
func (s *VestingSchedule) UnmarshalJSON(data []byte) error {
	type Alias VestingSchedule
	aux := &struct {
		Total   string `json:"total"`
		Claimed string `json:"claimed"`
		*Alias
	}{Alias: (*Alias)(s)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.Total = new(big.Int)
	s.Total.SetString(aux.Total, 10)
	s.Claimed = new(big.Int)
	s.Claimed.SetString(aux.Claimed, 10)
	return nil
}
