package sync

import (
	"context"
	"fmt"

	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/logging"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/output"
	"github.com/sdejongh/syncnorris/pkg/storage"
)

// Engine orchestrates the sync operation
type Engine struct {
	source         storage.Backend
	dest           storage.Backend
	comparator     compare.Comparator
	formatter      output.Formatter
	logger         logging.Logger
	operation      *models.SyncOperation
	pipelineConfig PipelineConfig
}

// NewEngine creates a new sync engine
func NewEngine(
	source, dest storage.Backend,
	comparator compare.Comparator,
	formatter output.Formatter,
	logger logging.Logger,
	operation *models.SyncOperation,
) *Engine {
	return &Engine{
		source:         source,
		dest:           dest,
		comparator:     comparator,
		formatter:      formatter,
		logger:         logger,
		operation:      operation,
		pipelineConfig: DefaultPipelineConfig(),
	}
}

// SetPipelineConfig overrides the default pipeline config. Must be called
// before Run.
func (e *Engine) SetPipelineConfig(cfg PipelineConfig) {
	e.pipelineConfig = cfg
}

// Run executes the sync operation using the pipeline architecture
func (e *Engine) Run(ctx context.Context) (*models.SyncReport, error) {
	// Use the new pipeline-based approach for one-way sync
	if e.operation.Mode == models.ModeOneWay {
		return e.runPipeline(ctx)
	}

	// Bidirectional sync
	if e.operation.Mode == models.ModeBidirectional {
		return e.runBidirectional(ctx)
	}

	return nil, fmt.Errorf("unknown sync mode: %s", e.operation.Mode)
}

// runPipeline executes sync using the producer-consumer pipeline
func (e *Engine) runPipeline(ctx context.Context) (*models.SyncReport, error) {
	cfg := e.pipelineConfig
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 5
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 1000
	}

	pipeline := NewPipeline(
		e.source,
		e.dest,
		e.comparator,
		e.formatter,
		e.logger,
		e.operation,
		cfg,
	)

	return pipeline.Run(ctx)
}

// runBidirectional executes sync using the bidirectional pipeline
func (e *Engine) runBidirectional(ctx context.Context) (*models.SyncReport, error) {
	cfg := e.pipelineConfig
	if cfg.MaxWorkers == 0 {
		cfg.MaxWorkers = 5
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 1000
	}

	pipeline := NewBidirectionalPipeline(
		e.source,
		e.dest,
		e.comparator,
		e.formatter,
		e.logger,
		e.operation,
		cfg,
	)

	return pipeline.Run(ctx)
}
