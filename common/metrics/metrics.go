package metrics

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ==================== 指标类型 ====================

// Counter 计数器
type Counter struct {
	value uint64
}

// Inc 增加计数
func (c *Counter) Inc() {
	atomic.AddUint64(&c.value, 1)
}

// Add 增加指定值
func (c *Counter) Add(n uint64) {
	atomic.AddUint64(&c.value, n)
}

// Value 获取当前值
func (c *Counter) Value() uint64 {
	return atomic.LoadUint64(&c.value)
}

// Reset 重置
func (c *Counter) Reset() {
	atomic.StoreUint64(&c.value, 0)
}

// Gauge 仪表盘（当前值）
type Gauge struct {
	value uint64
}

// Set 设置值
func (g *Gauge) Set(v uint64) {
	atomic.StoreUint64(&g.value, v)
}

// Add 增加值
func (g *Gauge) Add(v uint64) {
	atomic.AddUint64(&g.value, v)
}

// Sub 减少值
func (g *Gauge) Sub(v uint64) {
	atomic.AddUint64(&g.value, ^uint64(v-1))
}

// Value 获取当前值
func (g *Gauge) Value() uint64 {
	return atomic.LoadUint64(&g.value)
}

// Histogram 直方图
type Histogram struct {
	counts   map[uint64]uint64
	sum      uint64
	min      uint64
	max      uint64
	count    uint64
	mu       sync.Mutex
	buckets  []uint64
}

// NewHistogram 创建直方图
func NewHistogram(buckets []uint64) *Histogram {
	if buckets == nil {
		buckets = []uint64{1, 10, 50, 100, 500, 1000, 5000, 10000}
	}
	return &Histogram{
		counts:  make(map[uint64]uint64),
		sum:     0,
		min:     ^uint64(0),
		max:     0,
		buckets: buckets,
	}
}

// Observe 记录观察值
func (h *Histogram) Observe(v uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count++
	h.sum += v
	if v < h.min {
		h.min = v
	}
	if v > h.max {
		h.max = v
	}

	// 记录到 bucket
	for _, b := range h.buckets {
		if v <= b {
			h.counts[b]++
			break
		}
	}
}

// Count 获取样本数
func (h *Histogram) Count() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

// Sum 获取总和
func (h *Histogram) Sum() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sum
}

// Min 获取最小值
func (h *Histogram) Min() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count == 0 {
		return 0
	}
	return h.min
}

// Max 获取最大值
func (h *Histogram) Max() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.max
}

// Mean 获取平均值
func (h *Histogram) Mean() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count == 0 {
		return 0
	}
	return float64(h.sum) / float64(h.count)
}

// Timer 计时器
type Timer struct {
	histogram *Histogram
}

// NewTimer 创建计时器
func NewTimer() *Timer {
	return &Timer{
		histogram: NewHistogram(nil),
	}
}

// Time 时间操作
func (t *Timer) Time(fn func()) {
	start := time.Now()
	fn()
	t.histogram.Observe(uint64(time.Since(start).Milliseconds()))
}

// Observe 记录观察值（毫秒）
func (t *Timer) Observe(ms uint64) {
	t.histogram.Observe(ms)
}

// Histogram 获取直方图
func (t *Timer) Histogram() *Histogram {
	return t.histogram
}

// ==================== 系统指标 ====================

// SystemMetrics 系统指标
type SystemMetrics struct {
	// 区块链指标
	BlockHeight    *Gauge    // 当前区块高度
	BlockTimeMs    *Histogram // 区块时间（毫秒）
	TxPoolSize    *Gauge    // 交易池大小
	TxCount       *Counter  // 总交易数

	// 网络指标
	PeerCount      *Gauge    // 节点数量
	BytesIn        *Counter  // 接收字节
	BytesOut       *Counter  // 发送字节
	LatencyMs      *Histogram // 网络延迟（毫秒）

	// 共识指标
	ProposalCount  *Counter  // 提案数
	VoteCount      *Counter  // 投票数
	SlashCount     *Counter  // 惩罚次数

	// 合约指标
	StakeTotal     *Gauge    // 总质押
	RewardTotal    *Counter  // 总奖励
	TreasuryBalance *Gauge  // 国库余额

	// 性能指标
	CPUPercent     *Gauge    // CPU 使用率
	MemoryMB       *Gauge    // 内存使用 MB
	DiskMB         *Gauge    // 磁盘使用 MB

	// 错误指标
	ErrorCount     *Counter  // 错误数
}

