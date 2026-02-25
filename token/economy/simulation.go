package economy

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

// ==================== 模拟配置 ====================

// SimulationConfig 经济模拟配置
type SimulationConfig struct {
	InitialSupply        int64   // 初始供应量
	InitialNodes         int     // 初始节点数
	MonthlyNodeGrowth    float64 // 月度节点增长率
	BaseInflationRate    float64 // 基础通胀率
	FeeBurnPercentage   float64 // 手续费燃烧比例
	TransactionPerDay   int64   // 每日交易量
	AverageGasPrice     float64 // 平均 Gas 价格
	Years               int     // 模拟年数
}

// MarketScenario 市场场景
type MarketScenario string

const (
	ScenarioBull    MarketScenario = "bull"    // 牛市
	ScenarioBear    MarketScenario = "bear"    // 熊市
	ScenarioNormal  MarketScenario = "normal"  // 正常
	ScenarioStress MarketScenario = "stress"  // 压力
)

// ScenarioConfig 场景配置
var ScenarioConfigs = map[MarketScenario]SimulationConfig{
	ScenarioBull: {
		InitialSupply:     1_000_000_000,
		InitialNodes:       1000,
		MonthlyNodeGrowth:  0.15,  // +15%/月
		BaseInflationRate:  0.20,
		FeeBurnPercentage: 0.25,
		TransactionPerDay: 1_000_000,
		AverageGasPrice:    0.0001,
		Years:             10,
	},
	ScenarioBear: {
		InitialSupply:     1_000_000_000,
		InitialNodes:       1000,
		MonthlyNodeGrowth:  -0.05, // -5%/月
		BaseInflationRate:  0.20,
		FeeBurnPercentage:  0.25,
		TransactionPerDay:   100_000,
		AverageGasPrice:    0.00005,
		Years:             10,
	},
	ScenarioNormal: {
		InitialSupply:     1_000_000_000,
		InitialNodes:       1000,
		MonthlyNodeGrowth:  0.05,   // +5%/月
		BaseInflationRate:  0.20,
		FeeBurnPercentage:  0.25,
		TransactionPerDay:  500_000,
		AverageGasPrice:    0.0001,
		Years:             10,
	},
	ScenarioStress: {
		InitialSupply:     1_000_000_000,
		InitialNodes:       1000,
		MonthlyNodeGrowth:  0.02,  // +2%/月
		BaseInflationRate:  0.20,
		FeeBurnPercentage:  0.25,
		TransactionPerDay:  50_000,
		AverageGasPrice:    0.00001,
		Years:             10,
	},
}

// ==================== 模拟结果 ====================

// SimulationResult 模拟结果
type SimulationResult struct {
	Year                int     // 年份
	Nodes               int     // 节点数
	CirculatingSupply  int64   // 流通量
	InflationRate      float64 // 通胀率
	NetInflationRate   float64 // 净通胀率
	FeeBurned          int64   // 燃烧量
	APR                float64 // 年化收益率
}

// SustainabilityScore 可持续性评分
type SustainabilityScore struct {
	Score              int     // 综合评分 (0-100)
	NodeGrowthScore    int     // 节点增长评分
	FeeIncomeScore     int     // 手续费收入评分
	InflationScore    int     // 通胀控制评分
	BurnEfficiencyScore int    // 燃烧效率评分
	ParticipationScore int     // 参与度评分
}

// ==================== 经济模拟器 ====================

// EconomicSimulator 经济模拟器
type EconomicSimulator struct {
	config  *SimulationConfig
	rand    *rand.Rand
}

