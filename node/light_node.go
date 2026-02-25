package node

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== 节点类型 ====================

// NodeType 节点类型
type NodeType int

const (
	NodeTypeFull NodeType = iota // 全功能节点
	NodeTypeLight               // 轻节点
	NodeTypeStorage             // 存储节点
	NodeTypeCollector           // 采集节点
)

func (nt NodeType) String() string {
	switch nt {
	case NodeTypeFull:
		return "FullNode"
	case NodeTypeLight:
		return "LightNode"
	case NodeTypeStorage:
		return "StorageNode"
	case NodeTypeCollector:
		return "CollectorNode"
	default:
		return "Unknown"
	}
}

// ==================== 轻节点配置 ====================

// LightNodeConfig 轻节点配置
type LightNodeConfig struct {
	// 基本信息
	NodeType       NodeType   // 节点类型
	NodeName       string    // 节点名称
	
	// 采集配置
	EnableCollection bool     // 启用数据采集
	CollectionGames []string // 采集的游戏列表
	CollectionInterval time.Duration // 采集间隔
	MaxConcurrentGames int // 最大并发采集游戏数
	
	// 存储配置（自愿存储）
	EnableStorage bool      // 启用存储
	StorageQuota  int64    // 存储配额（字节）
	StoragePath   string   // 存储路径
	
	// 验证配置
	EnableValidation bool  // 启用验证
	MinStakeRequired int64 // 最小质押要求
	
	// 资源限制
	MaxCPUPercent int     // 最大CPU占用百分比
	MaxMemoryMB    int    // 最大内存使用 MB
	MaxBandwidthKB int    // 最大带宽 KB/s
	
	// 网络配置
	Bootnodes []string   // 引导节点
	ListenAddr  string  // 监听地址
}

// DefaultLightNodeConfig 默认轻节点配置
func DefaultLightNodeConfig() *LightNodeConfig {
	return &LightNodeConfig{
		NodeType:        NodeTypeLight,
		NodeName:        "LightNode",
		EnableCollection: true,
		CollectionGames: []string{},
		CollectionInterval: 5 * time.Minute,
		MaxConcurrentGames: 10,
		
		EnableStorage: true,
		StorageQuota:  10 * 1024 * 1024 * 1024, // 10GB
		StoragePath:   "./data",
		
		EnableValidation: false,
		MinStakeRequired: 0,
		
		MaxCPUPercent:   5,
		MaxMemoryMB:    500,
		MaxBandwidthKB: 1024,
		
		Bootnodes: []string{},
		ListenAddr: ":9090",
	}
}

// ==================== 轻节点 ====================

// LightNode 轻节点
type LightNode struct {
	config    *LightNodeConfig
	id        types.NodeID
	address   types.Address
	status    NodeStatus
	roles     map[NodeRole]bool
	
	// 采集器
	collector *Collector
	
	// 存储
	storage *VoluntaryStorage
	
	// 验证
	validator *LightValidator
	
	// 资源使用
	resources *ResourceMonitor
	
	// 同步
	mu    sync.RWMutex
	wg    sync.WaitGroup
	ctx   context.Context
	cancel context.CancelFunc
}

// NodeRole 节点角色
type NodeRole int

const (
	RoleCollector NodeRole = iota // 数据采集
	RoleStorage                  // 数据存储
	RoleValidator                // 数据验证
	RoleBackup                   // 备份存储
)

// NodeStatus 节点状态
type NodeStatus int

const (
	NodeStatusOffline NodeStatus = iota
	NodeStatusStarting
	NodeStatusOnline
	NodeStatusSyncing
	NodeStatusStopping
)

