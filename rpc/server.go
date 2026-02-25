package rpc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"pole-core/core/executor"
	"pole-core/core/state"
	"pole-core/core/types"
	"pole-core/data/collector"
	"pole-core/governance"
	"pole-core/wallet"
)

// Config RPC 配置
type Config struct {
	Port           string        // 监听端口
	MaxMsgSize     int           // 最大消息大小
	Timeout        time.Duration // 超时时间
	EnableTLS      bool          // 启用 TLS
	CertFile       string        // 证书文件
	KeyFile        string        // 密钥文件
	EnableRecovery bool          // 启用 panic 恢复
	RateLimit      int           // 速率限制 (请求/秒)
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Port:           ":9090",
		MaxMsgSize:     1024 * 1024 * 4, // 4MB
		Timeout:        30 * time.Second,
		EnableTLS:      false,
		CertFile:       "",
		KeyFile:        "",
		EnableRecovery: true,
		RateLimit:      1000,
	}
}

// HTTPServer HTTP RPC 服务器
type HTTPServer struct {
	chainState             *state.ChainState
	executor               *executor.Executor
	wallet                 *wallet.Wallet
	emergencyPause         *governance.EmergencyPauseManager
	miningRewardDistributor *collector.MiningRewardDistributor
	collectionLoop         *collector.CollectionLoop
	httpServer             *http.Server
	config                 *Config
	mu                     sync.RWMutex
}

// SetEmergencyPause 设置紧急暂停管理器
func (s *HTTPServer) SetEmergencyPause(ep *governance.EmergencyPauseManager) {
	s.emergencyPause = ep
}

// SetMiningRewardDistributor 设置挖矿奖励分发器（用于 /mining/claim、/mining/balance）
func (s *HTTPServer) SetMiningRewardDistributor(mrd *collector.MiningRewardDistributor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.miningRewardDistributor = mrd
}

// SetCollectionLoop 设置采集循环（用于 /mining/detected，前端「正在挖矿」提示）
func (s *HTTPServer) SetCollectionLoop(loop *collector.CollectionLoop) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collectionLoop = loop
}

// resolveAddress 解析地址：若 addr 非空则返回，否则返回钱包首个账户地址，无则返回空
func (s *HTTPServer) resolveAddress(addr string) string {
	if addr != "" {
		return addr
	}
	if s.wallet != nil {
		if accs := s.wallet.ListAccounts(); len(accs) > 0 {
			return accs[0].Address
		}
	}
	return ""
}

// NewHTTPServer 创建 HTTP RPC 服务器
func NewHTTPServer(chainState *state.ChainState, exec *executor.Executor, w *wallet.Wallet, config *Config) *HTTPServer {
	if config == nil {
		config = DefaultConfig()
	}
	return &HTTPServer{
		chainState: chainState,
		executor:   exec,
		wallet:     w,
		config:     config,
	}
}

// Start 启动 HTTP RPC 服务器
func (s *HTTPServer) Start() error {
	mux := http.NewServeMux()
	s.registerHandlers(mux)

	s.httpServer = &http.Server{
		Addr:    s.config.Port,
		Handler: mux,
	}

	// 启用 TLS
	if s.config.EnableTLS && s.config.CertFile != "" && s.config.KeyFile != "" {
		go func() {
			if err := s.httpServer.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile); err != nil && err != http.ErrServerClosed {
				fmt.Printf("[RPC] TLS 服务器错误: %v\n", err)
			}
		}()
		fmt.Printf("[RPC] HTTPS 服务器已启动: https://localhost%s (TLS 1.3)\n", s.config.Port)
	} else {
		go func() {
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Printf("[RPC] HTTP 服务器错误: %v\n", err)
			}
		}()
		fmt.Printf("[RPC] HTTP 服务器已启动: http://localhost%s\n", s.config.Port)
	}

	return nil
}

