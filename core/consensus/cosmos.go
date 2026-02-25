package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/store"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authmodule "github.com/cosmos/cosmos-sdk/x/auth"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz"
	bankmodule "github.com/cosmos/cosmos-sdk/x/bank"
	distrmodule "github.com/cosmos/cosmos-sdk/x/distribution"
	genutilmodule "github.com/cosmos/cosmos-sdk/x/genutil"
	govmodule "github.com/cosmos/cosmos-sdk/x/gov"
	groupmodule "github.com/cosmos/cosmos-sdk/x/group"
	mintmodule "github.com/cosmos/cosmos-sdk/x/mint"
	parammodule "github.com/cosmos/cosmos-sdk/x/params"
	slashmodule "github.com/cosmos/cosmos-sdk/x/slashing"
	stakingmodule "github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/spf13/cast"
	"github.com/tendermint/tendermint/abci/example/counter"
	"github.com/tendermint/tendermint/abci/example/kvstore"
	"github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/version"
)

/*
PoLE 区块链 - Cosmos SDK 集成

这是一个概念验证，展示如何将 PoLE 与 Cosmos SDK 集成。

注意：这是一个简化版本，需要进一步开发才能用于生产环境。
*/

// AppConfig 应用配置
type AppConfig struct {
	ChainID        string `mapstructure:"chain-id" json:"chain-id"`
	TokenSymbol    string `mapstructure:"token-symbol" json:"token-symbol"`
	Denom          string `mapstructure:"denom" json:"denom"`
	MinSelfStake   int64  `mapstructure:"min-self-stake" json:"min-self-stake"`
	MaxValidators  int    `mapstructure:"max-validators" json:"max-validators"`
}

// PoleApp PoLE 区块链应用
type PoleApp struct {
	*baseapp.BaseApp
	cdc               *codec.LegacyAmino
	appCodec          codec.Codec
	interfaceRegistry  codectypes.InterfaceRegistry
	keys              map[string]*crypto.PrivKey
	keyRing           keyring.Keyring
	accountKeeper     authmodule.AccountKeeper
	bankKeeper        bankmodule.Keeper
	stakingKeeper     stakingmodule.Keeper
	distrKeeper       distrmodule.Keeper
	mintKeeper       mintmodule.Keeper
	slashKeeper      slashmodule.Keeper
	paramKeeper       parammodule.Keeper
	govKeeper        govmodule.Keeper
	groupKeeper       groupmodule.Keeper
	authzKeeper       authzmodule.Keeper
	mm                *module.Manager
	config            *AppConfig
}

// NewPoleApp 创建 PoLE 应用
func NewPoleApp(logger log.Logger, dbm storetypes.DB, config *AppConfig) *PoleApp {
	// 创建应用
	app := &PoleApp{
		BaseApp:          baseapp.NewBaseApp("pole", logger, dbm),
		config:           config,
		keys:             make(map[string]*crypto.PrivKey),
		interfaceRegistry: codectypes.NewInterfaceRegistry(),
		cdc:              codec.NewLegacyAmino(),
	}

	// 初始化模块管理器
	app.mm = module.NewManager()

	return app
}

// Initialize 初始化应用
func (app *PoleApp) Initialize() error {
	// 设置路由
	app.Router()

	// 设置查询路由
	app.QueryRouter()

	// 安装模块
	app.mm.RegisterInterfaces(app.interfaceRegistry)
	app.cdc.RegisterInterfaces(app.interfaceRegistry)

	// 加载密钥
	app.keys["validator"] = crypto.GenPrivKeyEd25519()

	return nil
}

