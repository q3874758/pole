package monitor

import (
	"fmt"
	"sync"
	"time"
)

// ==================== 告警配置 ====================

// AlertLevel 告警级别
type AlertLevel string

const (
	AlertInfo    AlertLevel = "info"
	AlertWarning AlertLevel = "warning"
	AlertError   AlertLevel = "error"
	AlertCritical AlertLevel = "critical"
)

// AlertRule 告警规则
type AlertRule struct {
	Name      string        // 规则名称
	Condition func(*SystemState) bool // 触发条件
	Level     AlertLevel   // 告警级别
	Message   string       // 告警消息
	Cooldown  time.Duration // 冷却时间
}

// ==================== 系统状态 ====================

// SystemState 系统状态
type SystemState struct {
	BlockHeight    uint64        // 当前区块高度
	BlockTimeMs    uint64        // 上个区块时间（毫秒）
	PeerCount      int           // 节点数量
	TxPoolSize    int           // 交易池大小
	RPCLatencyMs  uint64        // RPC 延迟
	CPUPercent    uint64        // CPU 使用率
	MemoryMB      uint64        // 内存使用 MB
	LastBlockTime int64         // 上个区块时间戳
	Errors        uint64        // 错误计数
}

// NewSystemState 创建系统状态
func NewSystemState() *SystemState {
	return &SystemState{
		BlockHeight:  0,
		BlockTimeMs:  0,
		PeerCount:    0,
		TxPoolSize:  0,
		RPCLatencyMs: 0,
		CPUPercent:   0,
		MemoryMB:     0,
		LastBlockTime: time.Now().Unix(),
		Errors:       0,
	}
}

// ==================== 告警管理器 ====================

// Alert 告警
type Alert struct {
	ID        string     `json:"id"`
	RuleName  string     `json:"rule_name"`
	Level    AlertLevel `json:"level"`
	Message  string     `json:"message"`
	Time     int64      `json:"time"`
	Resolved bool       `json:"resolved"`
}

// AlertManager 告警管理器
type AlertManager struct {
	rules     []*AlertRule
	alerts    map[string]*Alert
	mu        sync.RWMutex
	handlers  []AlertHandler
	cooldowns map[string]time.Time
}

// AlertHandler 告警处理器
type AlertHandler interface {
	Handle(alert *Alert)
}

// NewAlertManager 创建告警管理器
func NewAlertManager() *AlertManager {
	return &AlertManager{
		rules:     make([]*AlertRule, 0),
		alerts:    make(map[string]*Alert),
		handlers:  make([]AlertHandler, 0),
		cooldowns: make(map[string]time.Time),
	}
}

// RegisterRule 注册告警规则
func (am *AlertManager) RegisterRule(rule *AlertRule) {
	am.rules = append(am.rules, rule)
}

// RegisterHandler 注册告警处理器
func (am *AlertManager) RegisterHandler(handler AlertHandler) {
	am.handlers = append(am.handlers, handler)
}

// Check 检查告警规则
func (am *AlertManager) Check(state *SystemState) {
	am.mu.Lock()
	defer am.mu.Unlock()

	for _, rule := range am.rules {
		// 检查冷却时间
		if lastTime, ok := am.cooldowns[rule.Name]; ok {
			if time.Since(lastTime) < rule.Cooldown {
				continue
			}
		}

		// 检查条件
		if rule.Condition(state) {
			alert := &Alert{
				ID:       fmt.Sprintf("%s-%d", rule.Name, time.Now().Unix()),
				RuleName: rule.Name,
				Level:    rule.Level,
				Message:  rule.Message,
				Time:     time.Now().Unix(),
			}

			// 避免重复告警
			if existing, ok := am.alerts[rule.Name]; !ok || existing.Resolved {
				am.alerts[rule.Name] = alert
				am.cooldowns[rule.Name] = time.Now()

				// 通知处理器
				for _, h := range am.handlers {
					h.Handle(alert)
				}
			}
		}
	}
}

// Resolve 解除告警
func (am *AlertManager) Resolve(ruleName string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if alert, ok := am.alerts[ruleName]; ok {
		alert.Resolved = true
	}
}

// GetActiveAlerts 获取活跃告警
func (am *AlertManager) GetActiveAlerts() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make([]*Alert, 0)
	for _, alert := range am.alerts {
		if !alert.Resolved {
			result = append(result, alert)
		}
	}
	return result
}

// ==================== 默认告警规则 ====================

// DefaultAlertRules 创建默认告警规则
func DefaultAlertRules() []*AlertRule {
	return []*AlertRule{
		{
			Name: "block_stalled",
			Condition: func(s *SystemState) bool {
				if s.BlockHeight == 0 {
					return false
				}
				// 超过 60 秒没有新块
				return time.Now().Unix()-s.LastBlockTime > 60
			},
			Level:    AlertCritical,
			Message:  "Block production stalled - no new blocks for 60 seconds",
			Cooldown: 30 * time.Second,
		},
		{
			Name: "block_time_high",
			Condition: func(s *SystemState) bool {
				// 出块时间超过 15 秒
				return s.BlockTimeMs > 15000
			},
			Level:    AlertWarning,
			Message:  "Block time too high",
			Cooldown: 60 * time.Second,
		},
		{
			Name: "no_peers",
			Condition: func(s *SystemState) bool {
				return s.PeerCount == 0
			},
			Level:    AlertError,
			Message:  "No connected peers",
			Cooldown: 30 * time.Second,
		},
		{
			Name: "low_peers",
			Condition: func(s *SystemState) bool {
				return s.PeerCount > 0 && s.PeerCount < 3
			},
			Level:    AlertWarning,
			Message:  "Low peer count",
			Cooldown: 120 * time.Second,
		},
		{
			Name: "tx_pool_full",
			Condition: func(s *SystemState) bool {
				return s.TxPoolSize > 10000
			},
			Level:    AlertWarning,
			Message:  "Transaction pool nearly full",
			Cooldown: 60 * time.Second,
		},
		{
			Name: "rpc_unavailable",
			Condition: func(s *SystemState) bool {
				// RPC 延迟过高
				return s.RPCLatencyMs > 5000
			},
			Level:    AlertError,
			Message:  "RPC latency too high",
			Cooldown: 30 * time.Second,
		},
		{
			Name: "high_cpu",
			Condition: func(s *SystemState) bool {
				return s.CPUPercent > 90
			},
			Level:    AlertWarning,
			Message:  "High CPU usage",
			Cooldown: 60 * time.Second,
		},
		{
			Name: "high_memory",
			Condition: func(s *SystemState) bool {
				return s.MemoryMB > 8000
			},
			Level:    AlertWarning,
			Message:  "High memory usage",
			Cooldown: 60 * time.Second,
		},
		{
			Name: "error_spike",
			Condition: func(s *SystemState) bool {
				// 错误数量突增（简化判断）
				return s.Errors > 100
			},
			Level:    AlertError,
			Message:  "Error count spike detected",
			Cooldown: 30 * time.Second,
		},
	}
}

// ==================== 告警处理器 ====================

// LogAlertHandler 日志告警处理器
type LogAlertHandler struct{}

func NewLogAlertHandler() *LogAlertHandler {
	return &LogAlertHandler{}
}

func (h *LogAlertHandler) Handle(alert *Alert) {
	prefix := "[ALERT]"
	switch alert.Level {
	case AlertCritical:
		prefix = "[CRITICAL]"
	case AlertError:
		prefix = "[ERROR]"
	case AlertWarning:
		prefix = "[WARN]"
	}
	fmt.Printf("%s %s: %s\n", prefix, alert.RuleName, alert.Message)
}
