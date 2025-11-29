package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Format represents the log output format
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// FileLoggerConfig holds configuration for file logging
type FileLoggerConfig struct {
	// Path is the log file path
	Path string
	// Format is the output format (json or text)
	Format Format
	// Level is the minimum log level
	Level Level
	// MaxSize is the maximum size in bytes before rotation (0 = no rotation)
	MaxSize int64
	// MaxBackups is the maximum number of backup files to keep
	MaxBackups int
}

// FileLogger implements Logger interface with file output
type FileLogger struct {
	config     FileLoggerConfig
	file       *os.File
	writer     io.Writer
	mu         sync.Mutex
	fields     Fields
	currentSize int64
}

// NewFileLogger creates a new file logger
func NewFileLogger(config FileLoggerConfig) (*FileLogger, error) {
	// Ensure directory exists
	dir := filepath.Dir(config.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(config.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	return &FileLogger{
		config:      config,
		file:        file,
		writer:      file,
		currentSize: info.Size(),
	}, nil
}

// Debug logs a debug message
func (l *FileLogger) Debug(ctx context.Context, msg string, fields Fields) {
	if l.config.Level <= DebugLevel {
		l.log(DebugLevel, msg, nil, fields)
	}
}

// Info logs an info message
func (l *FileLogger) Info(ctx context.Context, msg string, fields Fields) {
	if l.config.Level <= InfoLevel {
		l.log(InfoLevel, msg, nil, fields)
	}
}

// Warn logs a warning message
func (l *FileLogger) Warn(ctx context.Context, msg string, fields Fields) {
	if l.config.Level <= WarnLevel {
		l.log(WarnLevel, msg, nil, fields)
	}
}

// Error logs an error message
func (l *FileLogger) Error(ctx context.Context, msg string, err error, fields Fields) {
	if l.config.Level <= ErrorLevel {
		l.log(ErrorLevel, msg, err, fields)
	}
}

// WithFields returns a logger with additional fields
func (l *FileLogger) WithFields(fields Fields) Logger {
	newFields := make(Fields)
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}
	return &FileLogger{
		config:      l.config,
		file:        l.file,
		writer:      l.writer,
		fields:      newFields,
		currentSize: l.currentSize,
	}
}

// Close flushes and closes the logger
func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// log writes a log entry
func (l *FileLogger) log(level Level, msg string, err error, fields Fields) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check rotation before writing
	if l.config.MaxSize > 0 && l.currentSize >= l.config.MaxSize {
		l.rotate()
	}

	// Merge fields
	allFields := make(Fields)
	for k, v := range l.fields {
		allFields[k] = v
	}
	for k, v := range fields {
		allFields[k] = v
	}

	var line []byte
	var writeErr error

	if l.config.Format == FormatJSON {
		line, writeErr = l.formatJSON(level, msg, err, allFields)
	} else {
		line, writeErr = l.formatText(level, msg, err, allFields)
	}

	if writeErr != nil {
		return
	}

	n, _ := l.writer.Write(line)
	l.currentSize += int64(n)
}

// formatJSON formats a log entry as JSON
func (l *FileLogger) formatJSON(level Level, msg string, err error, fields Fields) ([]byte, error) {
	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     levelString(level),
		"message":   msg,
	}

	if err != nil {
		entry["error"] = err.Error()
	}

	for k, v := range fields {
		entry[k] = v
	}

	data, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		return nil, jsonErr
	}

	return append(data, '\n'), nil
}

// formatText formats a log entry as plain text
func (l *FileLogger) formatText(level Level, msg string, err error, fields Fields) ([]byte, error) {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	levelStr := levelString(level)

	line := fmt.Sprintf("%s [%s] %s", timestamp, levelStr, msg)

	if err != nil {
		line += fmt.Sprintf(" error=%q", err.Error())
	}

	for k, v := range fields {
		line += fmt.Sprintf(" %s=%v", k, v)
	}

	return []byte(line + "\n"), nil
}

// rotate rotates the log file
func (l *FileLogger) rotate() {
	if l.file == nil {
		return
	}

	// Close current file
	l.file.Close()

	// Rotate existing backups
	for i := l.config.MaxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.config.Path, i)
		newPath := fmt.Sprintf("%s.%d", l.config.Path, i+1)
		os.Rename(oldPath, newPath)
	}

	// Rename current to .1
	os.Rename(l.config.Path, l.config.Path+".1")

	// Remove oldest if exceeds max backups
	if l.config.MaxBackups > 0 {
		oldestPath := fmt.Sprintf("%s.%d", l.config.Path, l.config.MaxBackups+1)
		os.Remove(oldestPath)
	}

	// Open new file
	file, err := os.OpenFile(l.config.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}

	l.file = file
	l.writer = file
	l.currentSize = 0
}

// levelString returns the string representation of a log level
func levelString(level Level) string {
	switch level {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a log level string
func ParseLevel(s string) Level {
	switch s {
	case "debug", "DEBUG":
		return DebugLevel
	case "info", "INFO":
		return InfoLevel
	case "warn", "WARN", "warning", "WARNING":
		return WarnLevel
	case "error", "ERROR":
		return ErrorLevel
	default:
		return InfoLevel
	}
}

// LevelString returns level as string (exported version)
func LevelString(level Level) string {
	return levelString(level)
}
