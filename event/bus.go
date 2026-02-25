package event

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== 事件配置 ====================

// Config 事件配置
type Config struct {
	MaxQueueSize    int           // 最大队列大小
	WorkerCount    int           // 工作器数量
	RetryCount     int           // 重试次数
	RetryDelay     time.Duration // 重试延迟
	EnablePersistence bool      // 启用持久化
	RetentionPeriod time.Duration // 保留周期
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxQueueSize:     10000,
		WorkerCount:      4,
		RetryCount:       3,
		RetryDelay:       1 * time.Second,
		EnablePersistence: false,
		RetentionPeriod:  24 * time.Hour,
	}
}

// ==================== 事件类型 ====================

// EventType 事件类型
type EventType string

const (
	// 共识事件
	EventBlockProposed   EventType = "block_proposed"
	EventBlockAccepted   EventType = "block_accepted"
	EventBlockRejected   EventType = "block_rejected"
	EventVoteSubmitted   EventType = "vote_submitted"
	EventValidatorUpdate EventType = "validator_update"

	// 数据事件
	EventDataCollected    EventType = "data_collected"
	EventDataVerified     EventType = "data_verified"
	EventDataRejected     EventType = "data_rejected"
	EventGVSUpdated       EventType = "gvs_updated"

	// 经济事件
	EventBlockReward    EventType = "block_reward"
	EventDelegation     EventType = "delegation"
	EventUndelegation   EventType = "undelegation"
	EventSlash          EventType = "slash"
	EventTokensBurned   EventType = "tokens_burned"

	// 治理事件
	EventProposalCreated  EventType = "proposal_created"
	EventProposalVoted   EventType = "proposal_voted"
	EventProposalPassed  EventType = "proposal_passed"
	EventProposalExecuted EventType = "proposal_executed"

	// 网络事件
	EventNodeJoined    EventType = "node_joined"
	EventNodeLeft      EventType = "node_left"
	EventNodeheartbeat EventType = "node_heartbeat"
)

// ==================== 事件 ====================

// Event 事件
type Event struct {
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	Payload   interface{}       `json:"payload"`
	Metadata  map[string]string `json:"metadata"`
	Timestamp int64             `json:"timestamp"`
	Source    string           `json:"source"`
	BlockHeight uint64         `json:"block_height"`
}

// NewEvent 创建新事件
func NewEvent(eventType EventType, payload interface{}, source string) *Event {
	return &Event{
		ID:         generateEventID(),
		Type:       eventType,
		Payload:    payload,
		Metadata:   make(map[string]string),
		Timestamp:  time.Now().UnixNano(),
		Source:     source,
		BlockHeight: 0,
	}
}

// ToJSON 序列化为 JSON
func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON 从 JSON 反序列化
func (e *Event) FromJSON(data []byte) error {
	return json.Unmarshal(data, e)
}

// ==================== 事件总线 ====================

// EventBus 事件总线
type EventBus struct {
	config     *Config
	subs       map[EventType]map[chan *Event]struct{}
	handlers   map[EventType][]EventHandler
	queue      chan *Event
	workers    []worker
	persister  EventPersister
	mu         sync.RWMutex
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	isRunning  bool
}

// EventHandler 事件处理器
type EventHandler func(event *Event) error

// worker 工作器
type worker struct {
	id      int
	events  chan *Event
	handler EventHandler
}

// NewEventBus 创建事件总线
func NewEventBus(config *Config) *EventBus {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	bus := &EventBus{
		config:   config,
		subs:     make(map[EventType]map[chan *Event]struct{}),
		handlers: make(map[EventType][]EventHandler),
		queue:    make(chan *Event, config.MaxQueueSize),
		ctx:      ctx,
		cancel:   cancel,
	}

	// 启动工作器
	for i := 0; i < config.WorkerCount; i++ {
		bus.workers = append(bus.workers, worker{
			id:     i,
			events: make(chan *Event, config.MaxQueueSize/config.WorkerCount),
		})
	}

	return bus
}

