package contracts

import (
	"encoding/json"
	"math/big"
	"os"
)

// ==================== 创世配置 ====================

// GenesisConfig 创世配置（与白皮书一致）
// Staking/Governance 不序列化到 JSON，由节点启动时使用默认配置
type GenesisConfig struct {
	ChainID     string                 `json:"chain_id"`
	GenesisTime int64                  `json:"genesis_time"`
	Token       GenesisToken           `json:"token"`
	Allocations []GenesisAllocation    `json:"allocations"`
	Validators  []GenesisValidator     `json:"validators"`
	Vesting     []GenesisVesting       `json:"vesting,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`

	Staking    *StakingConfig    `json:"-"`
	Governance *GovernanceConfig `json:"-"`
}

// GenesisToken 创世代币配置
type GenesisToken struct {
	Name       string   `json:"name"`
	Symbol     string   `json:"symbol"`
	Decimals   uint8    `json:"decimals"`
	TotalSupply *big.Int `json:"total_supply"`
}

// GenesisAllocation 创世分配（白皮书：60% 节点奖励池, 20% 生态, 15% 社区, 5% 团队与投资人）
type GenesisAllocation struct {
	Address string   `json:"address"`
	Amount  *big.Int `json:"amount"`
	Label   string   `json:"label,omitempty"` // NodeRewardPool / Ecosystem / Community / TeamAndInvestors
}

// GenesisValidator 创世验证者
type GenesisValidator struct {
	Address    string   `json:"address"`
	PubKey     string   `json:"pub_key"`
	Stake      *big.Int `json:"stake"`
	Commission uint8    `json:"commission"`
}

// GenesisVesting 创世线性释放（白皮书：团队 12 月锁仓 + 24 月线性释放）
type GenesisVesting struct {
	Beneficiary   string   `json:"beneficiary"`
	Total         *big.Int `json:"total"`
	LockMonths    int      `json:"lock_months"`
	LinearMonths  int      `json:"linear_months"`
}

// MarshalJSON 自定义 GenesisToken.TotalSupply 序列化
func (g GenesisToken) MarshalJSON() ([]byte, error) {
	type Alias GenesisToken
	return json.Marshal(struct {
		TotalSupply string `json:"total_supply"`
		Alias
	}{
		TotalSupply: g.TotalSupply.String(),
		Alias:       (Alias)(g),
	})
}