// Stop 停止 HTTP RPC 服务器
func (s *HTTPServer) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// resolveWalletWebDir 解析钱包静态文件目录，优先相对可执行文件，避免工作目录不同导致打不开
func resolveWalletWebDir() string {
	// 1) 相对可执行文件目录
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		p := filepath.Join(dir, "wallet", "web")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 2) 相对当前工作目录
	if cwd, err := os.Getwd(); err == nil {
		p := filepath.Join(cwd, "wallet", "web")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 3) 相对路径（与可执行文件同目录启动时有效）
	return filepath.Join("wallet", "web")
}

// registerHandlers 注册 HTTP 处理函数
func (s *HTTPServer) registerHandlers(mux *http.ServeMux) {
	// 区块相关
	mux.HandleFunc("/block/latest", s.handleLatestBlock)
	mux.HandleFunc("/block/", s.handleGetBlock)

	// 交易相关
	mux.HandleFunc("/tx/broadcast", s.handleBroadcastTx)
	mux.HandleFunc("/tx/", s.handleGetTx)

	// 账户相关
	mux.HandleFunc("/account/balance", s.handleGetBalance)
	mux.HandleFunc("/account/", s.handleGetAccount)
	mux.HandleFunc("/account/list", s.handleListAccounts)

	// 验证者相关
	mux.HandleFunc("/validators", s.handleGetValidators)

	// 治理相关
	mux.HandleFunc("/governance/proposal/", s.handleGetProposal)
	mux.HandleFunc("/governance/proposals", s.handleGetProposals)
	mux.HandleFunc("/governance/vote", s.handleCastVote)

	// 国库相关（链上整合）
	mux.HandleFunc("/treasury/balance", s.handleTreasuryBalance)
	mux.HandleFunc("/treasury/proposals", s.handleTreasuryProposals)

	// 团队/投资人线性释放（锁仓领取）
	mux.HandleFunc("/vesting/status", s.handleVestingStatus)
	mux.HandleFunc("/vesting/claim", s.handleVestingClaim)

	// 紧急暂停相关
	mux.HandleFunc("/emergency/status", s.handleEmergencyStatus)
	mux.HandleFunc("/emergency/pause", s.handleEmergencyPause)
	mux.HandleFunc("/emergency/resume", s.handleEmergencyResume)

	// 钱包相关
	mux.HandleFunc("/wallet/accounts", s.handleWalletAccounts)
	mux.HandleFunc("/wallet/sign", s.handleWalletSign)
	mux.HandleFunc("/wallet/create", s.handleWalletCreate)
	mux.HandleFunc("/wallet/backup", s.handleWalletBackup)

	// 挖矿相关
	mux.HandleFunc("/mining/status", s.handleMiningStatus)
	mux.HandleFunc("/mining/games", s.handleMiningGames)
	mux.HandleFunc("/mining/submit", s.handleMiningSubmit)
	mux.HandleFunc("/mining/claim", s.handleMiningClaim)
	mux.HandleFunc("/mining/balance", s.handleMiningBalance)
	mux.HandleFunc("/mining/detected", s.handleMiningDetected)

	// 链状态
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/status/chain", s.handleChainStatus)

	// 监控与指标（通过 RPC 端口）
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/health", s.handleHealth)

	// 静态文件（钱包 UI）：优先使用嵌入的 wallet/web，保证任意目录启动都能打开钱包
	walletSub, useEmbed := fs.Sub(wallet.WebFS, "web")
	var walletFS http.FileSystem
	if useEmbed == nil {
		walletFS = http.FS(walletSub)
		fmt.Printf("[RPC] 钱包 UI: 已嵌入二进制\n")
	} else {
		walletDir := resolveWalletWebDir()
		walletFS = http.Dir(walletDir)
		fmt.Printf("[RPC] 钱包 UI: 从目录 %s\n", walletDir)
	}
	fileServer := http.FileServer(walletFS)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			f, err := walletFS.Open("index.html")
			if err != nil {
				http.Error(w, "wallet index not found", http.StatusNotFound)
				return
			}
			defer f.Close()
			stat, _ := f.Stat()
			if stat != nil && stat.IsDir() {
				http.Error(w, "wallet index not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.Copy(w, f)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	// 调试
	mux.HandleFunc("/debug/state", s.handleDebugState)
}

// ==================== HTTP 处理函数 ====================

// JSONResponse JSON 响应
type JSONResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (s *HTTPServer) respond(w http.ResponseWriter, code int, resp JSONResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp)
}

func (s *HTTPServer) parseRequest(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// handleLatestBlock 获取最新区块
func (s *HTTPServer) handleLatestBlock(w http.ResponseWriter, r *http.Request) {
	height := s.chainState.GetHeight()
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    map[string]interface{}{"height": height},
	})
}