// NewLightNode 创建轻节点
func NewLightNode(config *LightNodeConfig) (*LightNode, error) {
	if config == nil {
		config = DefaultLightNodeConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// 生成节点ID
	nodeID := types.NewNode(types.Address{}).ID

	// 创建角色映射
	roles := make(map[NodeRole]bool)
	if config.EnableCollection {
		roles[RoleCollector] = true
	}
	if config.EnableStorage {
		roles[RoleStorage] = true
	}
	if config.EnableValidation {
		roles[RoleValidator] = true
	}

	node := &LightNode{
		config:    config,
		id:        nodeID,
		status:    NodeStatusOffline,
		roles:     roles,
		resources: NewResourceMonitor(config.MaxCPUPercent, config.MaxMemoryMB),
		ctx:       ctx,
		cancel:    cancel,
	}

	// 初始化子模块
	if config.EnableCollection {
		node.collector = NewCollector(config.CollectionGames, config.CollectionInterval)
	}
	if config.EnableStorage {
		node.storage = NewVoluntaryStorage(config.StoragePath, config.StorageQuota)
	}
	if config.EnableValidation {
		node.validator = NewLightValidator(config.MinStakeRequired)
	}

	return node, nil
}

// Start 启动节点
func (ln *LightNode) Start() error {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	if ln.status == NodeStatusOnline {
		return fmt.Errorf("node already online")
	}

	ln.status = NodeStatusStarting

	// 启动资源监控
	ln.resources.Start()

	// 启动采集器
	if ln.collector != nil {
		ln.wg.Add(1)
		go func() {
			defer ln.wg.Done()
			ln.collector.Run(ln.ctx)
		}()
	}

	// 启动存储
	if ln.storage != nil {
		if err := ln.storage.Start(); err != nil {
			return fmt.Errorf("start storage: %v", err)
		}
	}

	// 启动验证器
	if ln.validator != nil {
		ln.wg.Add(1)
		go func() {
			defer ln.wg.Done()
			ln.validator.Run(ln.ctx)
		}()
	}

	ln.status = NodeStatusOnline
	return nil
}

// Stop 停止节点
func (ln *LightNode) Stop() error {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	ln.cancel()
	ln.wg.Wait()

	if ln.storage != nil {
		ln.storage.Stop()
	}

	ln.resources.Stop()
	ln.status = NodeStatusOffline

	return nil
}

// GetStatus 获取节点状态
func (ln *LightNode) GetStatus() NodeStatus {
	ln.mu.RLock()
	defer ln.mu.RUnlock()
	return ln.status
}

// GetID 获取节点ID
func (ln *LightNode) GetID() types.NodeID {
	return ln.id
}

// HasRole 检查是否有角色
func (ln *LightNode) HasRole(role NodeRole) bool {
	ln.mu.RLock()
	defer ln.mu.RUnlock()
	return ln.roles[role]
}

// ==================== 自愿存储 ====================

// VoluntaryStorage 自愿存储
type VoluntaryStorage struct {
	config     *StorageConfig
	store      *DataStore
	quotaUsed  int64
	quotaMax   int64
	mu         sync.RWMutex
	isRunning  bool
}

// StorageConfig 存储配置
type StorageConfig struct {
	StoragePath    string
	Quota         int64
	RetentionDays  int
	EnableErasure  bool
	DataShards    int
	ParityShards  int
}

// DataStore 数据存储
type DataStore struct {
	data map[string][]byte
	mu   sync.RWMutex
}

// NewVoluntaryStorage 创建自愿存储
func NewVoluntaryStorage(path string, quota int64) *VoluntaryStorage {
	return &VoluntaryStorage{
		config: &StorageConfig{
			StoragePath:    path,
			Quota:         quota,
			RetentionDays:  30,
			EnableErasure:  true,
			DataShards:    4,
			ParityShards:  2,
		},
		store:    &DataStore{data: make(map[string][]byte)},
		quotaMax: quota,
	}
}

// Start 启动存储
func (vs *VoluntaryStorage) Start() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.isRunning = true
	return nil
}

// Stop 停止存储
func (vs *VoluntaryStorage) Stop() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.isRunning = false
}

// Store 存储数据
func (vs *VoluntaryStorage) Store(key string, data []byte) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if !vs.isRunning {
		return fmt.Errorf("storage not running")
	}

	// 检查配额
	newSize := vs.quotaUsed + int64(len(data))
	if newSize > vs.quotaMax {
		return fmt.Errorf("storage quota exceeded")
	}

	// 纠删码编码（如果启用）
	if vs.config.EnableErasure {
		encoded := vs.encodeErasure(data)
		vs.store.data[key] = encoded
	} else {
		vs.store.data[key] = data
	}

	vs.quotaUsed = newSize
	return nil
}

// Retrieve 检索数据
func (vs *VoluntaryStorage) Retrieve(key string) ([]byte, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	data, ok := vs.store.data[key]
	if !ok {
		return nil, fmt.Errorf("data not found")
	}

	// 纠删码解码
	if vs.config.EnableErasure {
		return vs.decodeErasure(data)
	}

	return data, nil
}

// Delete 删除数据
func (vs *VoluntaryStorage) Delete(key string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if data, ok := vs.store.data[key]; ok {
		vs.quotaUsed -= int64(len(data))
		delete(vs.store.data, key)
	}

	return nil
}

