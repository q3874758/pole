package state

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"

	"pole-core/contracts"
)

// ChainState 链状态（包含所有合约）
type ChainState struct {
	// 链信息
	ChainID  string `json:"chain_id"`
	Height   uint64 `json:"height"`
	AppHash  []byte `json:"app_hash"`

	// 合约（运行时用，不序列化）
	token        *contracts.TokenContract
	staking      *contracts.StakingContract
	governance   *contracts.GovernanceContract
	treasury     *contracts.TreasuryContract
	vesting      *contracts.VestingContract

	// 状态文件路径
	dataDir string
	mu      sync.RWMutex
}

// NewChainState 创建链状态
func NewChainState(dataDir string) *ChainState {
	cs := &ChainState{
		dataDir:  dataDir,
		Height:   0,
		AppHash:  make([]byte, 32),
	}
	// 确保目录存在
	if dataDir != "" {
		_ = os.MkdirAll(dataDir, 0755)
	}
	return cs
}

// InitWithGenesis 用创世配置初始化
func (cs *ChainState) InitWithGenesis(g *contracts.GenesisConfig) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.ChainID = g.ChainID

	// 初始化合约配置
	tokenConfig := contracts.DefaultTokenConfig()
	stakingConfig := contracts.DefaultStakingConfig()
	governanceConfig := contracts.DefaultGovernanceConfig()

	// 如果创世有覆盖配置则用创世的
	if g.Token.Name != "" {
		tokenConfig.Name = g.Token.Name
		tokenConfig.Symbol = g.Token.Symbol
		tokenConfig.Decimals = g.Token.Decimals
		tokenConfig.TotalSupply = g.Token.TotalSupply
	}
	if g.Staking != nil {
		stakingConfig = g.Staking
	}
	if g.Governance != nil {
		governanceConfig = g.Governance
	}

	// 创建合约实例
	cs.token = contracts.NewTokenContract(tokenConfig)
	cs.staking = contracts.NewStakingContract(stakingConfig, cs.token)
	cs.governance = contracts.NewGovernanceContract(governanceConfig)
	cs.treasury = contracts.NewTreasuryContract(contracts.DefaultTreasuryConfig())

	// 应用创世分配
	if err := contracts.ApplyGenesis(cs.token, g); err != nil {
		return fmt.Errorf("apply genesis: %w", err)
	}

	// 团队/投资人线性释放：创建锁仓合约并应用创世 vesting 计划
	cs.vesting = contracts.NewVestingContract(cs.token, contracts.VestingPoolAddress)
	if err := contracts.ApplyVestingSchedules(cs.vesting, g); err != nil {
		return fmt.Errorf("apply vesting: %w", err)
	}

	// 注册创世验证者
	for _, v := range g.Validators {
		if v.Stake.Sign() <= 0 {
			continue
		}
		// 创建验证者（自质押）
		if err := cs.staking.RegisterValidator(v.Address, v.PubKey, v.Commission, v.Stake); err != nil {
			// 可能是重复注册，继续
			fmt.Printf("warn: register validator %s: %v\n", v.Address, err)
		}
	}

	// 保存初始状态
	return cs.save()
}

// GetToken 获取代币合约
func (cs *ChainState) GetToken() *contracts.TokenContract {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.token
}

// GetStaking 获取质押合约
func (cs *ChainState) GetStaking() *contracts.StakingContract {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.staking
}

// GetGovernance 获取治理合约
func (cs *ChainState) GetGovernance() *contracts.GovernanceContract {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.governance
}

// GetTreasury 获取国库合约
func (cs *ChainState) GetTreasury() *contracts.TreasuryContract {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.treasury
}

// GetVesting 获取锁仓合约
func (cs *ChainState) GetVesting() *contracts.VestingContract {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.vesting
}

// IncrementHeight 增加区块高度
func (cs *ChainState) IncrementHeight() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Height++
}

// GetHeight 获取当前高度
func (cs *ChainState) GetHeight() uint64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.Height
}

// SetAppHash 设置 AppHash
func (cs *ChainState) SetAppHash(hash []byte) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.AppHash = hash
}

// GetAppHash 获取 AppHash
func (cs *ChainState) GetAppHash() []byte {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	hash := make([]byte, len(cs.AppHash))
	copy(hash, cs.AppHash)
	return hash
}

// ==================== 状态持久化 ====================