// StartNode 启动节点
func StartNode(app *PoleApp, configPath string) error {
	// 加载 Tendermint 配置
	tmConfig := config.DefaultConfig()
	tmConfig.ChainID = app.config.ChainID

	// 创建应用
	appConn, err := proxy.NewLocalClientCreator(app)
	if err != nil {
		return fmt.Errorf("failed to create app connection: %w", err)
	}

	// 创建节点
	node, err := node.NewNode(
		tmConfig,
		privval.LoadOrGenFilePV(tmConfig.PrivValidatorKeyFile(), tmConfig.PrivValidatorStateFile()),
		[]p2p.NodeKey{},
		appConn,
		node.DefaultGenesisDocProviderFunc(tmConfig),
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	// 启动节点
	if err := node.Start(); err != nil {
		return fmt.Errorf("failed to start node: %w", err)
	}

	// 等待退出信号
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh

	// 停止节点
	node.Stop()
	return nil
}

// PoLE 特定模块 - GVS 计算引擎
type GVSEngine struct{}

func (g *GVSEngine) CalculateScore(gameData map[string]interface{}) float64 {
	// 简化的 GVS 计算
	// 实际实现需要更复杂的算法
	players := cast.ToInt(gameData["players"])
	playTime := cast.ToFloat64(gameData["play_time"])
	revenue := cast.ToFloat64(gameData["revenue"])

	// 基础分数 = 玩家数 * 0.1 + 游戏时间 * 0.5 + 收入 * 0.01
	score := float64(players)*0.1 + playTime*0.5 + revenue*0.01
	return score
}

// Main 主函数
func main() {
	logger := log.NewNopLogger()

	// 解析命令行参数
	configPath := "config"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	// 加载配置
	config := &AppConfig{
		ChainID:       "pole-1",
		TokenSymbol:   "POLE",
		Denom:         "upole",
		MinSelfStake:  10000,
		MaxValidators: 21,
	}

	// 创建应用
	app := NewPoleApp(logger, nil, config)
	if err := app.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 启动节点
	if err := app.StartNode(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "节点启动失败: %v\n", err)
		os.Exit(1)
	}
}

// 导出简化版本用于快速测试
func RunSimpleKVStore() {
	logger := log.NewNopLogger()
	db := &kvstore.MemDB{}
	app := kvstore.NewApplication()

	if err := node.NewKVStoreApplication(logger, db, app); err != nil {
		fmt.Printf("错误: %v\n", err)
	}
}

// CounterApp 用于测试
func RunCounter() {
	logger := log.NewNopLogger()
	db := &counter.MemDB{}
	app := counter.NewApplication()

	if err := node.NewCounterApplication(logger, db, app); err != nil {
		fmt.Printf("错误: %v\n", err)
	}
}

// ============ 模块接口 ============

// Module 接口定义
type Module interface {
	Name() string
	RegisterServices(codec.Codec)
}

// PoLEModule PoLE 自定义模块
type PoLEModule struct {
	gvsEngine GVSEngine
}

func (m *PoLEModule) Name() string { return "pole" }

func (m *PoLEModule) RegisterServices(cdc codec.Codec) {
	// 注册 gRPC 服务
}

// ValidatorSet 验证者集合
type ValidatorSet struct {
	Validators []Validator
}

type Validator struct {
	Address   []byte
	PubKey   []byte
	Stake    int64
	VotingPower int64
}

// AddValidator 添加验证者
func (vs *ValidatorSet) AddValidator(v Validator) {
	vs.Validators = append(vs.Validators, v)
}

// RemoveValidator 移除验证者
func (vs *ValidatorSet) RemoveValidator(addr []byte) {
	for i, v := range vs.Validators {
		if string(v.Address) == string(addr) {
			vs.Validators = append(vs.Validators[:i], vs.Validators[i+1:]...)
			return
		}
	}
}

// TotalStake 总质押
func (vs *ValidatorSet) TotalStake() int64 {
	var total int64
	for _, v := range vs.Validators {
		total += v.Stake
	}
	return total
}

// Proposer 选中的出块者
func (vs *ValidatorSet) Proposer() Validator {
	if len(vs.Validators) == 0 {
		return Validator{}
	}
	// 简化版：返回第一个验证者
	// 实际实现应该用 VRF 或轮询
	return vs.Validators[0]
}

// ============ 导出类型 ============

var (
	_ = types.NewIntWithDecimal
	_ = keyring.NewInMemory
	_ = client.Config
)

const (
	AppName    = "pole"
	DenomPOLE = "upole"
)