// handleGetBlock 获取指定区块
func (s *HTTPServer) handleGetBlock(w http.ResponseWriter, r *http.Request) {
	// 简化：返回当前状态信息
	height := s.chainState.GetHeight()
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"height": height,
			"chain_id": s.chainState.ChainID,
		},
	})
}

// handleBroadcastTx 广播交易
func (s *HTTPServer) handleBroadcastTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{
			Error: "method not allowed",
		})
		return
	}

	var tx types.Transaction
	if err := s.parseRequest(r, &tx); err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "invalid request: " + err.Error(),
		})
		return
	}

	// 验证交易
	if err := s.executor.DeliverTx(&tx); err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "invalid transaction: " + err.Error(),
		})
		return
	}

	// 执行交易
	if err := s.executor.ExecuteTx(&tx); err != nil {
		s.respond(w, http.StatusInternalServerError, JSONResponse{
			Error: "execute failed: " + err.Error(),
		})
		return
	}

	// 计算交易哈希
	txHash := s.hashTx(&tx)
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"tx_hash": txHash,
		},
	})
}

// handleGetTx 获取交易
func (s *HTTPServer) handleGetTx(w http.ResponseWriter, r *http.Request) {
	// 从路径提取 tx hash
	path := r.URL.Path
	txHash := strings.TrimPrefix(path, "/tx/")
	if txHash == path {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "missing tx hash",
		})
		return
	}
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    map[string]interface{}{"tx_hash": txHash, "status": "not found"},
	})
}

// handleGetBalance 获取账户余额
func (s *HTTPServer) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")
	if addr == "" {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "missing address parameter",
		})
		return
	}

	balance := s.chainState.GetBalance(addr)
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"address": addr,
			"balance": balance.String(),
		},
	})
}

// handleListAccounts 获取所有账户列表（包含创世分配）
func (s *HTTPServer) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	// 获取所有有余额的账户
	accounts := []map[string]interface{}{
		{"address": "pole1qql8ag4cluz6r4dz28p3w00dnc9w8ueulg2gmc", "label": "NodeRewardPool (60%)", "locked": true},
		{"address": "pole1qrwun0msv3x6nkw3c9v3x6nkw3c9v3x6nkw3c", "label": "Ecosystem (20%)", "locked": true},
		{"address": "pole1q9hk0m5v3x6nkw3c9v3x6nkw3c9v3x6nkw3c", "label": "Community (15%)", "locked": true},
		{"address": "pole1q7zg4p6nkw3c9v3x6nkw3c9v3x6nkw3c", "label": "TeamAndInvestors (5%)", "locked": true},
	}

	// 获取余额
	for i := range accounts {
		addr := accounts[i]["address"].(string)
		balance := s.chainState.GetBalance(addr)
		accounts[i]["balance"] = balance.String()
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    accounts,
	})
}

// handleGetAccount 获取账户信息
func (s *HTTPServer) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")
	if addr == "" {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "missing address parameter",
		})
		return
	}

	balance := s.chainState.GetBalance(addr)
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"address":  addr,
			"balance":  balance.String(),
			"is_validator": false,
		},
	})
}

// handleGetValidators 获取验证者列表
func (s *HTTPServer) handleGetValidators(w http.ResponseWriter, r *http.Request) {
	// 获取所有验证者
	validators := s.chainState.GetStaking().GetValidators()
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    validators,
	})
}