// StateFile 状态文件结构
type StateFile struct {
	ChainID    string          `json:"chain_id"`
	Height     uint64          `json:"height"`
	AppHash    string          `json:"app_hash"`
	Token      json.RawMessage `json:"token"`
	Staking    json.RawMessage `json:"staking"`
	Governance json.RawMessage `json:"governance"`
	Treasury   json.RawMessage `json:"treasury,omitempty"`
	Vesting    json.RawMessage `json:"vesting,omitempty"`
}

// SaveState 保存状态到文件
func (cs *ChainState) SaveState() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.save()
}

func (cs *ChainState) save() error {
	if cs.dataDir == "" {
		return nil // 无持久化
	}

	// 序列化合约状态
	tokenState, _ := json.Marshal(cs.token)
	stakingState, _ := json.Marshal(cs.staking)
	governanceState, _ := json.Marshal(cs.governance)
	treasuryState, _ := json.Marshal(cs.treasury)

	vestingState, _ := json.Marshal(cs.vesting)
	state := StateFile{
		ChainID:    cs.ChainID,
		Height:     cs.Height,
		AppHash:    fmt.Sprintf("%x", cs.AppHash),
		Token:      tokenState,
		Staking:    stakingState,
		Governance: governanceState,
		Treasury:   treasuryState,
		Vesting:    vestingState,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(cs.dataDir, "state.json")
	return os.WriteFile(path, data, 0644)
}

// LoadState 从文件加载状态
func (cs *ChainState) LoadState() error {
	if cs.dataDir == "" {
		return nil
	}

	path := filepath.Join(cs.dataDir, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 首次启动，无状态文件
		}
		return err
	}

	var state StateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	cs.ChainID = state.ChainID
	cs.Height = state.Height
	cs.AppHash = make([]byte, 32)
	if len(state.AppHash) > 0 {
		fmt.Sscanf(state.AppHash, "%x", &cs.AppHash)
	}

	// 反序列化合约状态
	if len(state.Token) > 0 {
		// 恢复 token 状态
		tokenConfig := contracts.DefaultTokenConfig()
		cs.token = contracts.NewTokenContract(tokenConfig)
		if err := json.Unmarshal(state.Token, cs.token); err != nil {
			return fmt.Errorf("unmarshal token: %w", err)
		}
	}

	if len(state.Staking) > 0 && cs.token != nil {
		stakingConfig := contracts.DefaultStakingConfig()
		cs.staking = contracts.NewStakingContract(stakingConfig, cs.token)
		if err := json.Unmarshal(state.Staking, cs.staking); err != nil {
			return fmt.Errorf("unmarshal staking: %w", err)
		}
	}

	if len(state.Governance) > 0 {
		govConfig := contracts.DefaultGovernanceConfig()
		cs.governance = contracts.NewGovernanceContract(govConfig)
		if err := json.Unmarshal(state.Governance, cs.governance); err != nil {
			return fmt.Errorf("unmarshal governance: %w", err)
		}
	}

	if len(state.Treasury) > 0 {
		treasuryConfig := contracts.DefaultTreasuryConfig()
		cs.treasury = contracts.NewTreasuryContract(treasuryConfig)
		if err := json.Unmarshal(state.Treasury, cs.treasury); err != nil {
			return fmt.Errorf("unmarshal treasury: %w", err)
		}
	}
	if cs.treasury == nil {
		cs.treasury = contracts.NewTreasuryContract(contracts.DefaultTreasuryConfig())
	}

	if len(state.Vesting) > 0 && cs.token != nil {
		cs.vesting = contracts.NewVestingContract(cs.token, contracts.VestingPoolAddress)
		if err := json.Unmarshal(state.Vesting, cs.vesting); err != nil {
			return fmt.Errorf("unmarshal vesting: %w", err)
		}
	}
	if cs.vesting == nil && cs.token != nil {
		cs.vesting = contracts.NewVestingContract(cs.token, contracts.VestingPoolAddress)
	}

	return nil
}

// ==================== 工具方法 ====================

// GetBalance 获取账户余额
func (cs *ChainState) GetBalance(addr string) *big.Int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.token == nil {
		return big.NewInt(0)
	}
	return cs.token.BalanceOf(addr)
}

// GetValidator 获取验证者信息
func (cs *ChainState) GetValidator(addr string) (*contracts.Validator, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.staking == nil {
		return nil, false
	}
	return cs.staking.GetValidator(addr)
}

// GetProposal 获取提案
func (cs *ChainState) GetProposal(id uint64) (*contracts.GovProposal, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.governance == nil {
		return nil, false
	}
	return cs.governance.GetProposal(id)
}
