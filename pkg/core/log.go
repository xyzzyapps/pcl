package core

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LogLevel represents severity of log messages.
type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelNone
)

func (l LogLevel) String() string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "NONE"
	}
}

// Logger provides structured profuse logging.
type Logger struct {
	mu     sync.Mutex
	level  LogLevel
	writer io.Writer
}

var defaultLogger = &Logger{
	level:  LevelInfo,
	writer: os.Stderr,
}

// SetGlobalLogLevel sets the global log level.
func SetGlobalLogLevel(level LogLevel) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.level = level
}

// SetGlobalLogWriter sets the global log output writer.
func SetGlobalLogWriter(w io.Writer) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.writer = w
}

// NewLogger creates a dedicated logger instance.
func NewLogger(level LogLevel, w io.Writer) *Logger {
	if w == nil {
		w = os.Stderr
	}
	return &Logger{
		level:  level,
		writer: w,
	}
}

func (l *Logger) logf(level LogLevel, tag string, format string, args ...interface{}) {
	if l == nil || level < l.level || l.level == LevelNone {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	if tag != "" {
		fmt.Fprintf(l.writer, "[%s] [%-5s] [%s] %s\n", ts, level.String(), tag, msg)
	} else {
		fmt.Fprintf(l.writer, "[%s] [%-5s] %s\n", ts, level.String(), msg)
	}
}

// Log methods on Logger
func (l *Logger) Trace(tag string, format string, args ...interface{}) {
	l.logf(LevelTrace, tag, format, args...)
}

func (l *Logger) Debug(tag string, format string, args ...interface{}) {
	l.logf(LevelDebug, tag, format, args...)
}

func (l *Logger) Info(tag string, format string, args ...interface{}) {
	l.logf(LevelInfo, tag, format, args...)
}

func (l *Logger) Warn(tag string, format string, args ...interface{}) {
	l.logf(LevelWarn, tag, format, args...)
}

func (l *Logger) Error(tag string, format string, args ...interface{}) {
	l.logf(LevelError, tag, format, args...)
}

// Global logger helper functions
func LogTrace(tag string, format string, args ...interface{}) {
	defaultLogger.Trace(tag, format, args...)
}

func LogDebug(tag string, format string, args ...interface{}) {
	defaultLogger.Debug(tag, format, args...)
}

func LogInfo(tag string, format string, args ...interface{}) {
	defaultLogger.Info(tag, format, args...)
}

func LogWarn(tag string, format string, args ...interface{}) {
	defaultLogger.Warn(tag, format, args...)
}

func LogError(tag string, format string, args ...interface{}) {
	defaultLogger.Error(tag, format, args...)
}