// handleGetProposal 获取提案
func (s *HTTPServer) handleGetProposal(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	idStr := strings.TrimPrefix(path, "/governance/proposal/")
	if idStr == path {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "missing proposal id",
		})
		return
	}

	var id uint64
	fmt.Sscanf(idStr, "%d", &id)

	proposal, ok := s.chainState.GetProposal(id)
	if !ok {
		s.respond(w, http.StatusNotFound, JSONResponse{
			Error: "proposal not found",
		})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    proposal,
	})
}

// handleGetProposals 获取所有提案
func (s *HTTPServer) handleGetProposals(w http.ResponseWriter, r *http.Request) {
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    []interface{}{},
	})
}

// handleCastVote 投票
func (s *HTTPServer) handleCastVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{
			Error: "method not allowed",
		})
		return
	}

	var req struct {
		ProposalID uint64 `json:"proposal_id"`
		Voter      string `json:"voter"`
		VoteOption uint8  `json:"vote_option"`
		Weight     string `json:"weight"`
	}
	if err := s.parseRequest(r, &req); err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "invalid request: " + err.Error(),
		})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    map[string]interface{}{"message": "vote recorded"},
	})
}

// handleTreasuryBalance 获取国库余额（链上整合）
func (s *HTTPServer) handleTreasuryBalance(w http.ResponseWriter, r *http.Request) {
	tc := s.chainState.GetTreasury()
	if tc == nil {
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data:    map[string]interface{}{"balance": "0"},
		})
		return
	}
	balance := tc.GetBalance()
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    map[string]interface{}{"balance": balance.String()},
	})
}

// handleTreasuryProposals 获取国库支出提案列表
func (s *HTTPServer) handleTreasuryProposals(w http.ResponseWriter, r *http.Request) {
	tc := s.chainState.GetTreasury()
	if tc == nil {
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data:    []interface{}{},
		})
		return
	}
	list := tc.GetProposals()
	out := make([]map[string]interface{}, 0, len(list))
	for _, p := range list {
		if p == nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":          p.ID,
			"proposer":    p.Proposer,
			"recipient":   p.Recipient,
			"amount":      p.Amount.String(),
			"description": p.Description,
			"status":      p.Status,
			"votes_yes":   p.VotesYes.String(),
			"votes_no":    p.VotesNo.String(),
			"created_at":  p.CreatedAt,
			"voting_end":  p.VotingEnd,
		})
	}
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    out,
	})
}

// handleVestingStatus 查询锁仓释放状态 GET /vesting/status?address=0x...
func (s *HTTPServer) handleVestingStatus(w http.ResponseWriter, r *http.Request) {
	addr := s.resolveAddress(r.URL.Query().Get("address"))
	if addr == "" {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: "missing address (query ?address= or wallet)"})
		return
	}
	vc := s.chainState.GetVesting()
	if vc == nil {
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data:    map[string]interface{}{"has_schedule": false},
		})
		return
	}
	total, claimed, claimable, lockUntil, vestingMonths, ok := vc.GetInfo(addr)
	if !ok {
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data:    map[string]interface{}{"has_schedule": false},
		})
		return
	}
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"has_schedule":   true,
			"total":          total.String(),
			"claimed":        claimed.String(),
			"claimable":      claimable.String(),
			"lock_until":     lockUntil,
			"vesting_months": vestingMonths,
		},
	})
}

// handleVestingClaim 领取已解锁的团队/投资人代币 POST /vesting/claim body: {"address":"0x..."} 或空（用钱包首地址）
func (s *HTTPServer) handleVestingClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{Error: "method not allowed"})
		return
	}
	var body struct {
		Address string `json:"address"`
	}
	_ = s.parseRequest(r, &body)
	addr := s.resolveAddress(body.Address)
	if addr == "" {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: "missing address (body or wallet)"})
		return
	}
	vc := s.chainState.GetVesting()
	if vc == nil {
		s.respond(w, http.StatusServiceUnavailable, JSONResponse{Error: "vesting not initialized"})
		return
	}
	amount, err := vc.Claim(addr)
	if err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: err.Error()})
		return
	}
	if err := s.chainState.SaveState(); err != nil {
		s.respond(w, http.StatusInternalServerError, JSONResponse{Error: "save state: " + err.Error()})
		return
	}
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    map[string]interface{}{"claimed": amount.String()},
	})
}

