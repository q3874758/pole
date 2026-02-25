package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"pole-core/common/metrics"
	"pole-core/common/monitor"
	"pole-core/contracts"
	"pole-core/core/consensus"
	"pole-core/core/executor"
	"pole-core/core/security"
	"pole-core/core/state"
	"pole-core/data/collector"
	"pole-core/execution/engine"
	"pole-core/governance"
	"pole-core/network/p2p"
	"pole-core/rpc"
	"pole-core/token/economy"
	"pole-core/token/rewards"
	"pole-core/wallet"
)

var (
	genesisPath = flag.String("genesis", "config/genesis.json", "path to genesis file")
	dataDir    = flag.String("data-dir", "data", "data directory")
	chainID    = flag.String("chain-id", "", "chain ID (overrides genesis)")
	network    = flag.String("network", "mainnet", "network type: mainnet, testnet")
	rpcPort    = flag.String("rpc-port", ":9090", "RPC port")
	p2pPort    = flag.String("p2p-port", ":26656", "P2P port")
	promPort   = flag.String("prometheus-port", ":9091", "Prometheus port")
	tlsEnabled = flag.Bool("tls", false, "enable TLS")
	bootnodes  = flag.String("bootnodes", "", "comma-separated bootnode addresses")
	logLevel   = flag.String("log-level", "info", "log level: debug, info, warn, error")
	mining     = flag.Bool("mining", false, "enable play-to-earn mining (auto collect game data)")
	help       = flag.Bool("help", false, "show help")
)

