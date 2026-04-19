//go:build !nogui

package gui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/logging"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/storage"
	"github.com/sdejongh/syncnorris/pkg/sync"
	"github.com/sdejongh/syncnorris/pkg/sync/job"
)

// RunOptions holds the configuration gathered from the GUI widgets
type RunOptions struct {
	Source     string
	Dest       string
	Mode       string // "oneway" or "bidirectional"
	Comparison string // "hash", "md5", "binary", "namesize", "timestamp"
	Conflict   string // "newer", "source-wins", "dest-wins", "both"
	Workers    int
	BufferSize int
	Excludes   string // comma-separated patterns
	DryRun     bool
	Delete     bool
	CreateDest bool
	Stateful   bool

	// Optional job tracking for pause/resume support (nil = disabled).
	// These are set by the state machine in app.go before calling runSync.
	Job       *job.Job
	JobStore  *job.JobStore
	PauseSoft chan struct{}
	PauseHard chan struct{}
}

// buildOperation creates a SyncOperation from RunOptions
func buildOperation(opts RunOptions) (*models.SyncOperation, error) {
	excludes := parseExcludes(opts.Excludes)

	workers := opts.Workers
	if workers < 1 {
		workers = 5
	}
	bufSize := opts.BufferSize
	if bufSize < 1024 {
		bufSize = 65536
	}

	op := &models.SyncOperation{
		ID:                 uuid.New().String(),
		SourcePath:         opts.Source,
		DestPath:           opts.Dest,
		Mode:               models.SyncMode(opts.Mode),
		ComparisonMethod:   models.ComparisonMethod(opts.Comparison),
		ConflictResolution: models.ConflictResolution(opts.Conflict),
		ExcludePatterns:    excludes,
		DryRun:             opts.DryRun,
		DeleteOrphans:      opts.Delete,
		MaxWorkers:         workers,
		BufferSize:         bufSize,
		Stateful:           opts.Stateful,
		CreatedAt:          time.Now(),
	}

	if err := op.Validate(); err != nil {
		return nil, err
	}
	return op, nil
}

// runSync runs a sync or compare operation in the background.
// It sends UIEvents via the formatter channel.
func runSync(ctx context.Context, opts RunOptions, events chan<- UIEvent) {
	op, err := buildOperation(opts)
	if err != nil {
		events <- UIEvent{Type: eventError, Error: fmt.Errorf("invalid configuration: %w", err)}
		return
	}

	// Create storage backends
	source, err := storage.NewLocal(op.SourcePath)
	if err != nil {
		events <- UIEvent{Type: eventError, Error: fmt.Errorf("source: %w", err)}
		return
	}
	defer source.Close()

	dest, err := storage.NewLocal(op.DestPath)
	if err != nil {
		events <- UIEvent{Type: eventError, Error: fmt.Errorf("destination: %w", err)}
		return
	}
	defer dest.Close()

	// Create comparator
	comparator, err := buildComparator(op.ComparisonMethod, op.BufferSize)
	if err != nil {
		events <- UIEvent{Type: eventError, Error: err}
		return
	}

	// Create formatter that sends to GUI
	formatter := NewGUIFormatter(events)

	// Create engine, apply pipeline config, and run
	engine := sync.NewEngine(source, dest, comparator, formatter, logging.NewNullLogger(), op)
	cfg := sync.DefaultPipelineConfig()
	cfg.Job = opts.Job
	cfg.JobStore = opts.JobStore
	cfg.PauseSoft = opts.PauseSoft
	cfg.PauseHard = opts.PauseHard
	engine.SetPipelineConfig(cfg)
	report, err := engine.Run(ctx)
	if err != nil {
		events <- UIEvent{Type: eventError, Error: err}
		return
	}

	// Report is also sent by formatter.Complete, but in case the pipeline
	// skips it (e.g. on cancellation), send it explicitly
	if report != nil {
		events <- UIEvent{Type: eventComplete, Report: report}
	}
}

func buildComparator(method models.ComparisonMethod, bufSize int) (compare.Comparator, error) {
	switch method {
	case models.CompareNameSize:
		return compare.NewCompositeComparator(false, bufSize), nil
	case models.CompareHash:
		return compare.NewCompositeComparator(true, bufSize), nil
	case models.CompareMD5:
		return compare.NewMD5Comparator(bufSize), nil
	case models.CompareBinary:
		return compare.NewBinaryComparator(bufSize), nil
	case models.CompareTimestamp:
		return compare.NewTimestampComparator(), nil
	default:
		return nil, fmt.Errorf("unsupported comparison method: %s", method)
	}
}

func parseExcludes(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
