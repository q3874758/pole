package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Loader 配置加载器
type Loader struct {
	mu      sync.RWMutex
	configs map[string]interface{}
}

// 全局配置加载器
var globalLoader = &Loader{
	configs: make(map[string]interface{}),
}

// GetLoader 获取全局配置加载器
func GetLoader() *Loader {
	return globalLoader
}

// Load 加载 JSON 配置文件
func (l *Loader) Load(path string, dest interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	l.mu.Lock()
	l.configs[path] = dest
	l.mu.Unlock()

	return nil
}

// LoadOrCreate 加载或创建默认配置
func (l *Loader) LoadOrCreate(path string, dest interface{}, defaults interface{}) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 创建默认配置
		data, err := json.MarshalIndent(defaults, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal default config: %w", err)
		}

		// 确保目录存在
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write default config: %w", err)
		}

		// 使用默认值
		if dest != defaults {
			data, _ := json.Marshal(defaults)
			json.Unmarshal(data, dest)
		}
		return nil
	}

	return l.Load(path, dest)
}

// Save 保存配置到文件
func (l *Loader) Save(path string, src interface{}) error {
	data, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	l.mu.Lock()
	l.configs[path] = src
	l.mu.Unlock()

	return nil
}

// Get 获取已加载的配置
func (l *Loader) Get(path string) (interface{}, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	c, ok := l.configs[path]
	return c, ok
}

// Reload 重新加载配置
func (l *Loader) Reload(path string, dest interface{}) error {
	return l.Load(path, dest)
}

// ==================== 环境变量支持 ====================

// EnvLoader 环境变量加载器
type EnvLoader struct {
	prefix string
}

// NewEnvLoader 创建环境变量加载器
func NewEnvLoader(prefix string) *EnvLoader {
	if prefix == "" {
		prefix = "POLE"
	}
	return &EnvLoader{prefix: prefix}
}

// Load 从环境变量加载配置
func (e *EnvLoader) Load(v interface{}) error {
	// 简化实现：支持简单的环境变量覆盖
	// 实际可以通过 struct tag 自动映射
	return nil
}

// GetEnv 获取环境变量（支持默认值）
func GetEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// GetEnvInt 获取整型环境变量
func GetEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return defaultValue
}

// GetEnvBool 获取布尔环境变量
func GetEnvBool(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		return strings.ToLower(v) == "true" || v == "1"
	}
	return defaultValue
}

// ==================== 验证器 ====================

// Validator 配置验证器
type Validator interface {
	Validate() error
}

// ValidateStruct 验证结构体
func ValidateStruct(v interface{}) error {
	// 简化实现
	// 实际可以使用 go-playground/validator 等库
	return nil
}

// Required 检查必需字段
func Required(v string) error {
	if v == "" {
		return fmt.Errorf("required field is empty")
	}
	return nil
}

// Range 检查范围
func Range(v, min, max int) error {
	if v < min || v > max {
		return fmt.Errorf("value %d out of range [%d, %d]", v, min, max)
	}
	return nil
}

// ==================== 全局便捷方法 ====================

// Load 加载 JSON 配置
func Load(path string, dest interface{}) error {
	return globalLoader.Load(path, dest)
}

// LoadOrCreate 加载或创建默认配置
func LoadOrCreate(path string, dest interface{}, defaults interface{}) error {
	return globalLoader.LoadOrCreate(path, dest, defaults)
}

// Save 保存配置
func Save(path string, src interface{}) error {
	return globalLoader.Save(path, src)
}

// GetEnv 获取环境变量
var Get = GetEnv
