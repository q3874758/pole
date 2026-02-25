package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/sha3"
)

// ============ PoLE 主网节点 ============

var (
	Version   = "1.0.0"
	ChainID    = int64(1) // PoLE 主网 ChainID
	NetworkID  = uint64(1)
	Bootnodes = []string{
		"/ip4/127.0.0.1/tcp/30303/p2p/node1",
		"/ip4/127.0.0.1/tcp/30304/p2p/node2",
	}
)

// 配置
type Config struct {
	ChainID       int64
	NetworkID    uint64
	DataDir      string
	KeyFile      string
	Password     string
	HTTPHost     string
	HTTPPort    int
	WSHost       string
	WSPort       int
	Bootnodes    []string
	MaxPeers     int
	GasPrice     string
	Identity     string
}

var (
	logger       = zerolog.New(os.Stderr).With().Timestamp().Logger()
	globalConfig *Config
)

// ============ 主网命令 ============

var mainnetCmd = &cobra.Command{
	Use:   "mainnet",
	Short: "启动 PoLE 主网节点",
	RunE:  runMainnet,
}

func init() {
	rootCmd.AddCommand(mainnetCmd)
	mainnetCmd.Flags().StringVar(&globalConfig.DataDir, "datadir", "./data", "数据目录")
	mainnetCmd.Flags().StringVar(&globalConfig.KeyFile, "keyfile", "", "密钥文件")
	mainnetCmd.Flags().StringVar(&globalConfig.Password, "password", "", "密码")
	mainnetCmd.Flags().StringVar(&globalConfig.HTTPHost, "http.host", "localhost", "HTTP RPC 主机")
	mainnetCmd.Flags().IntVar(&globalConfig.HTTPPort, "http.port", 8545, "HTTP RPC 端口")
	mainnetCmd.Flags().StringVar(&globalConfig.WSHost, "ws.host", "localhost", "WebSocket 主机")
	mainnetCmd.Flags().IntVar(&globalConfig.WSPort, "8546,", "ws.port", "WebSocket 端口")
	mainnetCmd.Flags().IntVar(&globalConfig.MaxPeers, "maxpeers", 50, "最大对等节点数")
	mainnetCmd.Flags().StringVar(&globalConfig.Bootnodes, "bootnodes", "", "引导节点")
}

func runMainnet(cmd *cobra.Command, args []string) error {
	logger.Info().Str("version", Version).Msg("PoLE 主网节点启动")
	
	// 设置主网配置
	configureMainnet()
	
	// 创建节点
	n, err := createNode()
	if err != nil {
		return fmt.Errorf("创建节点失败: %w", err)
	}
	
	// 启动 P2P
	if err := startP2P(n); err != nil {
		return fmt.Errorf("P2P 启动失败: %w", err)
	}
	
	// 启动以太坊栈
	stack, err := startEthStack(n)
	if err != nil {
		return fmt.Errorf("以太坊栈启动失败: %w", err)
	}
	
	// 注册 EVM 模块
	registerEVMModules(stack)
	
	// 等待退出信号
	waitForSignal()
	
	return nil
}

func configureMainnet() {
	if globalConfig == nil {
		globalConfig = &Config{
			ChainID:    ChainID,
			NetworkID:  NetworkID,
			DataDir:    "./pole-data",
			HTTPHost:   "0.0.0.0",
			HTTPPort:   8545,
			WSHost:     "0.0.0.0",
			WSPort:     8546,
			MaxPeers:   50,
			Bootnodes:  Bootnodes,
		}
	}
}

// ============ P2P 网络层 (libp2p) ============

type P2PNode struct {
	host    host.Interface
	discovery *mdns.MDNS
	peers    map[peer.ID]*peerInfo
}

type peerInfo struct {
	addr      string
	latency   time.Duration
	connected time.Time
}

func createNode() (*P2PNode, error) {
	// 创建 libp2p 节点
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/30303"),
		libp2p.Identity(globalConfig.Identity),
		libp2p.Security(libp2p.SecioSec),
		libp2p.Ping(true),
	}

	host, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	p2pNode := &P2PNode{
		host:  host,
		peers: make(map[peer.ID]*peerInfo),
	}

	// 启动 mDNS 发现
	p2pNode.startDiscovery()

	logger.Info().Str("peer_id", host.ID().String()).Msg("P2P 节点已启动")
	
	return p2pNode, nil
}

func (n *P2PNode) startDiscovery() {
	// mDNS 发现
	mdns := mdns.NewMdnsService(n.host, "pole-mainnet", n)
	n.discovery = mdns
	mdns.Start()
}

func (n *P2PNode) HandlePeerFound(p *peer.ID, addr multiaddr.Multiaddr) {
	logger.Info().Str("peer", p.String()).Msg("发现新节点")
	n.peers[*p] = &peerInfo{
		addr:      addr.String(),
		connected: time.Now(),
	}
}

