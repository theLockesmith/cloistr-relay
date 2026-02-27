package logging

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// Level represents log severity
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// ctxKey is the context key type for logger values
type ctxKey string

const (
	// CtxRequestID is the context key for request/connection ID
	CtxRequestID ctxKey = "request_id"
	// CtxPubkey is the context key for authenticated pubkey
	CtxPubkey ctxKey = "pubkey"
	// CtxRemoteIP is the context key for client IP
	CtxRemoteIP ctxKey = "remote_ip"
)

// Entry represents a structured log entry
type Entry struct {
	Timestamp string                 `json:"ts"`
	Level     Level                  `json:"level"`
	Message   string                 `json:"msg"`
	Component string                 `json:"component,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Pubkey    string                 `json:"pubkey,omitempty"`
	RemoteIP  string                 `json:"remote_ip,omitempty"`
	Duration  float64                `json:"duration_ms,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
}

// Logger provides structured JSON logging
type Logger struct {
	mu        sync.Mutex
	output    io.Writer
	level     Level
	component string
	useJSON   bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Config holds logger configuration
type Config struct {
	Level     string // "debug", "info", "warn", "error"
	UseJSON   bool   // Output JSON format (true for production)
	Component string // Component name (e.g., "relay", "admin")
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Level:     "info",
		UseJSON:   true,
		Component: "relay",
	}
}

// Init initializes the default logger
func Init(cfg *Config) {
	once.Do(func() {
		defaultLogger = New(cfg)
	})
}

// New creates a new logger instance
func New(cfg *Config) *Logger {
	level := LevelInfo
	switch cfg.Level {
	case "debug":
		level = LevelDebug
	case "warn":
		level = LevelWarn
	case "error":
		level = LevelError
	}

	return &Logger{
		output:    os.Stdout,
		level:     level,
		component: cfg.Component,
		useJSON:   cfg.UseJSON,
	}
}

// Default returns the default logger, initializing if needed
func Default() *Logger {
	if defaultLogger == nil {
		Init(DefaultConfig())
	}
	return defaultLogger
}

// WithComponent returns a new logger with a different component name
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		output:    l.output,
		level:     l.level,
		component: component,
		useJSON:   l.useJSON,
	}
}

// shouldLog checks if the given level should be logged
func (l *Logger) shouldLog(level Level) bool {
	levels := map[Level]int{
		LevelDebug: 0,
		LevelInfo:  1,
		LevelWarn:  2,
		LevelError: 3,
	}
	return levels[level] >= levels[l.level]
}

// log writes a log entry
func (l *Logger) log(ctx context.Context, level Level, msg string, fields map[string]interface{}, err error) {
	if !l.shouldLog(level) {
		return
	}

	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   msg,
		Component: l.component,
		Fields:    fields,
	}

	// Extract context values
	if ctx != nil {
		if reqID, ok := ctx.Value(CtxRequestID).(string); ok {
			entry.RequestID = reqID
		}
		if pubkey, ok := ctx.Value(CtxPubkey).(string); ok {
			entry.Pubkey = pubkey
		}
		if ip, ok := ctx.Value(CtxRemoteIP).(string); ok {
			entry.RemoteIP = ip
		}
	}

	if err != nil {
		entry.Error = err.Error()
	}

	// Add caller info for warnings and errors
	if level == LevelWarn || level == LevelError {
		if _, file, line, ok := runtime.Caller(2); ok {
			entry.Caller = formatCaller(file, line)
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.useJSON {
		data, _ := json.Marshal(entry)
		_, _ = l.output.Write(data)
		_, _ = l.output.Write([]byte("\n"))
	} else {
		// Human-readable format for development
		_, _ = l.output.Write([]byte(formatPlaintext(entry)))
	}
}

// formatCaller formats file:line for caller info
func formatCaller(file string, line int) string {
	// Shorten path to just filename
	short := file
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			short = file[i+1:]
			break
		}
	}
	return short + ":" + itoa(line)
}

// formatPlaintext creates human-readable log output
func formatPlaintext(e Entry) string {
	levelColors := map[Level]string{
		LevelDebug: "\033[36m", // Cyan
		LevelInfo:  "\033[32m", // Green
		LevelWarn:  "\033[33m", // Yellow
		LevelError: "\033[31m", // Red
	}
	reset := "\033[0m"

	result := e.Timestamp[:19] + " " + levelColors[e.Level] + string(e.Level) + reset + " "
	if e.Component != "" {
		result += "[" + e.Component + "] "
	}
	result += e.Message

	if e.RequestID != "" {
		result += " req=" + e.RequestID
	}
	if e.Pubkey != "" {
		result += " pubkey=" + e.Pubkey[:8] + "..."
	}
	if e.Error != "" {
		result += " error=" + e.Error
	}
	if e.Caller != "" {
		result += " caller=" + e.Caller
	}

	result += "\n"
	return result
}

// Debug logs at debug level
func (l *Logger) Debug(ctx context.Context, msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, LevelDebug, msg, f, nil)
}

// Info logs at info level
func (l *Logger) Info(ctx context.Context, msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, LevelInfo, msg, f, nil)
}

// Warn logs at warn level
func (l *Logger) Warn(ctx context.Context, msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, LevelWarn, msg, f, nil)
}

// Error logs at error level
func (l *Logger) Error(ctx context.Context, msg string, err error, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(ctx, LevelError, msg, f, err)
}

// WithDuration logs with duration field (for timing operations)
func (l *Logger) WithDuration(ctx context.Context, level Level, msg string, duration time.Duration, fields ...map[string]interface{}) {
	f := make(map[string]interface{})
	if len(fields) > 0 {
		for k, v := range fields[0] {
			f[k] = v
		}
	}
	f["duration_ms"] = float64(duration.Microseconds()) / 1000.0
	l.log(ctx, level, msg, f, nil)
}

// Package-level convenience functions using default logger

// Debug logs at debug level using default logger
func Debug(ctx context.Context, msg string, fields ...map[string]interface{}) {
	Default().Debug(ctx, msg, fields...)
}

// Info logs at info level using default logger
func Info(ctx context.Context, msg string, fields ...map[string]interface{}) {
	Default().Info(ctx, msg, fields...)
}

// Warn logs at warn level using default logger
func Warn(ctx context.Context, msg string, fields ...map[string]interface{}) {
	Default().Warn(ctx, msg, fields...)
}

// Error logs at error level using default logger
func Error(ctx context.Context, msg string, err error, fields ...map[string]interface{}) {
	Default().Error(ctx, msg, err, fields...)
}

// itoa converts int to string
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s string
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// WithRequestID adds a request ID to context
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, CtxRequestID, id)
}

// WithPubkey adds a pubkey to context
func WithPubkey(ctx context.Context, pubkey string) context.Context {
	return context.WithValue(ctx, CtxPubkey, pubkey)
}

// WithRemoteIP adds a remote IP to context
func WithRemoteIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, CtxRemoteIP, ip)
}
