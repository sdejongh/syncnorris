package config

import (
	"github.com/sdejongh/syncnorris/pkg/models"
)

// Config represents the application configuration
type Config struct {
	Sync        SyncConfig        `yaml:"sync"`
	Performance PerformanceConfig `yaml:"performance"`
	Output      OutputConfig      `yaml:"output"`
	Logging     LoggingConfig     `yaml:"logging"`
	Exclude     []string          `yaml:"exclude"`
}

// SyncConfig holds sync-related settings
type SyncConfig struct {
	Mode               models.SyncMode               `yaml:"mode"`
	Comparison         models.ComparisonMethod       `yaml:"comparison"`
	ConflictResolution models.ConflictResolution     `yaml:"conflict_resolution"`
}

// PerformanceConfig holds performance-related settings
type PerformanceConfig struct {
	MaxWorkers     int   `yaml:"max_workers"`
	BufferSize     int   `yaml:"buffer_size"`
	BandwidthLimit int64 `yaml:"bandwidth_limit"`
}

// OutputConfig holds output-related settings
type OutputConfig struct {
	Format   string `yaml:"format"`   // "human" or "json"
	Progress bool   `yaml:"progress"` // Show progress bars
	Quiet    bool   `yaml:"quiet"`    // Suppress non-error output
}

// LoggingConfig holds logging-related settings
type LoggingConfig struct {
	Enabled bool   `yaml:"enabled"`
	Format  string `yaml:"format"` // "json", "text", or "xml"
	Level   string `yaml:"level"`  // "debug", "info", "warn", "error"
	File    string `yaml:"file"`   // Log file path (empty = stderr)
}

// Default returns the default configuration
func Default() *Config {
	return &Config{
		Sync: SyncConfig{
			Mode:               models.ModeOneWay,
			Comparison:         models.CompareHash,
			ConflictResolution: models.ConflictAsk,
		},
		Performance: PerformanceConfig{
			MaxWorkers:     5,
			BufferSize:     65536,
			BandwidthLimit: 0,
		},
		Output: OutputConfig{
			Format:   "human",
			Progress: true,
			Quiet:    false,
		},
		Logging: LoggingConfig{
			Enabled: true,
			Format:  "json",
			Level:   "info",
			File:    "",
		},
		Exclude: []string{
			"*.tmp",
			".git/",
			"node_modules/",
		},
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Performance.MaxWorkers < 1 {
		return &models.ValidationError{
			Field:   "performance.max_workers",
			Message: "must be at least 1",
		}
	}

	if c.Performance.BufferSize < 1024 {
		return &models.ValidationError{
			Field:   "performance.buffer_size",
			Message: "must be at least 1024 bytes",
		}
	}

	validFormats := map[string]bool{"human": true, "json": true}
	if !validFormats[c.Output.Format] {
		return &models.ValidationError{
			Field:   "output.format",
			Message: "must be 'human' or 'json'",
		}
	}

	validLogFormats := map[string]bool{"json": true, "text": true, "xml": true}
	if !validLogFormats[c.Logging.Format] {
		return &models.ValidationError{
			Field:   "logging.format",
			Message: "must be 'json', 'text', or 'xml'",
		}
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Logging.Level] {
		return &models.ValidationError{
			Field:   "logging.level",
			Message: "must be 'debug', 'info', 'warn', or 'error'",
		}
	}

	return nil
}