func startP2P(n *P2PNode) error {
	// 连接引导节点
	for _, bn := range globalConfig.Bootnodes {
		if err := connectToBootnode(n, bn); err != nil {
			logger.Warn().Str("bootnode", bn).Err(err).Msg("连接引导节点失败")
		}
	}
	return nil
}

func connectToBootnode(n *P2PNode, addr string) error {
	parsed, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return err
	}

	info, err := peer.AddrInfoFromP2pAddr(parsed)
	if err != nil {
		return err
	}

	return n.host.Connect(n.Context(), *info)
}

// ============ 以太坊栈 ============

type EthStack struct {
	node      *node.Node
	eth       *eth.Ethereum
	backend   *ethclient.Client
	chain     *core.BlockChain
}

func startEthStack(p2p *P2PNode) (*EthStack, error) {
	// 创建节点配置
	nodeConfig := node.DefaultConfig
	nodeConfig.DataDir = globalConfig.DataDir
	nodeConfig.Name = "pole"
	nodeConfig.Version = Version
	nodeConfig.P2P = &p2p.Config{
		PrivateKey: generateKey(),
		MaxPeers:  globalConfig.MaxPeers,
	}
	nodeConfig.HTTPPort = fmt.Sprintf("%d", globalConfig.HTTPPort)
	nodeConfig.WSPort = fmt.Sprintf("%d", globalConfig.WSPort)

	// 创建节点
	stack, err := node.New(&nodeConfig)
	if err != nil {
		return nil, err
	}

	// 以太坊配置
	ethConfig := eth.DefaultConfig
	ethConfig.Genesis = getMainnetGenesis()
	ethConfig.NetworkId = globalConfig.NetworkID
	ethConfig.chainId = big.NewInt(globalConfig.ChainID)

	// 启动以太坊
	eth, err := eth.New(stack, &ethConfig)
	if err != nil {
		return nil, err
	}

	stack.RegisterAPIs(eth.APIs())

	// 启动节点
	if err := stack.Start(); err != nil {
		return nil, err
	}

	// 创建后端
	backend := ethclient.NewClient(stack.Attach())

	logger.Info().
		Str("http", fmt.Sprintf("%s:%d", globalConfig.HTTPHost, globalConfig.HTTPPort)).
		Str("ws", fmt.Sprintf("%s:%d", globalConfig.WSHost, globalConfig.WSPort)).
		Msg("以太坊服务已启动")

	return &EthStack{
		node:    stack,
		eth:     eth,
		backend: backend,
	}, nil
}

// ============ EVM 模块 ============

type EVMModule struct {
	stack *EthStack
}

func registerEVMModules(stack *node.Node) {
	module := &EVMModule{}
	stack.RegisterAPIs([]rpc.API{
		{
			Namespace: "pole",
			Version:   "1.0",
			Service:   module,
			Public:    true,
		},
	})
}

// PoLE 特定 EVM 方法

// GetGVS 获取游戏价值分数
func (m *EVMModule) GetGVS(gameID string) (map[string]interface{}, error) {
	// 简化实现
	return map[string]interface{}{
		"game_id":     gameID,
		"gvs_score":   100,
		"players":      1000,
		"play_time":   5000,
		"revenue":     10000,
		"calculation": "SHA256(game_data)",
	}, nil
}

// SubmitGameData 提交游戏数据
func (m *EVMModule) SubmitGameData(gameID string, data string) (string, error) {
	// 计算哈希
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:]), nil
}

// Stake 质押
func (m *EVMModule) Stake(amount *big.Int) (string, error) {
	// 简化实现
	txHash := crypto.Keccak256Hash([]byte(time.Now().String()))
	return txHash.Hex(), nil
}

// GetValidatorInfo 获取验证者信息
func (m *EVMModule) GetValidatorInfo(addr string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"address":      addr,
		"stake":       10000,
		"delegators":  5,
		"rewards":     100,
		"uptime":      99.9,
	}, nil
}

// ============ 创世区块 ============

func getMainnetGenesis() *core.Genesis {
	return &core.Genesis{
		Config:     params.MainnetChainConfig,
		Nonce:      0,
		ExtraData:  []byte("PoLE Mainnet Genesis"),
		GasLimit:   8000000,
		Difficulty: big.NewInt(1),
		Mixhash:    common.Hash{},
		Coinbase:   common.Address{},
		Alloc:      core.GenesisAlloc{},
		Number:     0,
		GasUsed:    0,
		ParentHash: common.Hash{},
		BaseFee:    params.MainnetChainConfig.EIP1559Compatible,
	}
}

// ============ 工具函数 ============

func generateKey() *ecdsa.PrivateKey {
	key, _ := btcec.GeneratePrivateKey()
	return key
}

func waitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	logger.Info().Msg("正在关闭节点...")
}

// ============ 主函数 ============

func main() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error().Err(err).Msg("执行失败")
		os.Exit(1)
	}
}
