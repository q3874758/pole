package economy

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ==================== 动态费用配置 ====================

// DynamicFeeConfig 动态费用配置
type DynamicFeeConfig struct {
	BaseGasPrice       decimal.Decimal // 基础 Gas 价格
	MaxGasPrice       decimal.Decimal // 最大 Gas 价格
	MinGasPrice       decimal.Decimal // 最小 Gas 价格
	GasAdjustment     decimal.Decimal // Gas 调整系数
	CongestionThreshold decimal.Decimal // 拥堵阈值 (0-1)
	Enabled            bool           // 是否启用动态费用
}

func DefaultDynamicFeeConfig() *DynamicFeeConfig {
	base, _ := decimal.NewFromString("0.0001") // 0.0001 POLE
	max, _ := decimal.NewFromString("1")       // 1 POLE
	min, _ := decimal.NewFromString("0.00001") // 0.00001 POLE
	return &DynamicFeeConfig{
		BaseGasPrice:        base,
		MaxGasPrice:        max,
		MinGasPrice:        min,
		GasAdjustment:      decimal.NewFromFloat(1.15),
		CongestionThreshold: decimal.NewFromFloat(0.5), // 50%
		Enabled:             true,
	}
}

// CongestionLevel 拥堵等级
type CongestionLevel string

const (
	CongestionLow      CongestionLevel = "Low"      // 0-30%
	CongestionMedium   CongestionLevel = "Medium"   // 30-70%
	CongestionHigh    CongestionLevel = "High"     // 70-90%
	CongestionCritical CongestionLevel = "Critical" // 90%+
)

// ==================== 动态费用管理器 ====================

// FeeManager 费用管理器
type FeeManager struct {
	config       *DynamicFeeConfig
	metrics     *NetworkMetrics
	mu          sync.RWMutex
}

type NetworkMetrics struct {
	BlockUtilization  decimal.Decimal // 区块利用率 (0-1)
	TxCount          int64           // 当前区块交易数
	AvgTxCount       decimal.Decimal // 平均交易数
	GasUsed          uint64          // 已使用 Gas
	GasLimit         uint64          // Gas 限制
	LastUpdateTime   time.Time
}

// NewFeeManager 创建费用管理器
func NewFeeManager(config *DynamicFeeConfig) *FeeManager {
	if config == nil {
		config = DefaultDynamicFeeConfig()
	}
	return &FeeManager{
		config: config,
		metrics: &NetworkMetrics{
			BlockUtilization: decimal.Zero,
			TxCount:         0,
			AvgTxCount:      decimal.Zero,
			GasUsed:         0,
			GasLimit:        50000000, // 默认 50M
			LastUpdateTime:   time.Now(),
		},
	}
}

// UpdateMetrics 更新网络指标
func (fm *FeeManager) UpdateMetrics(txCount int64, gasUsed uint64) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	fm.metrics.TxCount = txCount
	fm.metrics.GasUsed = gasUsed

	if fm.metrics.GasLimit > 0 {
		fm.metrics.BlockUtilization = decimal.NewFromInt(int64(gasUsed)).
			Div(decimal.NewFromInt(int64(fm.metrics.GasLimit)))
	}

	// 更新平均交易数 (简单移动平均)
	if fm.metrics.AvgTxCount.IsZero() {
		fm.metrics.AvgTxCount = decimal.NewFromInt(txCount)
	} else {
		fm.metrics.AvgTxCount = fm.metrics.AvgTxCount.Mul(decimal.NewFromFloat(0.9)).
			Add(decimal.NewFromInt(txCount).Mul(decimal.NewFromFloat(0.1)))
	}

	fm.metrics.LastUpdateTime = time.Now()
}

// GetCongestionLevel 获取拥堵等级
func (fm *FeeManager) GetCongestionLevel() CongestionLevel {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	util := fm.metrics.BlockUtilization
	if util.LessThan(decimal.NewFromFloat(0.3)) {
		return CongestionLow
	} else if util.LessThan(decimal.NewFromFloat(0.7)) {
		return CongestionMedium
	} else if util.LessThan(decimal.NewFromFloat(0.9)) {
		return CongestionHigh
	}
	return CongestionCritical
}

// GetFeeMultiplier 获取费用倍数
func (fm *FeeManager) GetFeeMultiplier() decimal.Decimal {
	level := fm.GetCongestionLevel()
	switch level {
	case CongestionLow:
		return decimal.NewFromFloat(1.0)
	case CongestionMedium:
		return decimal.NewFromFloat(1.5)
	case CongestionHigh:
		return decimal.NewFromFloat(2.0)
	case CongestionCritical:
		return decimal.NewFromFloat(3.0)
	default:
		return decimal.NewFromFloat(1.0)
	}
}

// GetCurrentGasPrice 获取当前 Gas 价格
func (fm *FeeManager) GetCurrentGasPrice() decimal.Decimal {
	if !fm.config.Enabled {
		return fm.config.BaseGasPrice
	}

	// 基础价格
	price := fm.config.BaseGasPrice

	// 应用拥堵倍数
	multiplier := fm.GetFeeMultiplier()
	price = price.Mul(multiplier)

	// 应用调整系数
	price = price.Mul(fm.config.GasAdjustment)

	// 限制范围
	if price.LessThan(fm.config.MinGasPrice) {
		price = fm.config.MinGasPrice
	}
	if price.GreaterThan(fm.config.MaxGasPrice) {
		price = fm.config.MaxGasPrice
	}

	return price
}

// CalculateFee 计算交易费用
func (fm *FeeManager) CalculateFee(gasUsed uint64) decimal.Decimal {
	gasPrice := fm.GetCurrentGasPrice()
	return gasPrice.Mul(decimal.NewFromInt(int64(gasUsed)))
}

// GetGasPriceForPriority 根据优先级获取 Gas 价格
func (fm *FeeManager) GetGasPriceForPriority(priority string) decimal.Decimal {
	basePrice := fm.GetCurrentGasPrice()

	switch priority {
	case "slow":
		return basePrice.Mul(decimal.NewFromFloat(0.8))
	case "normal":
		return basePrice
	case "fast":
		return basePrice.Mul(decimal.NewFromFloat(1.5))
	case "instant":
		return basePrice.Mul(decimal.NewFromFloat(2.0))
	default:
		return basePrice
	}
}

// GetConfig 获取配置
func (fm *FeeManager) GetConfig() *DynamicFeeConfig {
	return fm.config
}

// SetConfig 设置配置
func (fm *FeeManager) SetConfig(config *DynamicFeeConfig) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.config = config
}

// GetNetworkMetrics 获取网络指标
func (fm *FeeManager) GetNetworkMetrics() NetworkMetrics {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return *fm.metrics
}