// GetQuotaInfo 获取配额信息
func (vs *VoluntaryStorage) GetQuotaInfo() (used, available, total int64) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return vs.quotaUsed, vs.quotaMax - vs.quotaUsed, vs.quotaMax
}

// encodeErasure 纠删码编码 (简化实现)
func (vs *VoluntaryStorage) encodeErasure(data []byte) []byte {
	// 简化：添加简单的校验和
	checksum := calculateChecksum(data)
	return append(data, checksum...)
}

// decodeErasure 纠删码解码 (简化实现)
func (vs *VoluntaryStorage) decodeErasure(encoded []byte) ([]byte, error) {
	// 简化：验证校验和
	if len(encoded) < 4 {
		return nil, fmt.Errorf("invalid encoded data")
	}
	data := encoded[:len(encoded)-4]
	checksum := encoded[len(encoded)-4:]
	
	expected := calculateChecksum(data)
	if string(checksum) != string(expected) {
		return nil, fmt.Errorf("checksum mismatch")
	}
	
	return data, nil
}

// calculateChecksum 计算校验和
func calculateChecksum(data []byte) []byte {
	sum := uint32(0)
	for _, b := range data {
		sum = sum*31 + uint32(b)
	}
	return []byte{byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum)}
}

// ==================== 轻量验证器 ====================

// LightValidator 轻量验证器
type LightValidator struct {
	minStake     int64
	votes       map[string]*ValidationVote
	voteCounts  map[string]int
	mu          sync.RWMutex
}

// ValidationVote 验证投票
type ValidationVote struct {
	DataHash [32]byte
	Voter    types.NodeID
	Approve  bool
	Stake    int64
	Time     int64
}

// NewLightValidator 创建轻量验证器
func NewLightValidator(minStake int64) *LightValidator {
	return &LightValidator{
		minStake:    minStake,
		votes:       make(map[string]*ValidationVote),
		voteCounts: make(map[string]int),
	}
}

// Run 运行验证器
func (lv *LightValidator) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lv.cleanup()
		}
	}
}

// SubmitVote 提交投票
func (lv *LightValidator) SubmitVote(dataHash [32]byte, voter types.NodeID, approve bool, stake int64) error {
	if stake < lv.minStake {
		return fmt.Errorf("insufficient stake")
	}

	lv.mu.Lock()
	defer lv.mu.Unlock()

	key := fmt.Sprintf("%x", dataHash)
	lv.votes[key] = &ValidationVote{
		DataHash: dataHash,
		Voter:    voter,
		Approve:  approve,
		Stake:    stake,
		Time:     time.Now().Unix(),
	}
	lv.voteCounts[key]++

	return nil
}

// GetResult 获取验证结果
func (lv *LightValidator) GetResult(dataHash [32]byte) (bool, bool) {
	lv.mu.RLock()
	defer lv.mu.RUnlock()

	key := fmt.Sprintf("%x", dataHash)
	vote, ok := lv.votes[key]
	if !ok {
		return false, false
	}

	// 简单多数决
	return vote.Approve, true
}

// cleanup 清理过期投票
func (lv *LightValidator) cleanup() {
	lv.mu.Lock()
	defer lv.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute).Unix()
	for key, vote := range lv.votes {
		if vote.Time < cutoff {
			delete(lv.votes, key)
			delete(lv.voteCounts, key)
		}
	}
}

// ==================== 采集器 ====================

// Collector 采集器
type Collector struct {
	games      []string
	interval   time.Duration
	dataPoints []CollectedData
	mu         sync.RWMutex
}

// CollectedData 采集的数据
type CollectedData struct {
	GameID    types.GameID
	Players   uint64
	Timestamp int64
	Source    string
}

// NewCollector 创建采集器
func NewCollector(games []string, interval time.Duration) *Collector {
	return &Collector{
		games:    games,
		interval: interval,
		dataPoints: make([]CollectedData, 0),
	}
}

// Run 运行采集器
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect 执行采集
func (c *Collector) collect() {
	// 简化实现 - 实际会调用 Steam API
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	for _, game := range c.games {
		// 模拟采集
		data := CollectedData{
			GameID:    types.GameID(game),
			Players:   uint64(math.RandomUint64() % 10000),
			Timestamp: now,
			Source:    "steam",
		}
		c.dataPoints = append(c.dataPoints, data)
	}
}

// GetDataPoints 获取采集的数据
func (c *Collector) GetDataPoints() []CollectedData {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]CollectedData, len(c.dataPoints))
	copy(result, c.dataPoints)
	return result
}

// ==================== 资源监控 ====================