// UnmarshalJSON 自定义 GenesisToken.TotalSupply 反序列化
func (g *GenesisToken) UnmarshalJSON(data []byte) error {
	type Alias GenesisToken
	aux := &struct {
		TotalSupply string `json:"total_supply"`
		*Alias
	}{
		Alias: (*Alias)(g),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	g.TotalSupply = new(big.Int)
	g.TotalSupply.SetString(aux.TotalSupply, 10)
	return nil
}

// MarshalJSON GenesisAllocation.Amount
func (a GenesisAllocation) MarshalJSON() ([]byte, error) {
	type Alias GenesisAllocation
	return json.Marshal(struct {
		Amount string `json:"amount"`
		Alias
	}{
		Amount: a.Amount.String(),
		Alias:  (Alias)(a),
	})
}

// UnmarshalJSON GenesisAllocation.Amount
func (a *GenesisAllocation) UnmarshalJSON(data []byte) error {
	type Alias GenesisAllocation
	aux := &struct {
		Amount string `json:"amount"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	a.Amount = new(big.Int)
	a.Amount.SetString(aux.Amount, 10)
	return nil
}

// MarshalJSON GenesisValidator.Stake
func (v GenesisValidator) MarshalJSON() ([]byte, error) {
	type Alias GenesisValidator
	return json.Marshal(struct {
		Stake string `json:"stake"`
		Alias
	}{
		Stake: v.Stake.String(),
		Alias: (Alias)(v),
	})
}

// UnmarshalJSON GenesisValidator.Stake
func (v *GenesisValidator) UnmarshalJSON(data []byte) error {
	type Alias GenesisValidator
	aux := &struct {
		Stake string `json:"stake"`
		*Alias
	}{
		Alias: (*Alias)(v),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	v.Stake = new(big.Int)
	v.Stake.SetString(aux.Stake, 10)
	return nil
}

// MarshalJSON GenesisVesting.Total
func (v GenesisVesting) MarshalJSON() ([]byte, error) {
	type Alias GenesisVesting
	return json.Marshal(struct {
		Total string `json:"total"`
		Alias
	}{
		Total: v.Total.String(),
		Alias: (Alias)(v),
	})
}

// UnmarshalJSON GenesisVesting.Total
func (v *GenesisVesting) UnmarshalJSON(data []byte) error {
	type Alias GenesisVesting
	aux := &struct {
		Total string `json:"total"`
		*Alias
	}{
		Alias: (*Alias)(v),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	v.Total = new(big.Int)
	v.Total.SetString(aux.Total, 10)
	return nil
}

// DefaultGenesisConfig 默认创世配置（白皮书比例）
func DefaultGenesisConfig() *GenesisConfig {
	totalSupply := new(big.Int).Mul(big.NewInt(1_000_000_000), pow10(18))
	g := &GenesisConfig{
		ChainID:     "pole-mainnet-1",
		GenesisTime: 0,
		Token: GenesisToken{
			Name:        "Proof of Live Engagement",
			Symbol:      "POLE",
			Decimals:    18,
			TotalSupply: totalSupply,
		},
		Allocations: defaultAllocations(totalSupply),
		Validators:  []GenesisValidator{},
		Extra:       map[string]interface{}{},
	}
	g.Staking = DefaultStakingConfig()
	g.Governance = DefaultGovernanceConfig()
	return g
}

// defaultAllocations 按白皮书：60% 节点奖励池, 20% 生态, 15% 社区, 5% 团队与投资人
func defaultAllocations(total *big.Int) []GenesisAllocation {
	sixty := new(big.Int).Mul(total, big.NewInt(60))
	sixty = sixty.Div(sixty, big.NewInt(100))
	twenty := new(big.Int).Mul(total, big.NewInt(20))
	twenty = twenty.Div(twenty, big.NewInt(100))
	fifteen := new(big.Int).Mul(total, big.NewInt(15))
	fifteen = fifteen.Div(fifteen, big.NewInt(100))
	five := new(big.Int).Mul(total, big.NewInt(5))
	five = five.Div(five, big.NewInt(100))
	return []GenesisAllocation{
		{Address: "pole1noderewardpool000000000000000000000", Amount: sixty, Label: "NodeRewardPool"},
		{Address: "pole1ecosystem0000000000000000000000000", Amount: twenty, Label: "Ecosystem"},
		{Address: "pole1community00000000000000000000000000", Amount: fifteen, Label: "Community"},
		{Address: "pole1teamandinvestors000000000000000000", Amount: five, Label: "TeamAndInvestors"},
	}
}

// LoadGenesisConfig 从 JSON 文件加载创世配置
func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g GenesisConfig
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// SaveGenesisConfig 将创世配置写入 JSON 文件
func (g *GenesisConfig) SaveGenesisConfig(path string) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ApplyGenesis 用创世配置初始化代币合约（分配余额）；若有 vesting 则把锁仓总量打入 VestingPoolAddress
func ApplyGenesis(tc *TokenContract, g *GenesisConfig) error {
	if g == nil {
		return nil
	}
	alloc := make(map[string]*big.Int)
	for _, a := range g.Allocations {
		if a.Amount.Sign() <= 0 {
			continue
		}
		alloc[a.Address] = new(big.Int).Set(a.Amount)
	}
	// 团队/投资人线性释放：锁仓池获得所有 vesting 总量
	if len(g.Vesting) > 0 {
		poolTotal := new(big.Int)
		for _, v := range g.Vesting {
			if v.Total != nil && v.Total.Sign() > 0 {
				poolTotal.Add(poolTotal, v.Total)
			}
		}
		if poolTotal.Sign() > 0 {
			existing := alloc[VestingPoolAddress]
			if existing != nil {
				poolTotal.Add(poolTotal, existing)
			}
			alloc[VestingPoolAddress] = poolTotal
		}
	}
	return tc.InitializeGenesis(alloc)
}

// ApplyVestingSchedules 根据创世配置为 VestingContract 添加释放计划（在 InitWithGenesis 中创建 vesting 后调用）
func ApplyVestingSchedules(vc *VestingContract, g *GenesisConfig) error {
	if g == nil || len(g.Vesting) == 0 {
		return nil
	}
	for _, v := range g.Vesting {
		if v.Total == nil || v.Total.Sign() <= 0 || v.LinearMonths <= 0 {
			continue
		}
		lockUntil := g.GenesisTime + int64(v.LockMonths)*SecondsPerMonth
		if err := vc.AddSchedule(v.Beneficiary, v.Total, lockUntil, v.LinearMonths); err != nil {
			return err
		}
	}
	return nil
}
