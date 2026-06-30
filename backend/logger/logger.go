package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cebupac/backend/config"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelStrings = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

type Logger struct {
	mu        sync.Mutex
	output    io.Writer
	level     Level
	file      *os.File
	cfg       *config.Config
	entries   []LogEntry
	maxSize   int64
	currentSize int64
}

type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Context   map[string]string `json:"context,omitempty"`
}

var (
	instance *Logger
	once     sync.Once
)

// GetLogger returns the singleton logger instance
func GetLogger() *Logger {
	once.Do(func() {
		cfg := config.GetConfig()
		instance = &Logger{
			cfg:     cfg,
			level:   parseLevel(cfg.Logging.Level),
			entries: make([]LogEntry, 0),
			maxSize: int64(cfg.Logging.MaxSizeMB * 1024 * 1024),
		}
		instance.initOutput()
	})
	return instance
}

func (l *Logger) initOutput() {
	if l.cfg.Logging.OutputFile != "" {
		dir := filepath.Dir(l.cfg.Logging.OutputFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
			l.output = os.Stdout
			return
		}

		file, err := os.OpenFile(l.cfg.Logging.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
			l.output = os.Stdout
			return
		}

		l.file = file
		l.output = io.MultiWriter(os.Stdout, file)
		
		// Get current file size
		if info, err := file.Stat(); err == nil {
			l.currentSize = info.Size()
		}
	} else {
		l.output = os.Stdout
	}
}

func parseLevel(levelStr string) Level {
	switch levelStr {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn":
		return WARN
	case "error":
		return ERROR
	case "fatal":
		return FATAL
	default:
		return INFO
	}
}

func (l *Logger) log(level Level, message string, context map[string]string) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     levelStrings[level],
		Message:   message,
		Context:   context,
	}

	// Format log entry
	logLine := fmt.Sprintf("[%s] [%s] %s",
		entry.Timestamp.Format("2006-01-02 15:04:05"),
		entry.Level,
		entry.Message,
	)

	if len(context) > 0 {
		logLine += " |"
		for k, v := range context {
			logLine += fmt.Sprintf(" %s=%s", k, v)
		}
	}
	logLine += "\n"

	// Write to output
	if l.output != nil {
		l.output.Write([]byte(logLine))
		l.currentSize += int64(len(logLine))
	}

	// Store in memory for retrieval
	l.entries = append(l.entries, entry)
	if len(l.entries) > 1000 {
		l.entries = l.entries[len(l.entries)-1000:]
	}

	// Check if rotation is needed
	if l.currentSize >= l.maxSize {
		l.rotate()
	}
}

func (l *Logger) rotate() {
	if l.file == nil {
		return
	}

	l.file.Close()

	// Rename current file
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.%s", l.cfg.Logging.OutputFile, timestamp)
	os.Rename(l.cfg.Logging.OutputFile, backupPath)

	// Open new file
	file, err := os.OpenFile(l.cfg.Logging.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to rotate log file: %v\n", err)
		return
	}

	l.file = file
	l.output = io.MultiWriter(os.Stdout, file)
	l.currentSize = 0

	// Cleanup old backups
	l.cleanupOldLogs()
}

func (l *Logger) cleanupOldLogs() {
	dir := filepath.Dir(l.cfg.Logging.OutputFile)
	baseName := filepath.Base(l.cfg.Logging.OutputFile)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == filepath.Ext(baseName) {
			logFiles = append(logFiles, filepath.Join(dir, entry.Name()))
		}
	}

	if len(logFiles) <= l.cfg.Logging.MaxBackups {
		return
	}

	// Remove oldest files
	for i := 0; i < len(logFiles)-l.cfg.Logging.MaxBackups; i++ {
		os.Remove(logFiles[i])
	}
}

// Public logging methods
func (l *Logger) Debug(message string, context ...map[string]string) {
	ctx := mergeContext(context...)
	l.log(DEBUG, message, ctx)
}

func (l *Logger) Info(message string, context ...map[string]string) {
	ctx := mergeContext(context...)
	l.log(INFO, message, ctx)
}

func (l *Logger) Warn(message string, context ...map[string]string) {
	ctx := mergeContext(context...)
	l.log(WARN, message, ctx)
}

func (l *Logger) Error(message string, context ...map[string]string) {
	ctx := mergeContext(context...)
	l.log(ERROR, message, ctx)
}

func (l *Logger) Fatal(message string, context ...map[string]string) {
	ctx := mergeContext(context...)
	l.log(FATAL, message, ctx)
	os.Exit(1)
}

func (l *Logger) GetRecentLogs(count int) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Limit count to prevent excessive allocation
	maxCount := 10000
	if count > maxCount {
		count = maxCount
	}
	
	if count > len(l.entries) {
		count = len(l.entries)
	}

	start := len(l.entries) - count
	result := make([]LogEntry, count)
	copy(result, l.entries[start:])
	return result
}

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
	}
}

func mergeContext(contexts ...map[string]string) map[string]string {
	if len(contexts) == 0 {
		return nil
	}
	result := make(map[string]string)
	for _, ctx := range contexts {
		for k, v := range ctx {
			result[k] = v
		}
	}
	return result
}

// Helper functions for common logging patterns
func LogAuth(action, username, result string) {
	GetLogger().Info("Authentication event", map[string]string{
		"action":   action,
		"username": username,
		"result":   result,
	})
}

func LogAPIRequest(method, path, userID string, statusCode int) {
	GetLogger().Info("API request", map[string]string{
		"method":      method,
		"path":        path,
		"user_id":     userID,
		"status_code": fmt.Sprintf("%d", statusCode),
	})
}

func LogPayment(cardLast4, result, message string) {
	GetLogger().Info("Payment processed", map[string]string{
		"card_last4": cardLast4,
		"result":     result,
		"message":    message,
	})
}

func LogSecurity(event, details string) {
	GetLogger().Warn("Security event", map[string]string{
		"event":   event,
		"details": details,
	})
}
