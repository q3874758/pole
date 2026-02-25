package main

import (
	"fmt"
	"os"

	"pole-core/core/consensus"
	"pole-core/core/types"
	"pole-core/data/collector"
	"pole-core/execution/engine"
	"pole-core/token/economy"
	"pole-core/token/rewards"
	"pole-core/governance"
)

func main() {
	fmt.Println("========================================")
	fmt.Println("  PoLE Core - Go Implementation")
	fmt.Println("========================================")
	fmt.Println()
	
	// 初始化核心模块
	fmt.Println("[1] 初始化共识模块...")
	consensusConfig := consensus.DefaultConfig()
	cons := consensus.NewHybridConsensus(consensusConfig)
	state := cons.GetState()
	fmt.Printf("    初始高度: %d\n", state.Height)
	
	// 初始化数据采集器
	fmt.Println("[2] 初始化数据采集器...")
	collectorConfig := collector.DefaultConfig()
	dc := collector.NewDataCollector(collectorConfig)
	steamCollector := collector.NewSteamCollector("")
	dc.RegisterCollector(steamCollector)
	fmt.Println("    已注册 Steam 采集器")
	
	// 初始化 GVS 计算器
	fmt.Println("[3] 初始化 GVS 计算器...")
	engineConfig := engine.DefaultConfig()
	gvsCalc := engine.NewGvsCalculator(engineConfig)
	fmt.Println("    GVS 计算器就绪")
	
	// 初始化代币经济
	fmt.Println("[4] 初始化代币经济...")
	economyConfig := economy.DefaultConfig()
	tokenEco := economy.NewTokenEconomy(economyConfig)
	supplyInfo := tokenEco.GetSupplyInfo()
	fmt.Printf("    总供应量: %s\n", supplyInfo.TotalSupply.String())
	fmt.Printf("    通胀率: %.2f%%\n", tokenEco.GetInflationRate()*100)
	
	// 初始化奖励分发
	fmt.Println("[5] 初始化奖励分发...")
	rewardConfig := rewards.DefaultConfig()
	_ = rewards.NewRewardDistributor(rewardConfig)
	fmt.Println("    奖励分发器就绪")
	
	// 初始化治理
	fmt.Println("[6] 初始化治理...")
	governanceConfig := governance.DefaultConfig()
	gov := governance.NewGovernance(governanceConfig)
	params := gov.GetParams()
	fmt.Printf("    最小提案质押: %s\n", params.MinValidatorStake.String())
	
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  所有模块初始化完成!")
	fmt.Println("========================================")
	
	// 创建测试数据
	fmt.Println()
	fmt.Println("[测试] 创建测试数据...")
	
	// 创建测试节点
	testAddr := types.FromPublicKey([]byte("test"))
	validator := types.Validator{
		Address:     testAddr,
		PublicKey:   []byte("test_pubkey"),
		Stake:       types.MustNewDecimal("10000"),
		Delegations: types.MustNewDecimal("5000"),
		Commission:  10,
		Status:      types.ValidatorStatusActive,
		Moniker:     "Test Validator",
	}
	
	cons.UpdateValidators([]types.Validator{validator})
	fmt.Printf("    已添加测试验证者: %s\n", validator.Moniker)
	
	// 测试 GVS 计算
	testDataPoints := []types.GameDataPoint{
		{
			GameID:        "730",
			OnlinePlayers: 1000,
			PeakPlayers:  1500,
			Timestamp:     1700000000,
			Tier:          types.Tier1,
		},
		{
			GameID:        "730",
			OnlinePlayers: 1200,
			PeakPlayers:  1500,
			Timestamp:     1700000100,
			Tier:          types.Tier1,
		},
	}
	
	gvs := gvsCalc.Calculate(testDataPoints)
	gvsCalc.RecordGVS("730", gvs)
	fmt.Printf("    测试游戏 GVS: %.2f\n", gvs)
	
	// 测试数据采集
	games, err := dc.CollectGames(collector.PlatformSteam, []string{"730", "570"})
	if err != nil {
		fmt.Printf("    警告: 数据采集失败: %v\n", err)
	} else {
		fmt.Printf("    成功采集 %d 个游戏数据\n", len(games))
	}
	
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  PoLE Core 测试完成!")
	fmt.Println("========================================")
	
	os.Exit(0)
}