// Start 启动事件总线
func (eb *EventBus) Start() error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.isRunning {
		return fmt.Errorf("event bus already running")
	}

	// 启动工作器
	for i := range eb.workers {
		eb.wg.Add(1)
		go eb.runWorker(&eb.workers[i])
	}

	// 启动分发循环
	eb.wg.Add(1)
	go eb.dispatchLoop()

	eb.isRunning = true
	return nil
}

// Stop 停止事件总线
func (eb *EventBus) Stop() {
	eb.cancel()
	eb.wg.Wait()

	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.isRunning = false
}

// Subscribe 订阅事件
func (eb *EventBus) Subscribe(eventType EventType, ch chan *Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.subs[eventType] == nil {
		eb.subs[eventType] = make(map[chan *Event]struct{})
	}
	eb.subs[eventType][ch] = struct{}{}
}

// Unsubscribe 取消订阅
func (eb *EventBus) Unsubscribe(eventType EventType, ch chan *Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if subs, ok := eb.subs[eventType]; ok {
		delete(subs, ch)
		close(ch)
	}
}

// SubscribeHandler 订阅处理器
func (eb *EventBus) SubscribeHandler(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// Publish 发布事件
func (eb *EventBus) Publish(event *Event) error {
	if !eb.isRunning {
		return fmt.Errorf("event bus not running")
	}

	// 持久化
	if eb.config.EnablePersistence && eb.persister != nil {
		if err := eb.persister.Save(event); err != nil {
			return fmt.Errorf("persist event: %v", err)
		}
	}

	// 入队
	select {
	case eb.queue <- event:
		return nil
	default:
		return fmt.Errorf("event queue full")
	}
}

// PublishSync 同步发布
func (eb *EventBus) PublishSync(event *Event) error {
	eb.mu.RLock()
	handlers := eb.handlers[event.Type]
	subs := eb.subs[event.Type]
	eb.mu.RUnlock()

	// 调用处理器
	for _, handler := range handlers {
		if err := handler(event); err != nil {
			// 记录错误但继续
			fmt.Printf("event handler error: %v\n", err)
		}
	}

	// 发送到订阅者
	for ch := range subs {
		select {
		case ch <- event:
		default:
			// 队列满，跳过
		}
	}

	return nil
}

// dispatchLoop 分发循环
func (eb *EventBus) dispatchLoop() {
	defer eb.wg.Done()

	for {
		select {
		case <-eb.ctx.Done():
			return
		case event := <-eb.queue:
			// 分发到工作器
			workerIdx := int(event.Timestamp) % len(eb.workers)
			select {
			case eb.workers[workerIdx].events <- event:
			default:
				// 工作器队列满
			}
		}
	}
}

// runWorker 运行工作器
func (eb *EventBus) runWorker(w *worker) {
	defer eb.wg.Done()

	for {
		select {
		case <-eb.ctx.Done():
			return
		case event := <-w.events:
			eb.handleEvent(event)
		}
	}
}

// handleEvent 处理事件
func (eb *EventBus) handleEvent(event *Event) {
	eb.mu.RLock()
	handlers := eb.handlers[event.Type]
	subs := eb.subs[event.Type]
	eb.mu.RUnlock()

	// 调用处理器
	for _, handler := range handlers {
		if err := handler(event); err != nil {
			eb.retryHandler(handler, event)
		}
	}

	// 发送到订阅者
	for ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// retryHandler 重试处理器
func (eb *EventBus) retryHandler(handler EventHandler, event *Event) {
	for i := 0; i < eb.config.RetryCount; i++ {
		select {
		case <-eb.ctx.Done():
			return
		case <-time.After(eb.config.RetryDelay):
			if err := handler(event); err == nil {
				return
			}
		}
	}
}

// ==================== 事件持久化 ====================

// EventPersister 事件持久化器
type EventPersister interface {
	Save(event *Event) error
	Load(id string) (*Event, error)
	LoadByType(eventType EventType, limit int) ([]*Event, error)
	LoadByTimeRange(start, end int64) ([]*Event, error)
	Cleanup(retentionPeriod time.Duration) error
}

// MemoryEventStore 内存事件存储
type MemoryEventStore struct {
	events map[string]*Event
	mu     sync.RWMutex
}

// NewMemoryEventStore 创建内存存储
func NewMemoryEventStore() *MemoryEventStore {
	return &MemoryEventStore{
		events: make(map[string]*Event),
	}
}

// Save 保存事件
func (mes *MemoryEventStore) Save(event *Event) error {
	mes.mu.Lock()
	defer mes.mu.Unlock()

	mes.events[event.ID] = event
	return nil
}

// Load 加载事件
func (mes *MemoryEventStore) Load(id string) (*Event, error) {
	mes.mu.RLock()
	defer mes.mu.RUnlock()

	if event, ok := mes.events[id]; ok {
		return event, nil
	}
	return nil, fmt.Errorf("event not found")
}

// LoadByType 按类型加载
func (mes *MemoryEventStore) LoadByType(eventType EventType, limit int) ([]*Event, error) {
	mes.mu.RLock()
	defer mes.mu.RUnlock()

	var results []*Event
	for _, event := range mes.events {
		if event.Type == eventType {
			results = append(results, event)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// LoadByTimeRange 按时间范围加载
func (mes *MemoryEventStore) LoadByTimeRange(start, end int64) ([]*Event, error) {
	mes.mu.RLock()
	defer mes.mu.RUnlock()

	var results []*Event
	for _, event := range mes.events {
		if event.Timestamp >= start && event.Timestamp <= end {
			results = append(results, event)
		}
	}
	return results, nil
}

// Cleanup 清理
func (mes *MemoryEventStore) Cleanup(retentionPeriod time.Duration) error {
	mes.mu.Lock()
	defer mes.mu.Unlock()

	cutoff := time.Now().Add(-retentionPeriod).UnixNano()
	for id, event := range mes.events {
		if event.Timestamp < cutoff {
			delete(mes.events, id)
		}
	}
	return nil
}

// SetPersister 设置持久化器
func (eb *EventBus) SetPersister(p EventPersister) {
	eb.persister = p
}

// ==================== 事件过滤器 ====================

// EventFilter 事件过滤器
type EventFilter struct {
	Types       []EventType
	Sources     []string
	BlockHeight uint64
	StartTime   int64
	EndTime     int64
}

// Match 匹配事件
func (ef *EventFilter) Match(event *Event) bool {
	// 检查类型
	if len(ef.Types) > 0 {
		found := false
		for _, t := range ef.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 检查来源
	if len(ef.Sources) > 0 {
		found := false
		for _, s := range ef.Sources {
			if event.Source == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 检查区块高度
	if ef.BlockHeight > 0 && event.BlockHeight != ef.BlockHeight {
		return false
	}

	// 检查时间范围
	if ef.StartTime > 0 && event.Timestamp < ef.StartTime {
		return false
	}
	if ef.EndTime > 0 && event.Timestamp > ef.EndTime {
		return false
	}

	return true
}

// FilterEvents 过滤事件
func (eb *EventBus) FilterEvents(filter *EventFilter) []*Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	var results []*Event
	for _, handlers := range eb.handlers {
		for _, handler := range handlers {
			_ = handler // 需要修改以支持过滤
		}
	}

	// 从存储中获取
	if mes, ok := eb.persister.(*MemoryEventStore); ok {
		for _, event := range mes.events {
			if filter.Match(event) {
				results = append(results, event)
			}
		}
	}

	return results
}

// ==================== 工具函数 ====================

func generateEventID() string {
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), time.Now().Unix()%10000)
}

// ==================== 事件Payload类型 ====================

// BlockEventData 区块事件数据
type BlockEventData struct {
	Block   *types.Block `json:"block"`
	Proposer types.Address `json:"proposer"`
}

// DataEventData 数据事件数据
type DataEventData struct {
	GameID     types.GameID    `json:"game_id"`
	DataPoints []types.GameDataPoint `json:"data_points"`
	NodeID     types.NodeID    `json:"node_id"`
	Verified   bool           `json:"verified"`
}

// RewardEventData 奖励事件数据
type RewardEventData struct {
	Validator types.Address     `json:"validator"`
	Amount    types.TokenAmount `json:"amount"`
	Type      string           `json:"type"`
}

// ProposalEventData 提案事件数据
type ProposalEventData struct {
	ProposalID uint64   `json:"proposal_id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Voter      *types.Address `json:"voter,omitempty"`
}