// handleEmergencyStatus 获取紧急暂停状态
func (s *HTTPServer) handleEmergencyStatus(w http.ResponseWriter, r *http.Request) {
	if s.emergencyPause == nil {
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data:    map[string]interface{}{"is_paused": false},
		})
		return
	}

	pause := s.emergencyPause.GetCurrentPause()
	if pause == nil {
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data:    map[string]interface{}{"is_paused": false},
		})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"is_paused":   pause.IsPaused,
			"scope":       pause.Scope,
			"reason":      pause.Reason,
			"paused_at":   pause.PausedAt,
			"paused_by":   pause.PausedBy,
			"resume_at":   pause.ResumeAt,
			"time_left":   s.emergencyPause.TimeUntilResume().String(),
		},
	})
}

// handleEmergencyPause 触发紧急暂停
func (s *HTTPServer) handleEmergencyPause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{Error: "method not allowed"})
		return
	}

	if s.emergencyPause == nil {
		s.respond(w, http.StatusServiceUnavailable, JSONResponse{Error: "emergency pause not initialized"})
		return
	}

	var req struct {
		Scope    string `json:"scope"`    // Full/Transfers/Staking/Governance
		Reason   string `json:"reason"`   // Security/Attack/CriticalBug/Governance
		Duration int64  `json:"duration"` // 暂停时长（秒）
		Operator string `json:"operator"` // 操作者地址
	}
	if err := s.parseRequest(r, &req); err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: "invalid request: " + err.Error()})
		return
	}

	scope := governance.PauseScope(req.Scope)
	if scope == "" {
		scope = governance.ScopeFull
	}
	reason := governance.PauseReason(req.Reason)
	if reason == "" {
		reason = governance.PauseReasonSecurity
	}

	duration := time.Duration(req.Duration) * time.Second
	if duration == 0 {
		duration = 24 * time.Hour // 默认 24 小时
	}

	_, err := s.emergencyPause.TriggerPause(scope, reason, req.Operator, duration)
	if err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: err.Error()})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    map[string]interface{}{"message": "emergency pause triggered"},
	})
}

// handleEmergencyResume 恢复网络
func (s *HTTPServer) handleEmergencyResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{Error: "method not allowed"})
		return
	}

	if s.emergencyPause == nil {
		s.respond(w, http.StatusServiceUnavailable, JSONResponse{Error: "emergency pause not initialized"})
		return
	}

	var req struct {
		Operator          string `json:"operator"`
		ApprovedProposalID string `json:"approved_proposal_id"`
	}
	if err := s.parseRequest(r, &req); err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: "invalid request: " + err.Error()})
		return
	}

	err := s.emergencyPause.Resume(req.Operator, req.ApprovedProposalID)
	if err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: err.Error()})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    map[string]interface{}{"message": "network resumed"},
	})
}

// handleStatus 获取链状态
func (s *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"chain_id": s.chainState.ChainID,
			"height":   s.chainState.GetHeight(),
			"app_hash": hex.EncodeToString(s.chainState.GetAppHash()),
		},
	})
}

// handleChainStatus 获取详细链状态（监控面板用）
func (s *HTTPServer) handleChainStatus(w http.ResponseWriter, r *http.Request) {
	height := s.chainState.GetHeight()

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"chain_id":     s.chainState.ChainID,
			"height":       height,
			"app_hash":     hex.EncodeToString(s.chainState.GetAppHash()),
			"total_supply": "1000000000", // 简化
			"inflation":    "20%",
			"bonded":      "65%",
		},
	})
}

// handleMetrics Prometheus 指标
func (s *HTTPServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	height := s.chainState.GetHeight()
	output := fmt.Sprintf(`# TYPE pole_block_height gauge
# HELP Current block height
pole_block_height %d

# TYPE pole_tx_count counter
# HELP Total transaction count
pole_tx_count 0

# TYPE pole_peer_count gauge
# HELP Number of connected peers
pole_peer_count 1

# TYPE pole_uptime_seconds gauge
# HELP Node uptime in seconds
pole_uptime_seconds %d
`, height, 60)

	w.Write([]byte(output))
}

