package discovery

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== 服务发现配置 ====================

// Config 服务发现配置
type Config struct {
	HeartbeatInterval time.Duration // 心跳间隔
	HeartbeatTimeout  time.Duration // 心跳超时
	CleanupInterval   time.Duration // 清理间隔
	MaxServicesPerNode int          // 每节点最大服务数
	EnableLoadBalance bool          // 启用负载均衡
	HealthCheckInterval time.Duration // 健康检查间隔
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		HeartbeatInterval:  10 * time.Second,
		HeartbeatTimeout:   30 * time.Second,
		CleanupInterval:    60 * time.Second,
		MaxServicesPerNode: 10,
		EnableLoadBalance:  true,
		HealthCheckInterval: 15 * time.Second,
	}
}

// ==================== 服务 ====================

// Service 服务
type Service struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	Metadata    map[string]string `json:"metadata"`
	Protocol    string            `json:"protocol"`
	Weight      int               // 负载均衡权重
	Healthy     bool              // 健康状态
	LastHeartbeat int64           // 最后心跳时间
}

// NewService 创建新服务
func NewService(name, address string, port int) *Service {
	return &Service{
		Name:        name,
		Address:     address,
		Port:        port,
		Metadata:    make(map[string]string),
		Weight:      100,
		Healthy:    true,
		LastHeartbeat: time.Now().Unix(),
	}
}

// Endpoint 获取端点
func (s *Service) Endpoint() string {
	return fmt.Sprintf("%s:%d", s.Address, s.Port)
}

// ==================== 节点 ====================

// Node 节点
type Node struct {
	ID           types.NodeID     `json:"id"`
	Address      types.Address   `json:"address"`
	Services    []*Service      `json:"services"`
	Status      NodeStatus       `json:"status"`
	Reputation  float64         `json:"reputation"`
	Stake       types.TokenAmount `json:"stake"`
	LastSeen   int64            `json:"last_seen"`
}

// NodeStatus 节点状态
type NodeStatus int

const (
	NodeStatusUnknown NodeStatus = iota
	NodeStatusOnline
	NodeStatusOffline
	NodeStatusSuspected
)

// ==================== 服务注册表 ====================

// Registry 服务注册表
type Registry struct {
	config   *Config
	services map[string]map[string]*Service // name -> address -> service
	nodes    map[types.NodeID]*Node
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewRegistry 创建服务注册表
func NewRegistry(config *Config) *Registry {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Registry{
		config:   config,
		services: make(map[string]map[string]*Service),
		nodes:    make(map[types.NodeID]*Node),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start 启动注册表
func (r *Registry) Start() {
	go r.cleanupLoop()
	go r.healthCheckLoop()
}

// Stop 停止注册表
func (r *Registry) Stop() {
	r.cancel()
}

// Register 注册服务
func (r *Registry) Register(service *Service) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查服务数量限制
	if len(r.services[service.Name]) >= r.config.MaxServicesPerNode {
		return fmt.Errorf("max services reached for: %s", service.Name)
	}

	// 检查服务是否已存在
	if existing, ok := r.services[service.Name][service.Endpoint()]; ok {
		// 更新已存在的服务
		*existing = *service
		return nil
	}

	// 添加新服务
	if r.services[service.Name] == nil {
		r.services[service.Name] = make(map[string]*Service)
	}
	r.services[service.Name][service.Endpoint()] = service

	return nil
}

// Unregister 注销服务
func (r *Registry) Unregister(name, endpoint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if services, ok := r.services[name]; ok {
		delete(services, endpoint)
		if len(services) == 0 {
			delete(r.services, name)
		}
	}

	return nil
}

// Discover 发现服务
func (r *Registry) Discover(name string) ([]*Service, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	services, ok := r.services[name]
	if !ok {
		return nil, fmt.Errorf("service not found: %s", name)
	}

	result := make([]*Service, 0, len(services))
	for _, service := range services {
		if service.Healthy {
			result = append(result, service)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no healthy services: %s", name)
	}

	return result, nil
}

// DiscoverOne 发现单个服务 (带负载均衡)
func (r *Registry) DiscoverOne(name string) (*Service, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	services, ok := r.services[name]
	if !ok {
		return nil, fmt.Errorf("service not found: %s", name)
	}

	// 过滤健康服务
	var healthyServices []*Service
	for _, service := range services {
		if service.Healthy {
			healthyServices = append(healthyServices, service)
		}
	}

	if len(healthyServices) == 0 {
		return nil, fmt.Errorf("no healthy services: %s", name)
	}

	// 负载均衡选择
	if r.config.EnableLoadBalance {
		return r.selectByWeight(healthyServices), nil
	}

	// 随机选择
	return healthyServices[rand.Intn(len(healthyServices))], nil
}

// selectByWeight 按权重选择
func (r *Registry) selectByWeight(services []*Service) *Service {
	var totalWeight int
	for _, s := range services {
		totalWeight += s.Weight
	}

	if totalWeight == 0 {
		return services[rand.Intn(len(services))]
	}

	random := rand.Intn(totalWeight)
	for _, s := range services {
		random -= s.Weight
		if random < 0 {
			return s
		}
	}

	return services[0]
}

// Heartbeat 接收心跳
func (r *Registry) Heartbeat(service *Service) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if services, ok := r.services[service.Name]; ok {
		if s, ok := services[service.Endpoint()]; ok {
			s.LastHeartbeat = time.Now().Unix()
			s.Healthy = true
		}
	}
}

// GetAllServices 获取所有服务
func (r *Registry) GetAllServices() map[string][]*Service {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]*Service)
	for name, services := range r.services {
		result[name] = make([]*Service, 0, len(services))
		for _, service := range services {
			result[name] = append(result[name], service)
		}
	}

	return result
}

