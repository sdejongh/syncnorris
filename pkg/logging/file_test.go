package logging

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewFileLogger(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:       logPath,
		Format:     FormatText,
		Level:      InfoLevel,
		MaxSize:    1024 * 1024,
		MaxBackups: 3,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}
	defer logger.Close()

	if logger == nil {
		t.Error("NewFileLogger() returned nil")
	}

	// Verify file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Log file was not created")
	}
}

func TestNewFileLogger_CreatesDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Use a nested path that doesn't exist
	logPath := filepath.Join(tempDir, "nested", "dir", "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatText,
		Level:  InfoLevel,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}
	defer logger.Close()

	// Verify directory was created
	dir := filepath.Dir(logPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Log directory was not created")
	}
}

func TestFileLogger_LogLevels(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatText,
		Level:  InfoLevel, // Only INFO and above
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()

	// Debug should be filtered
	logger.Debug(ctx, "debug message", nil)
	// Info should be logged
	logger.Info(ctx, "info message", nil)
	// Warn should be logged
	logger.Warn(ctx, "warn message", nil)
	// Error should be logged
	logger.Error(ctx, "error message", nil, nil)

	logger.Close()

	// Read log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Debug should NOT be present
	if strings.Contains(logContent, "debug message") {
		t.Error("Debug message should be filtered at INFO level")
	}

	// Others should be present
	if !strings.Contains(logContent, "info message") {
		t.Error("Info message should be present")
	}
	if !strings.Contains(logContent, "warn message") {
		t.Error("Warn message should be present")
	}
	if !strings.Contains(logContent, "error message") {
		t.Error("Error message should be present")
	}
}

func TestFileLogger_DebugLevel(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatText,
		Level:  DebugLevel, // Log everything
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()
	logger.Debug(ctx, "debug message", nil)
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "debug message") {
		t.Error("Debug message should be present at DEBUG level")
	}
}

func TestFileLogger_TextFormat(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatText,
		Level:  InfoLevel,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()
	logger.Info(ctx, "test message", Fields{"key": "value", "count": 42})
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)

	// Check format: timestamp [LEVEL] message key=value
	if !strings.Contains(logContent, "[INFO]") {
		t.Error("Log should contain [INFO] level marker")
	}
	if !strings.Contains(logContent, "test message") {
		t.Error("Log should contain the message")
	}
	if !strings.Contains(logContent, "key=value") {
		t.Error("Log should contain the field")
	}
}

func TestFileLogger_JSONFormat(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatJSON,
		Level:  InfoLevel,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()
	logger.Info(ctx, "test message", Fields{"key": "value", "count": 42})
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Parse as JSON
	var entry map[string]interface{}
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	// Check fields
	if entry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", entry["level"])
	}
	if entry["message"] != "test message" {
		t.Errorf("message = %v, want 'test message'", entry["message"])
	}
	if entry["key"] != "value" {
		t.Errorf("key = %v, want 'value'", entry["key"])
	}
	if entry["timestamp"] == nil {
		t.Error("timestamp should be present")
	}
}

func TestFileLogger_ErrorWithErr(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatJSON,
		Level:  InfoLevel,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()
	testErr := &testError{msg: "something went wrong"}
	logger.Error(ctx, "operation failed", testErr, Fields{"operation": "test"})
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	if entry["error"] != "something went wrong" {
		t.Errorf("error = %v, want 'something went wrong'", entry["error"])
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestFileLogger_WithFields(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatJSON,
		Level:  InfoLevel,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()

	// Create logger with base fields
	loggerWithFields := logger.WithFields(Fields{"component": "sync"})

	// Log with additional fields
	loggerWithFields.Info(ctx, "test", Fields{"action": "copy"})
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("Failed to parse JSON log: %v", err)
	}

	// Should have both base and additional fields
	if entry["component"] != "sync" {
		t.Errorf("component = %v, want 'sync'", entry["component"])
	}
	if entry["action"] != "copy" {
		t.Errorf("action = %v, want 'copy'", entry["action"])
	}
}

func TestFileLogger_Rotation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:       logPath,
		Format:     FormatText,
		Level:      InfoLevel,
		MaxSize:    100, // Very small for testing
		MaxBackups: 2,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()

	// Write enough to trigger rotation
	for i := 0; i < 20; i++ {
		logger.Info(ctx, "This is a test message that is long enough to trigger rotation eventually", nil)
	}
	logger.Close()

	// Check that backup files were created
	backup1 := logPath + ".1"
	if _, err := os.Stat(backup1); os.IsNotExist(err) {
		t.Error("Backup file .1 should exist after rotation")
	}

	// Main file should still exist
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Main log file should still exist")
	}
}

func TestNullLogger(t *testing.T) {
	logger := NewNullLogger()
	ctx := context.Background()

	// None of these should panic
	logger.Debug(ctx, "debug", nil)
	logger.Info(ctx, "info", nil)
	logger.Warn(ctx, "warn", nil)
	logger.Error(ctx, "error", nil, nil)

	// WithFields should return a logger
	newLogger := logger.WithFields(Fields{"key": "value"})
	if newLogger == nil {
		t.Error("WithFields should return a logger")
	}

	// Close should not error
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", DebugLevel},
		{"DEBUG", DebugLevel},
		{"info", InfoLevel},
		{"INFO", InfoLevel},
		{"warn", WarnLevel},
		{"WARN", WarnLevel},
		{"warning", WarnLevel},
		{"WARNING", WarnLevel},
		{"error", ErrorLevel},
		{"ERROR", ErrorLevel},
		{"unknown", InfoLevel}, // Default
		{"", InfoLevel},        // Default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := LevelString(tt.level)
			if result != tt.expected {
				t.Errorf("LevelString(%v) = %q, want %q", tt.level, result, tt.expected)
			}
		})
	}
}

func TestFileLogger_ConcurrentWrites(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	config := FileLoggerConfig{
		Path:   logPath,
		Format: FormatText,
		Level:  InfoLevel,
	}

	logger, err := NewFileLogger(config)
	if err != nil {
		t.Fatalf("NewFileLogger() error = %v", err)
	}

	ctx := context.Background()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				logger.Info(ctx, "concurrent message", Fields{"goroutine": id, "iteration": j})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent writes")
		}
	}

	logger.Close()

	// Verify file is readable and has content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 1000 {
		t.Errorf("Expected 1000 log lines, got %d", len(lines))
	}
}
