package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"pole-core/core/types"
)

// ==================== P2P 配置 ====================

// Config P2P 配置
type Config struct {
	ListenAddr       string        // 监听地址
	BootstrapNodes   []string      // 引导节点列表
	MaxPeers         int           // 最大连接数
	MaxPendingPeers  int           // 最大待处理连接数
	HandshakeTimeout time.Duration // 握手超时
	ReadTimeout      time.Duration // 读取超时
	WriteTimeout     time.Duration // 写入超时
	pingInterval     time.Duration // ping 间隔
}

func DefaultConfig() *Config {
	return &Config{
		ListenAddr:       ":26656",
		BootstrapNodes:   []string{},
		MaxPeers:         50,
		MaxPendingPeers:  10,
		HandshakeTimeout: 10 * time.Second,
		ReadTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		pingInterval:     30 * time.Second,
	}
}

// ==================== 消息类型 ====================

// MsgType 消息类型
type MsgType byte

const (
	MsgHandshake       MsgType = 0x01 // 握手
	MsgPing            MsgType = 0x02 // Ping
	MsgPong            MsgType = 0x03 // Pong
	MsgBlock           MsgType = 0x10 // 区块
	MsgTx              MsgType = 0x11 // 交易
	MsgConsensus       MsgType = 0x12 // 共识消息
	MsgDataRequest     MsgType = 0x20 // 数据请求
	MsgDataResponse    MsgType = 0x21 // 数据响应
	MsgGameDataSubmit  MsgType = 0x22 // 游戏数据提交（挖矿）
	MsgGameDataConfirm MsgType = 0x23 // 游戏数据确认（验证节点）
	MsgVote            MsgType = 0x30 // 投票
	MsgProposal        MsgType = 0x31 // 提案
)

// Message P2P 消息
type Message struct {
	Type    MsgType     `json:"type"`
	Payload interface{} `json:"payload"`
}

