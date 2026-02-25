package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	PanicLevel
)

var levelNames = []string{"DEBUG", "INFO", "WARN", "ERROR", "PANIC"}

// Config 日志配置
type Config struct {
	Level      Level     // 日志级别
	Output     io.Writer // 输出目标
	TimeFormat string    // 时间格式
	EnableColor bool     // 启用颜色
}

func DefaultConfig() *Config {
	return &Config{
		Level:      InfoLevel,
		Output:     os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
		EnableColor: true,
	}
}

// Logger 日志记录器
type Logger struct {
	config *Config
	logger *log.Logger
	mu     sync.Mutex
}

// 全局日志器
var (
	defaultLogger *Logger
	once          sync.Once
)

// GetLogger 获取全局日志器
func GetLogger() *Logger {
	once.Do(func() {
		defaultLogger = New(DefaultConfig())
	})
	return defaultLogger
}

// New 创建日志器
func New(config *Config) *Logger {
	if config == nil {
		config = DefaultConfig()
	}
	if config.Output == nil {
		config.Output = os.Stdout
	}

	return &Logger{
		config: config,
		logger: log.New(config.Output, "", 0),
	}
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.Level = level
}

// SetOutput 设置输出
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.Output = w
	l.logger.SetOutput(w)
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DebugLevel, format, args...)
}

// Info 信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(InfoLevel, format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WarnLevel, format, args...)
}

// Error 错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ErrorLevel, format, args...)
}

// Panic 致命错误日志
func (l *Logger) Panic(format string, args ...interface{}) {
	l.log(PanicLevel, format, args...)
}

// log 内部日志方法
func (l *Logger) log(level Level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.config.Level {
		return
	}

	// 格式化时间
	timestamp := time.Now().Format(l.config.TimeFormat)

	// 格式化消息
	var msg string
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	} else {
		msg = format
	}

	// 构建日志行
	levelName := levelNames[level]
	logLine := fmt.Sprintf("[%s] [%s] %s", timestamp, levelName, msg)

	// 颜色（仅终端）
	if l.config.EnableColor {
		logLine = l.colorize(level, logLine)
	}

	l.logger.Println(logLine)
}

// colorize 添加颜色
func (l *Logger) colorize(level Level, msg string) string {
	// 简单实现，实际可根据终端支持情况增强
	return msg
}

// ==================== 全局便捷方法 ====================

// Debug 调试日志
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

// Info 信息日志
func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

// Warn 警告日志
func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

// Error 错误日志
func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

// Panic 致命错误日志
func Panic(format string, args ...interface{}) {
	GetLogger().Panic(format, args...)
}

// SetLevel 设置全局日志级别
func SetLevel(level Level) {
	GetLogger().SetLevel(level)
}