// ResourceMonitor 资源监控
type ResourceMonitor struct {
	maxCPUPercent int
	maxMemoryMB   int
	currentCPU    int
	currentMemory int
	mu           sync.RWMutex
	isRunning    bool
}

// NewResourceMonitor 创建资源监控
func NewResourceMonitor(maxCPU int, maxMemory int) *ResourceMonitor {
	return &ResourceMonitor{
		maxCPUPercent: maxCPU,
		maxMemoryMB:   maxMemory,
	}
}

// Start 启动监控
func (rm *ResourceMonitor) Start() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.isRunning = true
}

// Stop 停止监控
func (rm *ResourceMonitor) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.isRunning = false
}

// GetUsage 获取资源使用情况
func (rm *ResourceMonitor) GetUsage() (cpu int, memory int) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.currentCPU, rm.currentMemory
}

// CheckLimit 检查是否超出限制
func (rm *ResourceMonitor) CheckLimit() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.currentCPU <= rm.maxCPUPercent && rm.currentMemory <= rm.maxMemoryMB
}

// ==================== 节点管理器 ====================

// NodeManager 节点管理器
type NodeManager struct {
	nodes    map[types.NodeID]*LightNode
	registry *NodeRegistry
	mu       sync.RWMutex
}

// NodeRegistry 节点注册表
type NodeRegistry struct {
	nodesByType map[NodeType][]types.NodeID
	nodesByRole map[NodeRole][]types.NodeID
	mu          sync.RWMutex
}

// NewNodeManager 创建节点管理器
func NewNodeManager() *NodeManager {
	return &NodeManager{
		nodes:    make(map[types.NodeID]*LightNode),
		registry: &NodeRegistry{
			nodesByType: make(map[NodeType][]types.NodeID),
			nodesByRole: make(map[NodeRole][]types.NodeID),
		},
	}
}

// RegisterNode 注册节点
func (nm *NodeManager) RegisterNode(node *LightNode) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	nodeID := node.GetID()
	if _, exists := nm.nodes[nodeID]; exists {
		return fmt.Errorf("node already registered")
	}

	nm.nodes[nodeID] = node

	// 更新注册表
	nm.registry.nodesByType[node.config.NodeType] = append(
		nm.registry.nodesByType[node.config.NodeType], nodeID)

	for role := range node.roles {
		nm.registry.nodesByRole[role] = append(nm.registry.nodesByRole[role], nodeID)
	}

	return nil
}

// UnregisterNode 注销节点
func (nm *NodeManager) UnregisterNode(nodeID types.NodeID) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	node, exists := nm.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found")
	}

	delete(nm.nodes, nodeID)

	// 从注册表移除
	for role := range node.roles {
		nm.removeFromRole(role, nodeID)
	}

	return nil
}

// removeFromRole 从角色列表移除
func (nm *NodeManager) removeFromRole(role NodeRole, nodeID types.NodeID) {
	nodes := nm.registry.nodesByRole[role]
	for i, id := range nodes {
		if id == nodeID {
			nm.registry.nodesByRole[role] = append(nodes[:i], nodes[i+1:]...)
			break
		}
	}
}

// GetNodesByType 按类型获取节点
func (nm *NodeManager) GetNodesByType(nodeType NodeType) []*LightNode {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	nodeIDs := nm.registry.nodesByType[nodeType]
	result := make([]*LightNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		if node, ok := nm.nodes[id]; ok {
			result = append(result, node)
		}
	}
	return result
}

// GetNodesByRole 按角色获取节点
func (nm *NodeManager) GetNodesByRole(role NodeRole) []*LightNode {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	nodeIDs := nm.registry.nodesByRole[role]
	result := make([]*LightNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		if node, ok := nm.nodes[id]; ok {
			result = append(result, node)
		}
	}
	return result
}

// GetNodeCount 获取节点数量
func (nm *NodeManager) GetNodeCount() int {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return len(nm.nodes)
}

// GetStats 获取节点统计
func (nm *NodeManager) GetStats() NodeStats {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	stats := NodeStats{
		TotalNodes: len(nm.nodes),
		ByType:     make(map[string]int),
		ByRole:    make(map[string]int),
	}

	for nodeType, nodes := range nm.registry.nodesByType {
		stats.ByType[nodeType.String()] = len(nodes)
	}

	for role, nodes := range nm.registry.nodesByRole {
		stats.ByRole[role.String()] = len(nodes)
	}

	return stats
}

// NodeStats 节点统计
type NodeStats struct {
	TotalNodes int
	ByType     map[string]int
	ByRole     map[string]int
}