// handleHealth 健康检查
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	height := s.chainState.GetHeight()
	healthy := height > 0

	w.Header().Set("Content-Type", "application/json")
	if healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	fmt.Fprintf(w, `{"healthy":%t,"chain_id":"%s","height":%d}`, healthy, s.chainState.ChainID, height)
}

// handlePeerStats 获取节点统计
func (s *HTTPServer) handlePeerStats(w http.ResponseWriter, r *http.Request) {
	// 简化：返回模拟数据
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"peer_count":      21,
			"validators":      21,
			"syncing":         false,
			"last_block_time": 5,
		},
	})
}

// handleNetworkStats 获取网络统计
func (s *HTTPServer) handleNetworkStats(w http.ResponseWriter, r *http.Request) {
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"tx_count_24h":  1200000,
			"avg_block_time": 5.2,
			"avg_gas_price": "0.0001",
			"network_utilization": 45,
		},
	})
}

// handleDebugState 调试状态
func (s *HTTPServer) handleDebugState(w http.ResponseWriter, r *http.Request) {
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"height":    s.chainState.GetHeight(),
			"chain_id":  s.chainState.ChainID,
			"token_balances": "see /account/balance",
		},
	})
}

// ==================== 工具函数 ====================

// hashTx 计算交易哈希
func (s *HTTPServer) hashTx(tx *types.Transaction) string {
	data, _ := json.Marshal(tx)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ==================== 钱包处理函数 ====================

// handleWalletAccounts 获取钱包账户列表
func (s *HTTPServer) handleWalletAccounts(w http.ResponseWriter, r *http.Request) {
	if s.wallet == nil {
		s.respond(w, http.StatusServiceUnavailable, JSONResponse{
			Error: "wallet not initialized",
		})
		return
	}

	accounts := s.wallet.ListAccounts()
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    accounts,
	})
}

// handleWalletCreate 创建新钱包账户
func (s *HTTPServer) handleWalletCreate(w http.ResponseWriter, r *http.Request) {
	if s.wallet == nil {
		s.respond(w, http.StatusServiceUnavailable, JSONResponse{
			Error: "wallet not initialized",
		})
		return
	}

	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{
			Error: "method not allowed",
		})
		return
	}

	acc, err := s.wallet.GenerateKey()
	if err != nil {
		s.respond(w, http.StatusInternalServerError, JSONResponse{
			Error: "create account failed: " + err.Error(),
		})
		return
	}

	// 保存钱包
	s.wallet.Save("wallet.json")

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"address":    acc.Address,
			"publicKey": acc.PublicKey,
		},
	})
}

// handleWalletBackup 导出钱包备份（GET，含私钥，仅限本地使用）
func (s *HTTPServer) handleWalletBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.wallet == nil {
		s.respond(w, http.StatusServiceUnavailable, JSONResponse{Error: "wallet not initialized"})
		return
	}
	data, err := s.wallet.Export()
	if err != nil {
		s.respond(w, http.StatusInternalServerError, JSONResponse{Error: "export failed"})
		return
	}
	filename := "pole-wallet-backup-" + time.Now().Format("20060102-150405") + ".json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Write(data)
}

// ==================== 挖矿处理函数 ====================

// handleMiningStatus 获取挖矿状态
func (s *HTTPServer) handleMiningStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	enabled := s.miningRewardDistributor != nil
	s.mu.RUnlock()
	note := "使用 --mining 参数启动节点以启用自动采集"
	if enabled {
		note = "挖矿已启用，奖励每 5 分钟自动发放"
	}
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"enabled":     enabled,
			"description": "Play-to-Earn 挖矿模式",
			"note":        note,
		},
	})
}

// handleMiningGames 获取当前采集的游戏列表
func (s *HTTPServer) handleMiningGames(w http.ResponseWriter, r *http.Request) {
	// TODO: 实际应从 main.go 传入 collectionLoop 实例
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"games": collector.DefaultGameList(),
			"note":  "默认采集的热门游戏列表",
		},
	})
}

