package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"pole-core/common/metrics"
)

// ==================== Prometheus 指标导出 ====================

// PrometheusExporter Prometheus 导出器
type PrometheusExporter struct {
	systemMetrics *metrics.SystemMetrics
	httpSrv       *http.Server
	config        *PrometheusConfig
	mu             sync.RWMutex
}

// PrometheusConfig Prometheus 配置
type PrometheusConfig struct {
	Port     string        // 监听端口
	Path     string        // 指标路径
	Interval time.Duration // 收集间隔
}

func DefaultPrometheusConfig() *PrometheusConfig {
	return &PrometheusConfig{
		Port:     ":9091",
		Path:     "/metrics",
		Interval: 10 * time.Second,
	}
}

// NewPrometheusExporter 创建 Prometheus 导出器
func NewPrometheusExporter(sysMetrics *metrics.SystemMetrics, cfg *PrometheusConfig) *PrometheusExporter {
	if cfg == nil {
		cfg = DefaultPrometheusConfig()
	}
	if sysMetrics == nil {
		sysMetrics = metrics.NewSystemMetrics()
	}
	return &PrometheusExporter{
		systemMetrics: sysMetrics,
		config:        cfg,
	}
}

// Start 启动 Prometheus HTTP 服务器
func (p *PrometheusExporter) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc(p.config.Path, p.handleMetrics)
	mux.HandleFunc("/health", p.handleHealth)

	p.httpSrv = &http.Server{
		Addr:    p.config.Port,
		Handler: mux,
	}

	go func() {
		if err := p.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[Monitor] Prometheus 服务器错误: %v\n", err)
		}
	}()

	fmt.Printf("[Monitor] Prometheus 已启动: http://localhost%s%s\n", p.config.Port, p.config.Path)
	return nil
}

// Stop 停止
func (p *PrometheusExporter) Stop() error {
	if p.httpSrv != nil {
		return p.httpSrv.Close()
	}
	return nil
}

// handleMetrics 处理指标请求
func (p *PrometheusExporter) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// 收集并格式化指标
	output := p.collectMetrics()
	w.Write([]byte(output))
}

// collectMetrics 收集所有指标
func (p *PrometheusExporter) collectMetrics() string {
	// 区块链指标
	output := `# TYPE pole_block_height gauge
# HELP Current block height
pole_block_height %d

# TYPE pole_tx_count counter
# HELP Total transaction count
pole_tx_count %d

# TYPE pole_tx_pool_size gauge
# HELP Transaction pool size
pole_tx_pool_size %d

# TYPE pole_peer_count gauge
# HELP Number of connected peers
pole_peer_count %d

# TYPE pole_stake_total gauge
# HELP Total staked amount
pole_stake_total %d

# TYPE pole_treasury_balance gauge
# HELP Treasury balance
pole_treasury_balance %d

# TYPE pole_error_count counter
# HELP Total error count
pole_error_count %d

# TYPE pole_uptime_seconds gauge
# HELP Node uptime in seconds
pole_uptime_seconds %d
`
	// 获取系统指标
	sm := metrics.NewSystemMetrics()

	return fmt.Sprintf(output,
		sm.BlockHeight.Value(),
		sm.TxCount.Value(),
		sm.TxPoolSize.Value(),
		sm.PeerCount.Value(),
		sm.StakeTotal.Value(),
		sm.TreasuryBalance.Value(),
		sm.ErrorCount.Value(),
		getUptimeSeconds(),
	)
}

// handleHealth 健康检查
func (p *PrometheusExporter) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := p.CheckHealth()

	w.Header().Set("Content-Type", "application/json")
	if health.Healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(health)
}

// Health 健康状态
type Health struct {
	Healthy    bool      `json:"healthy"`
	Status     string    `json:"status"`
	Timestamp  int64     `json:"timestamp"`
	Components ComponentHealth `json:"components"`
}

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Blockchain bool `json:"blockchain"`
	Consensus  bool `json:"consensus"`
	RPC        bool `json:"rpc"`
	P2P        bool `json:"p2p"`
}

// CheckHealth 检查健康状态
func (p *PrometheusExporter) CheckHealth() *Health {
	health := &Health{
		Healthy:   true,
		Status:    "healthy",
		Timestamp: time.Now().Unix(),
		Components: ComponentHealth{
			Blockchain: true,
			Consensus:  true,
			RPC:        true,
			P2P:        true,
		},
	}

	// 检查区块链
	sm := metrics.NewSystemMetrics()
	if sm.BlockHeight.Value() == 0 {
		health.Components.Blockchain = false
		health.Healthy = false
		health.Status = "blockchain not ready"
	}

	// TODO: 检查其他组件

	if health.Healthy {
		health.Status = "all systems operational"
	}

	return health
}

// ==================== 启动时间 ====================

var startTime = time.Now()

func getUptimeSeconds() int {
	return int(time.Since(startTime).Seconds())
}