func main() {
	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	// 检测网络类型
	isMainnet := *network == "mainnet"
	chainName := "PoLE Testnet"
	if isMainnet {
		chainName = "PoLE Mainnet"
	}

	fmt.Println("========================================")
	fmt.Printf("  %s Node\n", chainName)
	fmt.Println("========================================")
	fmt.Println()

	// 如果指定了 network 但没有指定 genesis，使用默认路径
	if *genesisPath == "config/genesis.json" {
		if isMainnet {
			*genesisPath = "config/mainnet/genesis.json"
		} else {
			*genesisPath = "config/testnet/genesis.json"
		}
	}

	// 1. 加载创世配置
	fmt.Println("[1] 加载创世配置...")
	genesis, err := loadGenesis(*genesisPath)
	if err != nil {
		fmt.Printf("    错误: %v\n", err)
		os.Exit(1)
	}

	// 覆盖 chainID（如果指定）
	if *chainID != "" {
		genesis.ChainID = *chainID
	} else if isMainnet && genesis.ChainID != "pole-mainnet-1" {
		genesis.ChainID = "pole-mainnet-1"
	}

	fmt.Printf("    ChainID: %s\n", genesis.ChainID)
	fmt.Printf("    代币: %s (%s)\n", genesis.Token.Name, genesis.Token.Symbol)
	fmt.Printf("    分配账户数: %d\n", len(genesis.Allocations))

	// 2. 初始化链状态（含合约）
	fmt.Println("[2] 初始化链状态...")
	chainState := state.NewChainState(*dataDir)

	// 尝试加载已有状态
	if err := chainState.LoadState(); err != nil {
		fmt.Printf("    警告: 加载状态失败: %v\n", err)
	}

	// 如果没有已有状态，用创世初始化
	if chainState.GetHeight() == 0 {
		fmt.Println("    使用创世初始化...")
		if err := chainState.InitWithGenesis(genesis); err != nil {
			fmt.Printf("    错误: 初始化创世失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("    创世初始化完成")
	} else {
		fmt.Printf("    已加载状态: 高度 %d\n", chainState.GetHeight())
	}

	// 3. 初始化执行器
	fmt.Println("[3] 初始化交易执行器...")
	exec := executor.NewExecutor(chainState)
	fmt.Println("    执行器就绪")

	// 4. 初始化共识
	fmt.Println("[4] 初始化共识模块...")
	consensusConfig := consensus.DefaultConfig()
	cons := consensus.NewHybridConsensus(consensusConfig)
	consState := cons.GetState()
	fmt.Printf("    共识初始高度: %d\n", consState.Height)
	_ = cons

	// 5. 初始化数据采集器
	fmt.Println("[5] 初始化数据采集器...")
	collectorConfig := collector.DefaultConfig()
	dc := collector.NewDataCollector(collectorConfig)
	steamCollector := collector.NewSteamCollector("")
	dc.RegisterCollector(steamCollector)
	_ = dc
	fmt.Println("    已注册 Steam 采集器")

	// 5.1 启动自动采集循环（挖矿模式）
	var collectionLoop *collector.CollectionLoop
	var rewardDistributor *collector.MiningRewardDistributor
	const miningPoolAddr = "pole1miningpool00000000000000000000"
	if *mining {
		fmt.Println("    [5.1] 启动挖矿自动采集...")
		loopConfig := collector.DefaultLoopConfig()
		collectionLoop = collector.NewCollectionLoop(loopConfig, dc)

		// 初始化奖励池和分发器（对接链上代币合约）
		rewardCalc := collector.NewMiningRewardCalculator(nil)
		rewardPool := collector.NewMiningRewardPool(rewardCalc)
		var tokenAdapter collector.TokenContractInterface
		if chainState.GetToken() != nil {
			tokenAdapter = &tokenContractAdapter{token: chainState.GetToken()}
		}
		rewardDistributor = collector.NewMiningRewardDistributor(rewardPool, tokenAdapter, miningPoolAddr)

		// 加载持久化的待领奖励
		if err := rewardDistributor.Load(*dataDir + "/mining_rewards.json"); err != nil {
			// 首次无文件可忽略
		}

		// 启动采集循环
		collectionLoop.Start()
		fmt.Println("    挖矿采集循环已启动")
	}

	// 6. 初始化 GVS 计算器
	fmt.Println("[6] 初始化 GVS 计算器...")
	engineConfig := engine.DefaultConfig()
	gvsCalc := engine.NewGvsCalculator(engineConfig)
	_ = gvsCalc
	fmt.Println("    GVS 计算器就绪")

	// 6.1 初始化 P2P 网络
	fmt.Println("[6.1] 初始化 P2P 网络...")
	p2pConfig := p2p.DefaultConfig()
	p2pConfig.ListenAddr = *p2pPort
	if *bootnodes != "" {
		// 解析 bootnodes
		p2pConfig.BootstrapNodes = parseBootnodes(*bootnodes)
	}
	var p2pNet *p2p.Network
	p2pNet = p2p.NewNetwork(p2pConfig)
	if err := p2pNet.Start(); err != nil {
		fmt.Printf("    警告: P2P 网络启动失败: %v\n", err)
	} else {
		fmt.Printf("    P2P 网络已启动，监听: %s\n", *p2pPort)
	}

	// 设置 P2P 数据确认回调（挖矿模式）
	if *mining && rewardDistributor != nil && p2pNet != nil {
		p2pNet.OnDataConfirmed = rewardDistributor.OnDataConfirmed
		// 设置为验证节点模式以接收确认
		p2pNet.SetValidatorMode(true)
	}

	// 7. 初始化代币经济
	fmt.Println("[7] 初始化代币经济...")
	economyConfig := economy.DefaultConfig()
	tokenEco := economy.NewTokenEconomy(economyConfig)
	supplyInfo := tokenEco.GetSupplyInfo()
	fmt.Printf("    总供应量: %s\n", supplyInfo.TotalSupply.String())

	// 8. 初始化奖励分发（含 Slash 管理器）
	fmt.Println("[8] 初始化奖励分发...")
	rewardConfig := rewards.DefaultConfig()
	rewardDist := rewards.NewRewardDistributor(rewardConfig)
	_ = rewardDist
	fmt.Println("    奖励分发器就绪")

	// 8.1 初始化安全模块
	fmt.Println("[8.1] 初始化安全模块...")

	// TLS 配置
	tlsConfig := security.DefaultTLSConfig()
	tlsConfig.Enabled = *tlsEnabled
	// 如果启用 TLS 但证书不存在，自动生成
	if *tlsEnabled {
		tlsConfig.CertFile = *dataDir + "/node.crt"
		tlsConfig.KeyFile = *dataDir + "/node.key"
		if err := security.GenerateSelfSignedCert(tlsConfig.CertFile, tlsConfig.KeyFile, "localhost", 365); err != nil {
			fmt.Printf("    警告: TLS 证书生成失败: %v\n", err)
		} else {
			fmt.Printf("    TLS 证书已生成: %s\n", tlsConfig.CertFile)
		}
	}
	tlsServer, err := security.LoadTLSConfig(tlsConfig)
	if err != nil {
		fmt.Printf("    警告: TLS 配置加载失败: %v\n", err)
	}
	if tlsServer != nil {
		fmt.Println("    TLS 1.3 已就绪")
	} else if *tlsEnabled {
		fmt.Println("    TLS 未启用")
	}

	// 声誉管理器
	reputationMgr := security.NewReputationManager(nil)
	if err := reputationMgr.LoadFromFile(*dataDir + "/reputation.json"); err != nil {
		fmt.Printf("    声誉数据: 首次运行\n")
	}

	// 渐进式惩罚管理器
	progressivePenaltyMgr := security.NewProgressivePenaltyManager(nil)
	_ = progressivePenaltyMgr
	fmt.Println("    渐进式惩罚已就绪")

	// 紧急暂停管理器
	emergencyPauseMgr := governance.NewEmergencyPauseManager(nil)
	fmt.Println("    紧急暂停机制已就绪")

	// 9. 初始化监控模块
	fmt.Println("[9] 初始化监控模块...")

	// 系统指标
	sysMetrics := metrics.NewSystemMetrics()
	metrics.Register("block_height", sysMetrics.BlockHeight)
	metrics.Register("tx_count", sysMetrics.TxCount)
	metrics.Register("peer_count", sysMetrics.PeerCount)
	metrics.Register("error_count", sysMetrics.ErrorCount)

	// Prometheus 导出器
	promExporter := monitor.NewPrometheusExporter(sysMetrics, nil)
	if err := promExporter.Start(); err != nil {
		fmt.Printf("    警告: Prometheus 启动失败: %v\n", err)
	} else {
		fmt.Printf("    Prometheus: http://localhost%s/metrics\n", *promPort)
		fmt.Printf("    健康检查: http://localhost%s/health\n", *promPort)
	}

	// 告警管理器
	alertMgr := monitor.NewAlertManager()
	// 注册默认告警规则
	for _, rule := range monitor.DefaultAlertRules() {
		alertMgr.RegisterRule(rule)
	}
	alertMgr.RegisterHandler(monitor.NewLogAlertHandler())
	fmt.Println("    告警管理器已启动")

	// 10. 初始化钱包
	fmt.Println("[10] 初始化钱包...")
	w := wallet.NewWallet()
	if err := w.Load("wallet.json"); err != nil {
		fmt.Printf("    未找到钱包文件，将创建新钱包\n")
	} else {
		fmt.Printf("    已加载钱包: %d 个账户\n", len(w.ListAccounts()))
	}
	fmt.Println("    钱包就绪")

	// 11. 启动 RPC 服务器
	fmt.Println("[11] 启动 RPC 服务器...")
	rpcConfig := rpc.DefaultConfig()
	rpcConfig.Port = *rpcPort
	if *tlsEnabled {
		rpcConfig.EnableTLS = true
		rpcConfig.CertFile = tlsConfig.CertFile
		rpcConfig.KeyFile = tlsConfig.KeyFile
	}

	rpcServer := rpc.NewHTTPServer(chainState, exec, w, rpcConfig)
	rpcServer.SetEmergencyPause(emergencyPauseMgr)
	if rewardDistributor != nil {
		rpcServer.SetMiningRewardDistributor(rewardDistributor)
		if collectionLoop != nil {
			rpcServer.SetCollectionLoop(collectionLoop)
		}
		// 定期持久化挖矿待领奖励
		go func() {
			tick := time.NewTicker(2 * time.Minute)
			defer tick.Stop()
			for range tick.C {
				if err := rewardDistributor.Save(*dataDir + "/mining_rewards.json"); err != nil {
					// 仅打日志，不中断
				}
			}
		}()
		// 挖矿奖励自动发放：定期为所有有待领（≥3 确认）的地址执行链上转账
		go func() {
			tick := time.NewTicker(5 * time.Minute)
			defer tick.Stop()
			for range tick.C {
				rewardDistributor.AutoClaimAll()
				_ = rewardDistributor.Save(*dataDir + "/mining_rewards.json") // 发放后立即持久化，避免重启重复发放
			}
		}()
	}

	if err := rpcServer.Start(); err != nil {
		fmt.Printf("    警告: RPC 启动失败: %v\n", err)
	} else {
		scheme := "http"
		if *tlsEnabled {
			scheme = "https"
		}
		fmt.Printf("    RPC: %s://localhost%s/\n", scheme, *rpcPort)
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  节点初始化完成!")
	if isMainnet {
		fmt.Println("  网络: 主网 (pole-mainnet-1)")
	} else {
		fmt.Println("  网络: 测试网")
	}
	fmt.Println("  按 Ctrl+C 退出...")
	fmt.Println("========================================")

	fmt.Scanln()
}

// parseBootnodes 解析 bootnodes 字符串
func parseBootnodes(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, n := range strings.Split(s, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			result = append(result, n)
		}
	}
	return result
}

func loadGenesis(path string) (*contracts.GenesisConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("    创世文件不存在，使用默认配置: %s\n", path)
		return contracts.DefaultGenesisConfig(), nil
	}
	return contracts.LoadGenesisConfig(path)
}

// tokenContractAdapter 将 contracts.TokenContract 适配为 collector.TokenContractInterface
type tokenContractAdapter struct {
	token *contracts.TokenContract
}

func (a *tokenContractAdapter) Transfer(ctx context.Context, from, to string, amount *big.Int) error {
	if a.token == nil {
		return fmt.Errorf("token contract not set")
	}
	return a.token.Transfer(ctx, from, to, amount)
}

func (a *tokenContractAdapter) GetBalance(addr string) *big.Int {
	if a.token == nil {
		return big.NewInt(0)
	}
	return a.token.BalanceOf(addr)
}