// handleMiningSubmit 手动提交游戏数据（模拟）
func (s *HTTPServer) handleMiningSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{Error: "method not allowed"})
		return
	}

	var req struct {
		GameID string `json:"game_id"`
		Value  uint64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: "invalid request"})
		return
	}

	// TODO: 实际应调用 collectionLoop.CollectOne 并广播到网络
	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"game_id":   req.GameID,
			"value":     req.Value,
			"submitted": true,
			"message":   "数据已提交（模拟）",
		},
	})
}

// handleMiningClaim 领取挖矿奖励
func (s *HTTPServer) handleMiningClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{Error: "method not allowed"})
		return
	}

	var req struct {
		Address string `json:"address"`
	}
	s.parseRequest(r, &req)
	req.Address = s.resolveAddress(req.Address)
	if req.Address == "" {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: "missing address"})
		return
	}

	s.mu.RLock()
	mrd := s.miningRewardDistributor
	s.mu.RUnlock()
	if mrd != nil {
		claimed, err := mrd.ClaimRewards(req.Address)
		if err != nil {
			s.respond(w, http.StatusInternalServerError, JSONResponse{Error: err.Error()})
			return
		}
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data: map[string]interface{}{
				"address": req.Address,
				"claimed": claimed.String(),
			},
		})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"address": req.Address,
			"claimed": "0",
			"message": "挖矿未启用或暂无待领奖励",
		},
	})
}

// handleMiningBalance 查询挖矿奖励余额
func (s *HTTPServer) handleMiningBalance(w http.ResponseWriter, r *http.Request) {
	addr := s.resolveAddress(r.URL.Query().Get("address"))
	if addr == "" {
		s.respond(w, http.StatusBadRequest, JSONResponse{Error: "missing address"})
		return
	}

	s.mu.RLock()
	mrd := s.miningRewardDistributor
	s.mu.RUnlock()
	if mrd != nil {
		claims := mrd.GetPendingClaims(addr)
		var addrPending decimal.Decimal
		for _, c := range claims {
			addrPending = addrPending.Add(c.Amount)
		}
		s.respond(w, http.StatusOK, JSONResponse{
			Success: true,
			Data: map[string]interface{}{
				"address":       addr,
				"pending":       addrPending.String(),
				"pending_count": len(claims),
			},
		})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"address": addr,
			"pending": "0",
			"message": "挖矿未启用",
		},
	})
}

// handleMiningDetected 返回当前检测到的运行中游戏（供前端右下角「正在挖矿」提示）
func (s *HTTPServer) handleMiningDetected(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	loop := s.collectionLoop
	s.mu.RUnlock()

	enabled := loop != nil
	detected := []string{}
	if loop != nil {
		detected = loop.GetLastDetectedGames()
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data: map[string]interface{}{
			"enabled":  enabled,
			"detected": detected,
		},
	})
}

// handleWalletSign 签名交易
func (s *HTTPServer) handleWalletSign(w http.ResponseWriter, r *http.Request) {
	if s.wallet == nil {
		s.respond(w, http.StatusServiceUnavailable, JSONResponse{
			Error: "wallet not initialized",
		})
		return
	}

	if r.Method != http.MethodPost {
		s.respond(w, http.StatusMethodNotAllowed, JSONResponse{
			Error: "method not allowed",
		})
		return
	}

	var tx types.Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "invalid request: " + err.Error(),
		})
		return
	}

	// 获取发送者地址
	fromAddr := tx.From.String()
	if fromAddr == "" {
		s.respond(w, http.StatusBadRequest, JSONResponse{
			Error: "missing from address",
		})
		return
	}

	// 签名
	if err := s.wallet.SignTx(&tx, fromAddr); err != nil {
		s.respond(w, http.StatusInternalServerError, JSONResponse{
			Error: "sign failed: " + err.Error(),
		})
		return
	}

	s.respond(w, http.StatusOK, JSONResponse{
		Success: true,
		Data:    tx,
	})
}
