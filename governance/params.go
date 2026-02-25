package governance

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// ==================== 治理参数（白皮书 5.3）====================

// ParamKey 参数键
type ParamKey string

const (
	// 经济模型参数
	ParamInflationRate       ParamKey = "InflationRate"       // 年通胀率 0-30%
	ParamDecayFactor         ParamKey = "DecayFactor"         // 衰减因子 0.3-0.7
	ParamTxBurnRatio         ParamKey = "TxBurnRatio"         // 交易燃烧比例 20-30%
	ParamRewardBurnThreshold ParamKey = "RewardBurnThreshold" // 奖励燃烧阈值 5000-50000
	ParamRewardBurnRatio     ParamKey = "RewardBurnRatio"     // 奖励燃烧比例 5-20%
	ParamGovBurnRatio        ParamKey = "GovBurnRatio"        // 治理燃烧比例 0.5-2%

	// 网络运营参数
	ParamValidatorMinStake   ParamKey = "ValidatorMinStake"   // 验证节点质押门槛
	ParamMaxValidators       ParamKey = "MaxValidators"       // 最大验证节点数 21-100
	ParamEpochLength         ParamKey = "EpochLength"         // 纪元长度 1000-50000
	ParamUnbondingPeriodDays ParamKey = "UnbondingPeriodDays" // 解绑周期 7-28 天
	ParamDataDeviationBps    ParamKey = "DataDeviationBps"    // 数据偏差阈值 1000-5000 (10%-50%)

	// Slash 惩罚参数
	ParamSlashDoubleSign    ParamKey = "SlashDoubleSign"    // 双签 5-100%
	ParamSlashOffline       ParamKey = "SlashOffline"       // 离线 1-10%
	ParamSlashDeviationMid  ParamKey = "SlashDeviationMid"  // 偏差 20-50% 时 5-20%
	ParamSlashDeviationHigh ParamKey = "SlashDeviationHigh" // 偏差 >50% 时 10-50%

	// 动态费用参数
	ParamBaseGasPrice   ParamKey = "BaseGasPrice"   // 0.00001-0.001
	ParamMaxGasPrice    ParamKey = "MaxGasPrice"    // 0.1-10
	ParamGasAdjustment  ParamKey = "GasAdjustment"  // 1.0-2.0
	ParamCongestionBps  ParamKey = "CongestionBps" // 拥堵阈值 3000-7000 (30%-70%)
)

// ParamRange 参数可调范围
type ParamRange struct {
	Min, Max decimal.Decimal
}

// GovernanceParams 治理参数存储
type GovernanceParams struct {
	mu     sync.RWMutex
	values map[ParamKey]decimal.Decimal
	ranges map[ParamKey]ParamRange
}

// DefaultGovernanceParams 白皮书默认值与可调范围
func DefaultGovernanceParams() *GovernanceParams {
	gp := &GovernanceParams{
		values: make(map[ParamKey]decimal.Decimal),
		ranges: make(map[ParamKey]ParamRange),
	}
	// 经济
	gp.set(ParamInflationRate, "0.2", "0", "0.3")
	gp.set(ParamDecayFactor, "0.5", "0.3", "0.7")
	gp.set(ParamTxBurnRatio, "0.25", "0.2", "0.3")
	gp.set(ParamRewardBurnThreshold, "10000", "5000", "50000")
	gp.set(ParamRewardBurnRatio, "0.1", "0.05", "0.2")
	gp.set(ParamGovBurnRatio, "0.01", "0.005", "0.02")
	// 网络
	gp.set(ParamValidatorMinStake, "10000", "1000", "100000")
	gp.set(ParamMaxValidators, "21", "21", "100")
	gp.set(ParamEpochLength, "14400", "1000", "50000")
	gp.set(ParamUnbondingPeriodDays, "21", "7", "28")
	gp.set(ParamDataDeviationBps, "2000", "1000", "5000") // 20% = 2000 bps
	// Slash
	gp.set(ParamSlashDoubleSign, "0.05", "0.05", "1")
	gp.set(ParamSlashOffline, "0.01", "0.01", "0.1")
	gp.set(ParamSlashDeviationMid, "0.05", "0.05", "0.2")
	gp.set(ParamSlashDeviationHigh, "0.2", "0.1", "0.5")
	// Gas
	gp.set(ParamBaseGasPrice, "0.0001", "0.00001", "0.001")
	gp.set(ParamMaxGasPrice, "1", "0.1", "10")
	gp.set(ParamGasAdjustment, "1.15", "1", "2")
	gp.set(ParamCongestionBps, "5000", "3000", "7000")
	return gp
}

func (gp *GovernanceParams) set(key ParamKey, defaultVal, min, max string) {
	v, _ := decimal.NewFromString(defaultVal)
	mn, _ := decimal.NewFromString(min)
	mx, _ := decimal.NewFromString(max)
	gp.values[key] = v
	gp.ranges[key] = ParamRange{Min: mn, Max: mx}
}

