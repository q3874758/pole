package worktracker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ============ 工作记录 ============

type TaskType string

const (
	TaskCoding     TaskType = "coding"      // 代码生成
	TaskWriting    TaskType = "writing"      // 文字创作
	TaskResearch   TaskType = "research"    // 调研分析
	TaskDebug      TaskType = "debug"        // 调试修复
	TaskDeploy     TaskType = "deploy"       // 部署运维
	TaskReview     TaskType = "review"       // 代码审查
	TaskDoc        TaskType = "doc"          // 文档编写
	TaskAnalysis   TaskType = "analysis"     // 数据分析
)

// WorkRecord 工作记录
type WorkRecord struct {
	ID           string    `json:"id"`            // 唯一ID
	AgentID      string    `json:"agent_id"`      // Agent ID
	TaskType     TaskType  `json:"task_type"`    // 任务类型
	TaskDesc     string    `json:"task_desc"`     // 任务描述
	Status       string    `json:"status"`         // pending/completed/failed
	StartedAt    int64     `json:"started_at"`   // 开始时间
	CompletedAt  int64     `json:"completed_at"`  // 完成时间
	
	// 工作量指标
	TokensInput   int64     `json:"tokens_input"`   // 输入 token
	TokensOutput  int64     `json:"tokens_output"`  // 输出 token
	CodeLines     int       `json:"code_lines"`     // 生成代码行
	CodeFiles     int       `json:"code_files"`     // 生成文件数
	WordsWritten  int       `json:"words_written"`  // 文字产出
	BugsFixed     int       `json:"bugs_fixed"`    // 修复 bug 数
	APICalls      int       `json:"api_calls"`      // API 调用次数
	ErrorsFixed   int       `json:"errors_fixed"`  // 错误修复数
	
	// 验证
	ProofHash    string    `json:"proof_hash"`    // 工作证明哈希
	Signature    string    `json:"signature"`      // 签名
}

// ============ 工作量计算 ============

// Weights 权重配置
var Weights = map[TaskType]float64{
	TaskCoding:   1.5,   // 代码生成价值高
	TaskDebug:    2.0,   // 调试修复价值最高
	TaskDeploy:   1.8,   // 部署运维
	TaskReview:   1.2,   // 代码审查
	TaskWriting:  1.0,   // 文字创作
	TaskResearch: 1.3,   // 调研分析
	TaskDoc:      0.8,   // 文档编写
	TaskAnalysis: 1.4,   // 数据分析
}

// CalculateValue 计算工作价值
func (w *WorkRecord) CalculateValue() float64 {
	// 基础价值 = 任务类型权重 × 完成状态
	statusMultiplier := 1.0
	if w.Status == "failed" {
		statusMultiplier = 0.3
	}
	
	baseValue := Weights[w.TaskType] * statusMultiplier
	
	// 代码贡献
	codeValue := float64(w.CodeLines) * 0.01
	if w.BugsFixed > 0 {
		codeValue += float64(w.BugsFixed) * 5.0 // 修复 bug 价值高
	}
	
	// 文字贡献
	wordValue := float64(w.WordsWritten) * 0.001
	
	// API 调用效率
	apiEfficiency := 0.0
	if w.APICalls > 0 && w.Status == "completed" {
		apiEfficiency = 1.0 / float64(w.APICalls) * 10
	}
	
	// Token 消耗成本
	tokenCost := float64(w.TokensInput+w.TokensOutput) * 0.0001
	
	return (baseValue + codeValue + wordValue + apiEfficiency) - tokenCost
}

// GenerateProof 生成工作证明
func (w *WorkRecord) GenerateProof() string {
	data := fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%d|%d|%d",
		w.AgentID,
		w.TaskType,
		w.TaskDesc,
		w.Status,
		w.TokensInput,
		w.TokensOutput,
		w.CodeLines,
		w.WordsWritten,
		w.BugsFixed,
		w.CompletedAt,
	)
	
	hash := sha256.Sum256([]byte(data))
	w.ProofHash = hex.EncodeToString(hash[:])
	
	return w.ProofHash
}

// ============ 工作量追踪器 ============

// Tracker 工作量追踪器
type Tracker struct {
	mu         sync.RWMutex
	records    map[string]*WorkRecord
	stats      *Stats
	dataDir    string
}

