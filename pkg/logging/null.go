package logging

import "context"

// NullLogger is a logger that discards all output
// Used when logging is disabled
type NullLogger struct{}

// NewNullLogger creates a new null logger
func NewNullLogger() *NullLogger {
	return &NullLogger{}
}

// Debug does nothing
func (l *NullLogger) Debug(ctx context.Context, msg string, fields Fields) {}

// Info does nothing
func (l *NullLogger) Info(ctx context.Context, msg string, fields Fields) {}

// Warn does nothing
func (l *NullLogger) Warn(ctx context.Context, msg string, fields Fields) {}

// Error does nothing
func (l *NullLogger) Error(ctx context.Context, msg string, err error, fields Fields) {}

// WithFields returns the same null logger
func (l *NullLogger) WithFields(fields Fields) Logger {
	return l
}

// Close does nothing
func (l *NullLogger) Close() error {
	return nil
}