// NewEconomicSimulator 创建模拟器
func NewEconomicSimulator(config *SimulationConfig) *EconomicSimulator {
	if config == nil {
		cfg := ScenarioConfigs[ScenarioNormal] // copy
		config = &cfg
	}
	return &EconomicSimulator{
		config: config,
		rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RunSimulation 运行模拟
func (es *EconomicSimulator) RunSimulation() []SimulationResult {
	results := make([]SimulationResult, 0, es.config.Years)

	// 初始值
	circulatingSupply := es.config.InitialSupply
	nodes := es.config.InitialNodes
	inflationRate := es.config.BaseInflationRate
	monthlyGrowth := es.config.MonthlyNodeGrowth

	// 每年铸造量
	annualMint := float64(es.config.InitialSupply) * inflationRate

	for year := 1; year <= es.config.Years; year++ {
		// 模拟节点增长
		monthlyChange := float64(nodes) * monthlyGrowth
		nodes = int(float64(nodes) + monthlyChange*12)
		if nodes < 10 {
			nodes = 10 // 最少10个节点
		}

		// 模拟交易燃烧
		annualFee := float64(es.config.TransactionPerDay) * 365 * es.config.AverageGasPrice
		feeBurned := int64(float64(annualFee) * es.config.FeeBurnPercentage * 1e18)

		// 净通胀
		netInflation := annualMint - float64(feeBurned)
		netInflationRate := netInflation / float64(circulatingSupply)

		// 更新流通量
		circulatingSupply += int64(annualMint) - feeBurned

		// 计算 APR (简化)
		apr := 0.0
		if nodes > 0 {
			apr = (annualMint - float64(feeBurned)) / float64(nodes) / 10000 * 100
		}

		result := SimulationResult{
			Year:               year,
			Nodes:              nodes,
			CirculatingSupply:  circulatingSupply,
			InflationRate:      inflationRate,
			NetInflationRate:   netInflationRate,
			FeeBurned:          feeBurned,
			APR:                apr,
		}
		results = append(results, result)

		// 通胀衰减 (每两年减半)
		if year%2 == 0 {
			inflationRate *= 0.5
			annualMint = float64(es.config.InitialSupply) * inflationRate
		}
	}

	return results
}

// RunAllScenarios 运行所有场景模拟
func (es *EconomicSimulator) RunAllScenarios() map[MarketScenario][]SimulationResult {
	results := make(map[MarketScenario][]SimulationResult)
	for scenario, config := range ScenarioConfigs {
		es.config = &config
		results[scenario] = es.RunSimulation()
	}
	return results
}

// CalculateSustainabilityScore 计算可持续性评分
func (es *EconomicSimulator) CalculateSustainabilityScore(result SimulationResult) SustainabilityScore {
	score := SustainabilityScore{}

	// 节点增长评分 (20%)
	nodeGrowthMonthly := math.Pow(float64(result.Nodes)/float64(es.config.InitialNodes), 1.0/float64(result.Year)) - 1
	if nodeGrowthMonthly >= 0.10 {
		score.NodeGrowthScore = 100
	} else if nodeGrowthMonthly > 0 {
		score.NodeGrowthScore = int(nodeGrowthMonthly * 10 * 100)
	} else {
		score.NodeGrowthScore = 0
	}

	// 手续费收入评分 (25%) - 假设成本覆盖50%为100分
	annualFee := float64(es.config.TransactionPerDay) * 365 * es.config.AverageGasPrice * 1e18
	nodeCost := 10000.0 // 每个节点年成本 (POLE)
	feeCoverage := (annualFee * 1e18) / (float64(result.Nodes) * nodeCost)
	if feeCoverage >= 0.5 {
		score.FeeIncomeScore = 100
	} else {
		score.FeeIncomeScore = int(feeCoverage * 200)
	}

	// 通胀控制评分 (20%)
	if result.NetInflationRate <= 0.02 {
		score.InflationScore = 100
	} else if result.NetInflationRate <= 0.05 {
		score.InflationScore = 80
	} else if result.NetInflationRate <= 0.10 {
		score.InflationScore = 60
	} else {
		score.InflationScore = 40
	}

	// 燃烧效率评分 (20%)
	if result.FeeBurned > 0 && result.InflationRate > 0 {
		burnEfficiency := float64(result.FeeBurned) / (float64(es.config.InitialSupply) * result.InflationRate)
		if burnEfficiency >= 1.0 {
			score.BurnEfficiencyScore = 100
		} else {
			score.BurnEfficiencyScore = int(burnEfficiency * 100)
		}
	}

	// 参与度评分 (15%)
	circulationRatio := float64(result.CirculatingSupply) / float64(es.config.InitialSupply)
	if circulationRatio >= 0.5 {
		score.ParticipationScore = 100
	} else {
		score.ParticipationScore = int(circulationRatio * 200)
	}

	// 综合评分
	score.Score = score.NodeGrowthScore*20/100 + score.FeeIncomeScore*25/100 +
		score.InflationScore*20/100 + score.BurnEfficiencyScore*20/100 +
		score.ParticipationScore*15/100

	return score
}

// PrintResults 打印模拟结果
func (es *EconomicSimulator) PrintResults(results []SimulationResult) {
	fmt.Println("\n========== 经济模拟结果 ==========")
	fmt.Printf("%-6s %-8s %-12s %-10s %-10s %-10s %-8s\n",
		"年份", "节点数", "流通量", "通胀率", "净通胀率", "燃烧量", "APR%")
	fmt.Println("------------------------------------------------------------")

	for _, r := range results {
		fmt.Printf("%-6d %-8d %-12d %-9.2f%% %-9.2f%% %-10d %-7.2f%%\n",
			r.Year, r.Nodes, r.CirculatingSupply,
			r.InflationRate*100, r.NetInflationRate*100,
			r.FeeBurned, r.APR)
	}
}

// PrintSustainabilityScore 打印可持续性评分
func (es *EconomicSimulator) PrintSustainabilityScore(ss SustainabilityScore) {
	fmt.Println("\n========== 可持续性评分 ==========")
	fmt.Printf("综合评分: %d/100\n", ss.Score)
	fmt.Printf("节点增长: %d/100 (权重20%%)\n", ss.NodeGrowthScore)
	fmt.Printf("手续费收入: %d/100 (权重25%%)\n", ss.FeeIncomeScore)
	fmt.Printf("通胀控制: %d/100 (权重20%%)\n", ss.InflationScore)
	fmt.Printf("燃烧效率: %d/100 (权重20%%)\n", ss.BurnEfficiencyScore)
	fmt.Printf("参与度: %d/100 (权重15%%)\n", ss.ParticipationScore)

	if ss.Score >= 75 {
		fmt.Println("\n评级: 优秀 (可持续)")
	} else if ss.Score >= 50 {
		fmt.Println("\n评级: 良好 (基本可持续)")
	} else {
		fmt.Println("\n评级: 需改进 (存在风险)")
	}
}
