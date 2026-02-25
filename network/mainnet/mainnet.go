package mainnet

import (
	"flag"
	"fmt"
	"os"

	"pole-core/contracts"
	"pole-core/core/consensus"
	"pole-core/core/executor"
	"pole-core/core/security"
	"pole-core/core/state"
	"pole-core/core/types"
	"pole-core/data/collector"
	"pole-core/execution/engine"
	"pole-core/governance"
	"pole-core/network/p2p"
	"pole-core/rpc"
	"pole-core/token/economy"
	"pole-core/token/rewards"
	"pole-core/wallet"
)

// MainnetConfig 主网配置
type MainnetConfig struct {
	ChainID        string   // Chain ID (pole-mainnet-1)
	NetworkName    string   // 网络名称
	GenesisFile    string   // 创世文件路径
	DataDir        string   // 数据目录
	RPCPort        string   // RPC 端口
	P2PPort        string   // P2P 端口
	PrometheusPort string   // 监控端口
	EnableTLS      bool     // 启用 TLS
	Bootnodes      []string // 引导节点列表
}

// TestnetConfig 测试网配置
type TestnetConfig struct {
	ChainID        string
	NetworkName    string
	GenesisFile    string
	DataDir        string
	RPCPort        string
	P2PPort        string
	PrometheusPort string
	EnableTLS      bool
	Bootnodes      []string
}

// DefaultMainnetConfig 默认主网配置
func DefaultMainnetConfig() *MainnetConfig {
	return &MainnetConfig{
		ChainID:        "pole-mainnet-1",
		NetworkName:    "PoLE Mainnet",
		GenesisFile:    "config/mainnet/genesis.json",
		DataDir:        "data/mainnet",
		RPCPort:        ":9090",
		P2PPort:        ":26656",
		PrometheusPort: ":9091",
		EnableTLS:      true,
		Bootnodes:      []string{},
	}
}

// DefaultTestnetConfig 默认测试网配置
func DefaultTestnetConfig() *TestnetConfig {
	return &TestnetConfig{
		ChainID:        "pole-testnet-1",
		NetworkName:    "PoLE Testnet",
		GenesisFile:    "config/testnet/genesis.json",
		DataDir:        "data/testnet",
		RPCPort:        ":9090",
		P2PPort:        ":26656",
		PrometheusPort: ":9091",
		EnableTLS:      false,
		Bootnodes:      []string{},
	}
}

// NetworkType 网络类型
type NetworkType string

const (
	NetworkMainnet NetworkType = "mainnet"
	NetworkTestnet NetworkType = "testnet"
)

// DetectNetwork 检测网络类型
func DetectNetwork(chainID string) NetworkType {
	if chainID == "pole-mainnet-1" {
		return NetworkMainnet
	}
	return NetworkTestnet
}

// IsMainnet 是否主网
func IsMainnet(chainID string) bool {
	return chainID == "pole-mainnet-1"
}

// LoadMainnetGenesis 加载主网创世
func LoadMainnetGenesis(path string) (*contracts.GenesisConfig, error) {
	return contracts.LoadGenesisConfig(path)
}

// CreateMainnetGenesis 创建主网创世
func CreateMainnetGenesis(path string, config *MainnetConfig) error {
	genesis := contracts.DefaultGenesisConfig()
	genesis.ChainID = config.ChainID
	genesis.Token.Name = "Proof of Live Engagement"
	genesis.Token.Symbol = "POLE"
	genesis.Token.Decimals = 18
	genesis.Token.TotalSupply = new(big.Int).Mul(big.NewInt(1_000_000_000), pow10(18))

	// 白皮书分配
	genesis.Allocations = []contracts.GenesisAllocation{
		{Label: "NodeRewardPool", Address: "", Amount: big.NewInt(0).Mul(big.NewInt(600_000_000), pow10(18))}, // 60%
		{Label: "Ecosystem", Address: "", Amount: big.NewInt(0).Mul(big.NewInt(200_000_000), pow10(18))},    // 20%
		{Label: "Community", Address: "", Amount: big.NewInt(0).Mul(big.NewInt(150_000_000), pow10(18))},    // 15%
		{Label: "TeamAndInvestors", Address: "", Amount: big.NewInt(0).Mul(big.NewInt(50_000_000), pow10(18))}, // 5%
	}

	// 保存创世文件
	data, _ := json.MarshalIndent(genesis, "", "  ")
	return os.WriteFile(path, data, 0644)
}