// Stats 统计数据
type Stats struct {
	TotalTasks      int            `json:"total_tasks"`
	CompletedTasks  int            `json:"completed_tasks"`
	FailedTasks    int            `json:"failed_tasks"`
	TotalTokens    int64          `json:"total_tokens"`
	TotalCodeLines int            `json:"total_code_lines"`
	TotalWords     int            `json:"total_words"`
	BugsFixed      int            `json:"bugs_fixed"`
	TotalValue     float64        `json:"total_value"`
	ByTaskType     map[string]int `json:"by_task_type"`
}

// NewTracker 创建追踪器
func NewTracker(dataDir string) (*Tracker, error) {
	t := &Tracker{
		records: make(map[string]*WorkRecord),
		stats: &Stats{
			ByTaskType: make(map[string]int),
		},
		dataDir: dataDir,
	}
	
	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	
	// 加载历史记录
	t.load()
	
	return t, nil
}

// StartTask 开始任务
func (t *Tracker) StartTask(agentID, taskDesc string, taskType TaskType) *WorkRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	record := &WorkRecord{
		ID:        generateID(),
		AgentID:   agentID,
		TaskType:  taskType,
		TaskDesc:  taskDesc,
		Status:    "pending",
		StartedAt: time.Now().UnixMilli(),
	}
	
	t.records[record.ID] = record
	return record
}

// CompleteTask 完成任务
func (t *Tracker) CompleteTask(record *WorkRecord, result TaskResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	record.Status = "completed"
	record.CompletedAt = time.Now().UnixMilli()
	record.TokensInput = result.TokensInput
	record.TokensOutput = result.TokensOutput
	record.CodeLines = result.CodeLines
	record.CodeFiles = result.CodeFiles
	record.WordsWritten = result.WordsWritten
	record.BugsFixed = result.BugsFixed
	record.APICalls = result.APICalls
	record.ErrorsFixed = result.ErrorsFixed
	
	// 生成证明
	record.GenerateProof()
	
	// 更新统计
	t.updateStats(record)
	
	// 持久化
	t.save(record)
}

// FailTask 任务失败
func (t *Tracker) FailTask(record *WorkRecord, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	record.Status = "failed"
	record.CompletedAt = time.Now().UnixMilli()
	record.TaskDesc = record.TaskDesc + " [ERROR: " + errMsg + "]"
	
	t.stats.FailedTasks++
	t.stats.TotalTasks++
	
	t.save(record)
}

// GetStats 获取统计
func (t *Tracker) GetStats() Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	return *t.stats
}

// GetRecords 获取记录
func (t *Tracker) GetRecords(limit int) []*WorkRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	records := make([]*WorkRecord, 0, len(t.records))
	for _, r := range t.records {
		records = append(records, r)
	}
	
	// 按时间排序
	for i := 0; i < len(records)-1; i++ {
		for j := i + 1; j < len(records); j++ {
			if records[j].CompletedAt > records[i].CompletedAt {
				records[i], records[j] = records[j], records[i]
			}
		}
	}
	
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	
	return records
}

func (t *Tracker) updateStats(r *WorkRecord) {
	t.stats.TotalTasks++
	if r.Status == "completed" {
		t.stats.CompletedTasks++
	}
	t.stats.TotalTokens += r.TokensInput + r.TokensOutput
	t.stats.TotalCodeLines += r.CodeLines
	t.stats.TotalWords += r.WordsWritten
	t.stats.BugsFixed += r.BugsFixed
	t.stats.TotalValue += r.CalculateValue()
	t.stats.ByTaskType[string(r.TaskType)]++
}

func (t *Tracker) save(r *WorkRecord) {
	filename := filepath.Join(t.dataDir, fmt.Sprintf("%s.json", r.ID))
	data, _ := json.MarshalIndent(r, "", "  ")
	os.WriteFile(filename, data, 0644)
}

func (t *Tracker) load() {
	files, _ := os.ReadDir(t.dataDir)
	for _, f := range files {
		if f.IsDir() || len(f.Name()) < 5 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(t.dataDir, f.Name()))
		if err != nil {
			continue
		}
		var r WorkRecord
		if json.Unmarshal(data, &r) == nil {
			t.records[r.ID] = &r
			t.updateStats(&r)
		}
	}
}

// TaskResult 任务结果
type TaskResult struct {
	TokensInput   int64
	TokensOutput  int64
	CodeLines     int
	CodeFiles     int
	WordsWritten  int
	BugsFixed     int
	APICalls     int
	ErrorsFixed   int
}

func generateID() string {
	hash := sha256.Sum256([]byte(time.Now().String()))
	return hex.EncodeToString(hash[:])[:16]
}