// Peer 节点信息
type Peer struct {
	ID        string    `json:"id"`
	Addr      string    `json:"addr"`
	PubKey    []byte   `json:"pub_key"`
	Latency   time.Duration `json:"latency"`
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"last_seen"`
}

// ==================== P2P 网络 ====================

// Network P2P 网络
type Network struct {
	config    *Config
	nodeID   string      // 节点 ID
	peers     map[string]*Peer
	pending   map[string]*Peer
	listener  net.Listener
	httpSrv   *http.Server
	mu        sync.RWMutex
	msgChan   chan *PeerMessage
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	// 挖矿相关
	isValidator    bool                                           // 是否为验证节点
	OnDataConfirmed func(payload map[string]interface{})          // 数据确认回调
}

// GetNodeID 获取节点 ID
func (n *Network) GetNodeID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.nodeID
}

// GetP2PAddr 获取 P2P 地址
func (n *Network) GetP2PAddr() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.listener == nil {
		return ""
	}
	return n.listener.Addr().String()
}

// PeerMessage 节点消息
type PeerMessage struct {
	From    string
	Message *Message
}

// NewNetwork 创建 P2P 网络
func NewNetwork(cfg *Config) *Network {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Network{
		config:   cfg,
		peers:    make(map[string]*Peer),
		pending:  make(map[string]*Peer),
		msgChan:  make(chan *PeerMessage, 1000),
		running:  false,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start 启动 P2P 网络
func (n *Network) Start() error {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return fmt.Errorf("network already running")
	}

	// 生成节点 ID（简化：使用随机字符串）
	n.nodeID = fmt.Sprintf("pole%016x", time.Now().UnixNano())

	// 启动监听
	ln, err := net.Listen("tcp", n.config.ListenAddr)
	if err != nil {
		n.mu.Unlock()
		return fmt.Errorf("listen: %w", err)
	}
	n.listener = ln

	// 启动 HTTP API（可选，用于节点发现）
	n.startHTTPAPI()

	n.running = true
	n.mu.Unlock()

	// 启动接受循环
	go n.acceptLoop()

	// 连接引导节点
	go n.dialBootstrapNodes()

	// 启动 ping 循环
	go n.pingLoop()

	// 启动消息处理循环
	go n.handleMessages()

	fmt.Printf("[P2P] 网络已启动: %s\n", n.config.ListenAddr)
	return nil
}

// Stop 停止 P2P 网络
func (n *Network) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.running {
		return
	}

	n.cancel()
	n.running = false

	// 关闭监听
	if n.listener != nil {
		n.listener.Close()
	}

	// 关闭 HTTP
	if n.httpSrv != nil {
		n.httpSrv.Shutdown(context.Background())
	}

	// 关闭所有连接（简化处理）
	n.peers = make(map[string]*Peer)
	n.pending = make(map[string]*Peer)

	fmt.Printf("[P2P] 网络已停止\n")
}

// acceptLoop 接受连接循环
func (n *Network) acceptLoop() {
	for {
		select {
		case <-n.ctx.Done():
			return
		default:
		}

		// 使用超时 accept
		conn, err := n.listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if n.running {
				fmt.Printf("[P2P] 接受连接错误: %v\n", err)
			}
			continue
		}

		go n.handleConnection(conn)
	}
}

// handleConnection 处理新连接
func (n *Network) handleConnection(conn net.Conn) {
	defer conn.Close()

	// 简化：直接添加为节点（实际需要握手协议）
	addr := conn.RemoteAddr().String()
	peer := &Peer{
		ID:        addr,
		Addr:      addr,
		Connected: true,
		LastSeen:  time.Now(),
	}

	n.mu.Lock()
	if len(n.peers) >= n.config.MaxPeers {
		n.mu.Unlock()
		conn.Close()
		return
	}
	n.peers[peer.ID] = peer
	n.mu.Unlock()

	fmt.Printf("[P2P] 新节点连接: %s (总数: %d)\n", peer.ID, len(n.peers))

	// 保持连接
	buf := make([]byte, 4096)
	for n.running {
		conn.SetDeadline(time.Now().Add(n.config.ReadTimeout))
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("[P2P] 读取错误 from %s: %v\n", peer.ID, err)
			}
			break
		}
		// 处理消息（简化）
		_ = n
	}

	// 断开连接
	n.mu.Lock()
	delete(n.peers, peer.ID)
	n.mu.Unlock()
	fmt.Printf("[P2P] 节点断开: %s\n", peer.ID)
}

// dialBootstrapNodes 连接引导节点
func (n *Network) dialBootstrapNodes() {
	for _, addr := range n.config.BootstrapNodes {
		select {
		case <-n.ctx.Done():
			return
		default:
		}

		go n.dialNode(addr)
	}
}

// dialNode 连接到节点
func (n *Network) dialNode(addr string) {
	conn, err := net.DialTimeout("tcp", addr, n.config.HandshakeTimeout)
	if err != nil {
		fmt.Printf("[P2P] 连接失败 %s: %v\n", addr, err)
		return
	}
	defer conn.Close()

	n.handleConnection(conn)
}

// pingLoop Ping 循环
func (n *Network) pingLoop() {
	ticker := time.NewTicker(n.config.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			n.mu.RLock()
			for _, peer := range n.peers {
				// 简化：更新最后可见时间
				peer.LastSeen = time.Now()
			}
			n.mu.RUnlock()
		}
	}
}

// handleMessages 消息处理循环
func (n *Network) handleMessages() {
	for {
		select {
		case <-n.ctx.Done():
			return
		case msg := <-n.msgChan:
			n.processMessage(msg)
		}
	}
}

// processMessage 处理消息
func (n *Network) processMessage(msg *PeerMessage) {
	switch msg.Message.Type {
	case MsgBlock:
		// 广播区块
		n.Broadcast(msg.Message, msg.From)
	case MsgTx:
		// 广播交易
		n.Broadcast(msg.Message, msg.From)
	case MsgDataRequest:
		// 处理数据请求
	case MsgDataResponse:
		// 处理数据响应
	case MsgGameDataSubmit:
		// 处理游戏数据提交（挖矿）
		n.handleGameDataSubmit(msg)
	case MsgGameDataConfirm:
		// 处理游戏数据确认（验证节点）
		n.handleGameDataConfirm(msg)
	default:
		fmt.Printf("[P2P] 未知消息类型: %v\n", msg.Message.Type)
	}
}

// handleGameDataSubmit 处理游戏数据提交
func (n *Network) handleGameDataSubmit(msg *PeerMessage) {
	// 解析提交的数据
	payload, ok := msg.Message.Payload.(map[string]interface{})
	if !ok {
		return
	}

	// 验证数据签名（简化版）
	// 实际需要验证签名是否有效

	// 广播给其他节点
	n.Broadcast(msg.Message, msg.From)

	// 如果是验证节点，生成确认
	if n.isValidator {
		n.sendConfirmation(payload)
	}
}

// handleGameDataConfirm 处理游戏数据确认
func (n *Network) handleGameDataConfirm(msg *PeerMessage) {
	// 解析确认消息
	// 通知奖励模块
	if n.OnDataConfirmed != nil {
		payload, ok := msg.Message.Payload.(map[string]interface{})
		if ok {
			n.OnDataConfirmed(payload)
		}
	}
}

// sendConfirmation 发送确认消息
func (n *Network) sendConfirmation(submitPayload map[string]interface{}) {
	dataHash, ok := submitPayload["data_hash"].(string)
	if !ok {
		return
	}

	confirmMsg := &Message{
		Type: MsgGameDataConfirm,
		Payload: map[string]interface{}{
			"data_hash": dataHash,
			"validator": n.nodeID,
			"timestamp": time.Now().Unix(),
			"status":    "confirmed",
		},
	}
	n.Broadcast(confirmMsg, "")
}

// BroadcastGameData 广播游戏数据（挖矿提交）
func (n *Network) BroadcastGameData(data interface{}) error {
	msg := &Message{
		Type:    MsgGameDataSubmit,
		Payload: data,
	}
	n.Broadcast(msg, "")
	return nil
}

// SetValidatorMode 设置验证节点模式
func (n *Network) SetValidatorMode(isValidator bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.isValidator = isValidator
}

// Broadcast 广播消息（排除发送者）
func (n *Network) Broadcast(msg *Message, excludeID string) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	data, _ := json.Marshal(msg)
	for id, peer := range n.peers {
		if id == excludeID {
			continue
		}
		// 简化：实际应该通过连接发送
		_ = peer
		_ = data
	}
}

// SendTo 发送消息到指定节点
func (n *Network) SendTo(peerID string, msg *Message) error {
	n.mu.RLock()
	peer, ok := n.peers[peerID]
	n.mu.RUnlock()

	if !ok {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	data, _ := json.Marshal(msg)
	// 简化：实际应该通过连接发送
	_ = data
	_ = peer.Addr
	return nil
}

// GetPeers 获取所有节点
func (n *Network) GetPeers() []*Peer {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]*Peer, 0, len(n.peers))
	for _, p := range n.peers {
		result = append(result, p)
	}
	return result
}

// GetPeerCount 获取节点数量
func (n *Network) GetPeerCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.peers)
}

// ==================== HTTP API（节点发现）====================

func (n *Network) startHTTPAPI() {
	mux := http.NewServeMux()
	mux.HandleFunc("/p2p/peers", n.handlePeers)
	mux.HandleFunc("/p2p/connect", n.handleConnect)

	n.httpSrv = &http.Server{
		Addr:    ":26660",
		Handler: mux,
	}

	go func() {
		if err := n.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[P2P] HTTP API 错误: %v\n", err)
		}
	}()
}

func (n *Network) handlePeers(w http.ResponseWriter, r *http.Request) {
	peers := n.GetPeers()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(peers),
		"peers": peers,
	})
}

func (n *Network) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Addr string `json:"addr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	go n.dialNode(req.Addr)
	json.NewEncoder(w).Encode(map[string]string{"status": "connecting"})
}

// ==================== 简化版广播实现 ====================

// BroadcastBlock 广播区块
func (n *Network) BroadcastBlock(block *types.Block) {
	msg := &Message{
		Type:    MsgBlock,
		Payload: block,
	}
	n.Broadcast(msg, "")
}

// BroadcastTx 广播交易
func (n *Network) BroadcastTx(tx *types.Transaction) {
	msg := &Message{
		Type:    MsgTx,
		Payload: tx,
	}
	n.Broadcast(msg, "")
}

// BroadcastData 广播游戏数据
func (n *Network) BroadcastData(data *types.GameDataPoint) {
	msg := &Message{
		Type:    MsgDataResponse,
		Payload: data,
	}
	n.Broadcast(msg, "")
}

// ==================== 节点发现（简化）====================

// DiscoverNodes 节点发现
func (n *Network) DiscoverNodes() []string {
	// 简化：从已知节点列表随机选择
	n.mu.RLock()
	peers := make([]string, 0, len(n.peers))
	for id := range n.peers {
		peers = append(peers, id)
	}
	n.mu.RUnlock()

	// 随机打乱
	rand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	// 返回前 N 个
	if len(peers) > 10 {
		peers = peers[:10]
	}
	return peers
}
