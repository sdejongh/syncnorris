package logging

import (
	"context"
)

// Level represents log severity
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// Fields represents structured log fields
type Fields map[string]interface{}

// Logger defines the interface for logging
// Implementations include JSON, text, and XML loggers
type Logger interface {
	// Debug logs a debug message
	Debug(ctx context.Context, msg string, fields Fields)

	// Info logs an info message
	Info(ctx context.Context, msg string, fields Fields)

	// Warn logs a warning message
	Warn(ctx context.Context, msg string, fields Fields)

	// Error logs an error message
	Error(ctx context.Context, msg string, err error, fields Fields)

	// WithFields returns a logger with additional fields
	WithFields(fields Fields) Logger

	// Close flushes and closes the logger
	Close() error
}