// GetServicesByName 按名称获取服务
func (r *Registry) GetServicesByName(name string) []*Service {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if services, ok := r.services[name]; ok {
		result := make([]*Service, 0, len(services))
		for _, service := range services {
			result = append(result, service)
		}
		return result
	}

	return nil
}

// cleanupLoop 清理过期服务
func (r *Registry) cleanupLoop() {
	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.cleanup()
		}
	}
}

// cleanup 清理过期服务
func (r *Registry) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	timeout := time.Now().Add(-r.config.HeartbeatTimeout).Unix()
	removed := 0

	for name, services := range r.services {
		for endpoint, service := range services {
			if service.LastHeartbeat < timeout {
				delete(services, endpoint)
				removed++
			}
		}
		if len(services) == 0 {
			delete(r.services, name)
		}
	}

	if removed > 0 {
		fmt.Printf("discovery: cleaned up %d expired services\n", removed)
	}
}

// healthCheckLoop 健康检查循环
func (r *Registry) healthCheckLoop() {
	ticker := time.NewTicker(r.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.healthCheck()
		}
	}
}

// healthCheck 健康检查
func (r *Registry) healthCheck() {
	r.mu.Lock()
	defer r.mu.Unlock()

	timeout := time.Now().Add(-r.config.HeartbeatTimeout).Unix()

	for _, services := range r.services {
		for _, service := range services {
			if service.LastHeartbeat < timeout {
				service.Healthy = false
			}
		}
	}
}

// ==================== 服务发现客户端 ====================

// Client 服务发现客户端
type Client struct {
	registry *Registry
	mu       sync.RWMutex
	cache    map[string][]*Service
	cacheTTL time.Duration
}

// NewClient 创建服务发现客户端
func NewClient(registry *Registry) *Client {
	return &Client{
		registry: registry,
		cache:    make(map[string][]*Service),
		cacheTTL: 30 * time.Second,
	}
}

// FindService 查找服务
func (c *Client) FindService(name string) (*Service, error) {
	// 尝试从缓存获取
	if services, ok := c.getCached(name); len(services) > 0 {
		return c.registry.DiscoverOne(name)
	}

	// 从注册表获取
	return c.registry.DiscoverOne(name)
}

// FindServices 查找多个服务
func (c *Client) FindServices(name string) ([]*Service, error) {
	// 尝试从缓存获取
	if services, ok := c.getCached(name); len(services) > 0 {
		return services, nil
	}

	// 从注册表获取
	services, err := c.registry.Discover(name)
	if err != nil {
		return nil, err
	}

	// 更新缓存
	c.cache[name] = services

	return services, nil
}

// getCached 从缓存获取
func (c *Client) getCached(name string) ([]*Service, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	services, ok := c.cache[name]
	return services, ok
}

// InvalidateCache 失效缓存
func (c *Client) InvalidateCache(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, name)
}

// ==================== 负载均衡器 ====================

// LoadBalancer 负载均衡器
type LoadBalancer struct {
	strategy LoadBalanceStrategy
}

// LoadBalanceStrategy 负载均衡策略
type LoadBalanceStrategy int

const (
	StrategyRoundRobin LoadBalanceStrategy = iota
	StrategyWeighted
	StrategyLeastConnections
	StrategyRandom
)

// NewLoadBalancer 创建负载均衡器
func NewLoadBalancer(strategy LoadBalanceStrategy) *LoadBalancer {
	return &LoadBalancer{
		strategy: strategy,
	}
}

// Select 选择服务
func (lb *LoadBalancer) Select(services []*Service) (*Service, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services available")
	}

	// 过滤健康服务
	var healthy []*Service
	for _, s := range services {
		if s.Healthy {
			healthy = append(healthy, s)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy services")
	}

	switch lb.strategy {
	case StrategyRoundRobin:
		return healthy[0], nil // 需要维护索引
	case StrategyWeighted:
		return lb.weightedSelect(healthy), nil
	case StrategyLeastConnections:
		return healthy[0], nil // 需要连接跟踪
	case StrategyRandom:
		return healthy[rand.Intn(len(healthy))], nil
	default:
		return healthy[0], nil
	}
}

// weightedSelect 权重选择
func (lb *LoadBalancer) weightedSelect(services []*Service) *Service {
	var totalWeight int
	for _, s := range services {
		totalWeight += s.Weight
	}

	random := rand.Intn(totalWeight)
	for _, s := range services {
		random -= s.Weight
		if random < 0 {
			return s
		}
	}

	return services[0]
}

// ==================== 工具函数 ====================

// GetLocalServices 获取本地服务列表
func GetLocalServices() []*Service {
	// 简化实现
	return []*Service{
		{
			Name:    "collector",
			Address: "localhost",
			Port:    9091,
		},
		{
			Name:    "consensus",
			Address: "localhost",
			Port:    9092,
		},
		{
			Name:    "rpc",
			Address: "localhost",
			Port:    9090,
		},
	}
}
