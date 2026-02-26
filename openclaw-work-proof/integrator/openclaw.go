package openclaw

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"worktracker"
)

// OpenClawIntegrator OpenClaw 集成器
type OpenClawIntegrator struct {
	tracker    *worktracker.Tracker
	agentID    string
	statsURL   string
	eventChan  chan *Event
	stopChan   chan bool
	wg         sync.WaitGroup
}

// Event OpenClaw 事件
type Event struct {
	Type      string    `json:"type"`       // message/tool/exec/done
	Timestamp int64     `json:"timestamp"`
	Content   string    `json:"content"`
	AgentID   string    `json:"agent_id"`
	SessionID string    `json:"session_id"`
	Model     string    `json:"model"`
	Tokens    *Tokens   `json:"tokens"`
	Tools     []*Tool   `json:"tools"`
}

// Tokens Token 统计
type Tokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Cache  int64 `json:"cache"`
}

// Tool 工具调用
type Tool struct {
	Name     string `json:"name"`
	Input    string `json:"input"`
	Output   string `json:"output"`
	Duration int64  `json:"duration_ms"`
	Success  bool   `json:"success"`
}

// NewOpenClawIntegrator 创建集成器
func NewOpenClawIntegrator(tracker *worktracker.Tracker, agentID string) *OpenClawIntegrator {
	return &OpenClawIntegrator{
		tracker:   tracker,
		agentID:   agentID,
		statsURL:  "http://localhost:18789/api/stats",
		eventChan: make(chan *Event, 1000),
		stopChan:  make(chan bool),
	}
}

// StartPolling 开始轮询
func (o *OpenClawIntegrator) StartPolling(interval time.Duration) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for {
			select {
			case <-o.stopChan:
				return
			case <-ticker.C:
				o.poll()
			}
		}
	}()
}

// poll 轮询获取数据
func (o *OpenClawIntegrator) poll() {
	// 获取会话统计
	resp, err := http.Get(o.statsURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return
	}
	
	var stats StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return
	}
	
	// 处理每个会话
	for _, session := range stats.Sessions {
		event := &Event{
			Type:      "session",
			Timestamp: time.Now().UnixMilli(),
			AgentID:   o.agentID,
			SessionID: session.ID,
			Model:     session.Model,
			Tokens: &Tokens{
				Input:  session.Tokens.Input,
				Output: session.Tokens.Output,
			},
		}
		o.eventChan <- event
	}
}

// StartListener 启动日志监听
func (o *OpenClawIntegrator) StartListener(logFile string) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		
		// 简化版：读取日志文件
		// 实际实现应该用 tail -f
		
		for {
			select {
			case <-o.stopChan:
				return
			case event := <-o.eventChan:
				o.processEvent(event)
			}
		}
	}()
}

// processEvent 处理事件
func (o *OpenClawIntegrator) processEvent(event *Event) {
	// 根据事件类型判断任务
	taskType := o.detectTaskType(event)
	
	if taskType == "" {
		return
	}
	
	// 开始任务
	record := o.tracker.StartTask(
		o.agentID,
		fmt.Sprintf("Session: %s", event.SessionID),
		taskType,
	)
	
	// 等待任务完成（简化版：直接标记完成）
	time.Sleep(100 * time.Millisecond)
	
	result := worktracker.TaskResult{
		TokensInput:  event.Tokens.Input,
		TokensOutput: event.Tokens.Output,
	}
	
	// 解析工具调用获取更多信息
	for _, tool := range event.Tools {
		if tool.Success {
			switch tool.Name {
			case "exec", "bash", "powershell":
				result.CodeLines += estimateCodeLines(tool.Output)
			case "write", "edit":
				result.CodeFiles++
				result.CodeLines += estimateCodeLines(tool.Output)
			}
		}
	}
	
	o.tracker.CompleteTask(record, result)
}

// detectTaskType 检测任务类型
func (o *OpenClawIntegrator) detectTaskType(event *Event) worktracker.TaskType {
	content := event.Content
	tools := event.Tools
	
	// 根据内容关键词判断
	lower := strings.ToLower(content)
	
	// 代码相关
	codeKeywords := []string{"func ", "def ", "class ", "const ", "let ", "import ", "package "}
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			return worktracker.TaskCoding
		}
	}
	
	// 调试相关
	debugKeywords := []string{"error", "bug", "fix", "debug", "exception", "traceback"}
	for _, kw := range debugKeywords {
		if strings.Contains(lower, kw) {
			return worktracker.TaskDebug
		}
	}
	
	// 部署相关
	deployKeywords := []string{"deploy", "docker", "kubernetes", "kubectl", "npm run", "build", "serve"}
	for _, kw := range deployKeywords {
		if strings.Contains(lower, kw) {
			return worktracker.TaskDeploy
		}
	}
	
	// 文档相关
	docKeywords := []string{"readme", "document", "comment", "explain", "describe"}
	for _, kw := range docKeywords {
		if strings.Contains(lower, kw) {
			return worktracker.TaskDoc
		}
	}
	
	// 工具判断
	for _, tool := range tools {
		if strings.HasPrefix(tool.Name, "exec") || strings.HasPrefix(tool.Name, "bash") {
			return worktracker.TaskCoding
		}
		if strings.HasPrefix(tool.Name, "write") || strings.HasPrefix(tool.Name, "edit") {
			if isCodeFile(tool.Name) {
				return worktracker.TaskCoding
			}
			return worktracker.TaskWriting
		}
	}
	
	// 默认
	return worktracker.TaskResearch
}

func estimateCodeLines(output string) int {
	lines := strings.Split(output, "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 0 && !strings.HasPrefix(line, "//") && 
		   !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "/*") {
			count++
		}
	}
	return count
}

func isCodeFile(name string) bool {
	codeExts := []string{".go", ".js", ".ts", ".py", ".rs", ".java", ".cpp", ".c", ".h", ".cs"}
	for _, ext := range codeExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// StatsResponse 统计响应
type StatsResponse struct {
	Sessions []Session `json:"sessions"`
}

type Session struct {
	ID     string    `json:"id"`
	Model  string    `json:"model"`
	Tokens TokenInfo `json:"tokens"`
	Started int64   `json:"started"`
}

type TokenInfo struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

// Stop 停止
func (o *OpenClawIntegrator) Stop() {
	close(o.stopChan)
	o.wg.Wait()
}

// ============ HTTP API ============

type APIServer struct {
	tracker *worktracker.Tracker
	port    string
}

func NewAPIServer(tracker *worktracker.Tracker, port string) *APIServer {
	return &APIServer{
		tracker: tracker,
		port:    port,
	}
}

func (a *APIServer) Start() {
	http.HandleFunc("/api/stats", a.handleStats)
	http.HandleFunc("/api/records", a.handleRecords)
	http.HandleFunc("/api/proof", a.handleProof)
	
	go http.ListenAndServe(a.port, nil)
}

func (a *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := a.tracker.GetStats()
	json.NewEncoder(w).Encode(stats)
}

func (a *APIServer) handleRecords(w http.ResponseWriter, r *http.Request) {
	records := a.tracker.GetRecords(50)
	json.NewEncoder(w).Encode(records)
}

func (a *APIServer) handleProof(w http.ResponseWriter, r *http.Request) {
	records := a.tracker.GetRecords(100)
	
	// 生成汇总证明
	combined := ""
	for _, r := range records {
		combined += r.ProofHash
	}
	
	json.NewEncoder(w).Encode(map[string]string{
		"combined_hash": fmt.Sprintf("%x", combined),
		"record_count": fmt.Sprintf("%d", len(records)),
	})
}
