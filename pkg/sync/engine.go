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
	source     storage.Backend
	dest       storage.Backend
	comparator compare.Comparator
	formatter  output.Formatter
	logger     logging.Logger
	operation  *models.SyncOperation
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
		source:     source,
		dest:       dest,
		comparator: comparator,
		formatter:  formatter,
		logger:     logger,
		operation:  operation,
	}
}

// Run executes the sync operation using the pipeline architecture
func (e *Engine) Run(ctx context.Context) (*models.SyncReport, error) {
	// Use the new pipeline-based approach for one-way sync
	if e.operation.Mode == models.ModeOneWay {
		return e.runPipeline(ctx)
	}

	// Bidirectional sync not yet implemented
	return nil, fmt.Errorf("bidirectional sync not yet implemented")
}

// runPipeline executes sync using the producer-consumer pipeline
func (e *Engine) runPipeline(ctx context.Context) (*models.SyncReport, error) {
	config := PipelineConfig{
		MaxWorkers: e.operation.MaxWorkers,
		QueueSize:  1000,
	}

	pipeline := NewPipeline(
		e.source,
		e.dest,
		e.comparator,
		e.formatter,
		e.logger,
		e.operation,
		config,
	)

	return pipeline.Run(ctx)
}