// Get 获取参数值
func (gp *GovernanceParams) Get(key ParamKey) (decimal.Decimal, bool) {
	gp.mu.RLock()
	defer gp.mu.RUnlock()
	v, ok := gp.values[key]
	return v, ok
}

// GetRange 获取可调范围
func (gp *GovernanceParams) GetRange(key ParamKey) (ParamRange, bool) {
	gp.mu.RLock()
	defer gp.mu.RUnlock()
	r, ok := gp.ranges[key]
	return r, ok
}

// Set 通过治理设置参数（需在范围内）
func (gp *GovernanceParams) Set(key ParamKey, value decimal.Decimal) error {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	r, ok := gp.ranges[key]
	if !ok {
		return fmt.Errorf("unknown param: %s", key)
	}
	if value.LessThan(r.Min) || value.GreaterThan(r.Max) {
		return fmt.Errorf("param %s out of range [%s, %s]", key, r.Min, r.Max)
	}
	gp.values[key] = value
	return nil
}

// InflationRate 年通胀率
func (gp *GovernanceParams) InflationRate() decimal.Decimal { v, _ := gp.Get(ParamInflationRate); return v }

// DecayFactor 衰减因子
func (gp *GovernanceParams) DecayFactor() decimal.Decimal { v, _ := gp.Get(ParamDecayFactor); return v }

// TxBurnRatio 交易燃烧比例
func (gp *GovernanceParams) TxBurnRatio() decimal.Decimal { v, _ := gp.Get(ParamTxBurnRatio); return v }

// RewardBurnThreshold 奖励燃烧阈值
func (gp *GovernanceParams) RewardBurnThreshold() decimal.Decimal {
	v, _ := gp.Get(ParamRewardBurnThreshold)
	return v
}

// RewardBurnRatio 奖励燃烧比例
func (gp *GovernanceParams) RewardBurnRatio() decimal.Decimal { v, _ := gp.Get(ParamRewardBurnRatio); return v }

// GovBurnRatio 治理燃烧比例
func (gp *GovernanceParams) GovBurnRatio() decimal.Decimal { v, _ := gp.Get(ParamGovBurnRatio); return v }

// ValidatorMinStake 验证节点最低质押
func (gp *GovernanceParams) ValidatorMinStake() decimal.Decimal {
	v, _ := gp.Get(ParamValidatorMinStake)
	return v
}

// MaxValidators 最大验证节点数
func (gp *GovernanceParams) MaxValidators() int {
	v, _ := gp.Get(ParamMaxValidators)
	return int(v.IntPart())
}

// EpochLength 纪元长度
func (gp *GovernanceParams) EpochLength() int64 {
	v, _ := gp.Get(ParamEpochLength)
	return v.IntPart()
}

// UnbondingPeriod 解绑周期
func (gp *GovernanceParams) UnbondingPeriod() time.Duration {
	v, _ := gp.Get(ParamUnbondingPeriodDays)
	days := v.IntPart()
	return time.Duration(days) * 24 * time.Hour
}

// DataDeviationBps 数据偏差阈值 (bps)
func (gp *GovernanceParams) DataDeviationBps() int {
	v, _ := gp.Get(ParamDataDeviationBps)
	return int(v.IntPart())
}

// SlashDoubleSign 双签惩罚比例
func (gp *GovernanceParams) SlashDoubleSign() decimal.Decimal {
	v, _ := gp.Get(ParamSlashDoubleSign)
	return v
}

// SlashOffline 离线惩罚比例
func (gp *GovernanceParams) SlashOffline() decimal.Decimal {
	v, _ := gp.Get(ParamSlashOffline)
	return v
}

// SlashDeviationMid 偏差 20-50% 惩罚
func (gp *GovernanceParams) SlashDeviationMid() decimal.Decimal {
	v, _ := gp.Get(ParamSlashDeviationMid)
	return v
}

// SlashDeviationHigh 偏差 >50% 惩罚
func (gp *GovernanceParams) SlashDeviationHigh() decimal.Decimal {
	v, _ := gp.Get(ParamSlashDeviationHigh)
	return v
}

// BaseGasPrice 基础 Gas 价格
func (gp *GovernanceParams) BaseGasPrice() decimal.Decimal {
	v, _ := gp.Get(ParamBaseGasPrice)
	return v
}

// MaxGasPrice 最大 Gas 价格
func (gp *GovernanceParams) MaxGasPrice() decimal.Decimal {
	v, _ := gp.Get(ParamMaxGasPrice)
	return v
}

// GasAdjustment Gas 调整系数
func (gp *GovernanceParams) GasAdjustment() decimal.Decimal {
	v, _ := gp.Get(ParamGasAdjustment)
	return v
}

// CongestionBps 拥堵阈值 (bps)
func (gp *GovernanceParams) CongestionBps() int {
	v, _ := gp.Get(ParamCongestionBps)
	return int(v.IntPart())
}
