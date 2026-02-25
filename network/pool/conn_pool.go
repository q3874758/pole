package pool

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Config 连接池配置
type Config struct {
	MaxIdle     int           // 最大空闲连接
	MaxOpen     int           // 最大打开连接
	MaxLifetime time.Duration // 最大生命周期
	MaxIdleTime time.Duration // 最大空闲时间
}

func DefaultConfig() *Config {
	return &Config{
		MaxIdle:     10,
		MaxOpen:     100,
		MaxLifetime: 30 * time.Minute,
		MaxIdleTime: 10 * time.Minute,
	}
}

// Conn 连接包装
type Conn struct {
	net.Conn
	createdAt time.Time
	lastUsed  time.Time
	inUse     bool
}

// ConnectionPool 连接池
type ConnectionPool struct {
	config *Config
	mu     sync.Mutex
	conns  []*Conn
	count  int32 // 当前打开的连接数
	ctx    context.Context
	cancel context.CancelFunc
}

// NewConnectionPool 创建连接池
func NewConnectionPool(cfg *Config) *ConnectionPool {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ConnectionPool{
		config: cfg,
		conns:  make([]*Conn, 0),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Get 获取连接
func (p *ConnectionPool) Get(network, addr string) (net.Conn, error) {
	// 先尝试从空闲池获取
	p.mu.Lock()
	for i := len(p.conns) - 1; i >= 0; i-- {
		conn := p.conns[i]
		if !conn.inUse && time.Since(conn.lastUsed) < p.config.MaxIdleTime {
			conn.inUse = true
			p.mu.Unlock()
			return conn, nil
		}
	}
	p.mu.Unlock()

	// 检查是否达到最大连接数
	if atomic.LoadInt32(&p.count) >= int32(p.config.MaxOpen) {
		// 等待空闲连接或超时
		ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
		defer cancel()
		<-ctx.Done()
		return nil, ErrPoolExhausted
	}

	// 创建新连接
	conn, err := net.DialTimeout(network, addr, 10*time.Second)
	if err != nil {
		return nil, err
	}

	atomic.AddInt32(&p.count, 1)
	return &Conn{
		Conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		inUse:     true,
	}, nil
}

// Put 归还连接
func (p *ConnectionPool) Put(conn net.Conn) {
	if conn == nil {
		return
	}

	c, ok := conn.(*Conn)
	if !ok {
		conn.Close()
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// 检查连接是否过期
	if time.Since(c.createdAt) > p.config.MaxLifetime {
		c.Conn.Close()
		atomic.AddInt32(&p.count, -1)
		return
	}

	c.inUse = false
	c.lastUsed = time.Now()
	p.conns = append(p.conns, c)
}

// Close 关闭连接池
func (p *ConnectionPool) Close() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.conns {
		conn.Conn.Close()
	}
	atomic.StoreInt32(&p.count, 0)
	p.conns = nil
}

// Count 获取连接数
func (p *ConnectionPool) Count() int32 {
	return atomic.LoadInt32(&p.count)
}

var ErrPoolExhausted = &poolError{"connection pool exhausted"}

type poolError struct {
	msg string
}

func (e *poolError) Error() string {
	return e.msg
}

// ==================== 对象池（通用）====================

// Pool 对象池接口
type Pool interface {
	Get() interface{}
	Put(interface{})
}

// ObjectPool 通用对象池
type ObjectPool struct {
	mu       sync.Mutex
	objects  []interface{}
	factory  func() interface{}
	reset    func(interface{})
	maxSize  int
}

// NewObjectPool 创建对象池
func NewObjectPool(factory func() interface{}, reset func(interface{}), maxSize int) *ObjectPool {
	return &ObjectPool{
		factory: factory,
		reset:   reset,
		maxSize: maxSize,
		objects: make([]interface{}, 0),
	}
}

// Get 获取对象
func (p *ObjectPool) Get() interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.objects) > 0 {
		obj := p.objects[len(p.objects)-1]
		p.objects = p.objects[:len(p.objects)-1]
		return obj
	}

	return p.factory()
}

// Put 归还对象
func (p *ObjectPool) Put(obj interface{}) {
	if obj == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.reset != nil {
		p.reset(obj)
	}

	if len(p.objects) < p.maxSize {
		p.objects = append(p.objects, obj)
	}
}