// NewSystemMetrics 创建系统指标
func NewSystemMetrics() *SystemMetrics {
	return &SystemMetrics{
		BlockHeight:   NewGauge(),
		BlockTimeMs:    NewHistogram([]uint64{100, 500, 1000, 2000, 5000}),
		TxPoolSize:    NewGauge(),
		TxCount:       NewCounter(),

		PeerCount:     NewGauge(),
		BytesIn:       NewCounter(),
		BytesOut:      NewCounter(),
		LatencyMs:     NewHistogram([]uint64{50, 100, 200, 500, 1000}),

		ProposalCount: NewCounter(),
		VoteCount:     NewCounter(),
		SlashCount:    NewCounter(),

		StakeTotal:    NewGauge(),
		RewardTotal:   NewCounter(),
		TreasuryBalance: NewGauge(),

		CPUPercent:    NewGauge(),
		MemoryMB:      NewGauge(),
		DiskMB:        NewGauge(),

		ErrorCount:    NewCounter(),
	}
}

// NewGauge 创建仪表盘
func NewGauge() *Gauge {
	return &Gauge{}
}

// NewCounter 创建计数器
func NewCounter() *Counter {
	return &Counter{}
}

// ==================== 指标注册表 ====================

// Registry 指标注册表
type Registry struct {
	metrics map[string]interface{}
	mu      sync.RWMutex
}

// 全局注册表
var globalRegistry = &Registry{
	metrics: make(map[string]interface{}),
}

// Register 注册指标
func (r *Registry) Register(name string, metric interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics[name] = metric
}

// Get 获取指标
func (r *Registry) Get(name string) interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.metrics[name]
}

// GetCounter 获取计数器
func (r *Registry) GetCounter(name string) *Counter {
	if c, ok := r.Get(name).(*Counter); ok {
		return c
	}
	return nil
}

// GetGauge 获取仪表盘
func (r *Registry) GetGauge(name string) *Gauge {
	if g, ok := r.Get(name).(*Gauge); ok {
		return g
	}
	return nil
}

// GetHistogram 获取直方图
func (r *Registry) GetHistogram(name string) *Histogram {
	if h, ok := r.Get(name).(*Histogram); ok {
		return h
	}
	return nil
}

// All 获取所有指标
func (r *Registry) All() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]interface{}, len(r.metrics))
	for k, v := range r.metrics {
		result[k] = v
	}
	return result
}

// ==================== 全局便捷方法 ====================

// Register 注册全局指标
func Register(name string, metric interface{}) {
	globalRegistry.Register(name, metric)
}

// GetCounter 获取全局计数器
func GetCounter(name string) *Counter {
	return globalRegistry.GetCounter(name)
}

// GetGauge 获取全局仪表盘
func GetGauge(name string) *Gauge {
	return globalRegistry.GetGauge(name)
}

// ==================== 辅助函数 ====================

// formatMetric 格式化指标输出
func formatMetric(name string, value interface{}) string {
	switch v := value.(type) {
	case *Counter:
		return fmt.Sprintf("%s: %d", name, v.Value())
	case *Gauge:
		return fmt.Sprintf("%s: %d", name, v.Value())
	case *Histogram:
		return fmt.Sprintf("%s: count=%d mean=%.2f min=%d max=%d",
			name, v.Count(), v.Mean(), v.Min(), v.Max())
	default:
		return fmt.Sprintf("%s: %v", name, v)
	}
}

// String 实现 Stringer 接口
func (sm *SystemMetrics) String() string {
	return fmt.Sprintf(`SystemMetrics:
  Blockchain:
    BlockHeight: %d
    TxCount: %d
  Network:
    PeerCount: %d
    BytesIn: %d
    BytesOut: %d
  Consensus:
    ProposalCount: %d
    VoteCount: %d
    SlashCount: %d
  Errors:
    ErrorCount: %d`,
		sm.BlockHeight.Value(),
		sm.TxCount.Value(),
		sm.PeerCount.Value(),
		sm.BytesIn.Value(),
		sm.BytesOut.Value(),
		sm.ProposalCount.Value(),
		sm.VoteCount.Value(),
		sm.SlashCount.Value(),
		sm.ErrorCount.Value(),
	)
}

