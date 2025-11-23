package sync

import (
	"context"
	"fmt"

	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/storage"
)

// OneWaySync handles one-way synchronization logic
type OneWaySync struct {
	source storage.Backend
	dest   storage.Backend
}

// NewOneWaySync creates a new one-way sync handler
func NewOneWaySync(source, dest storage.Backend) *OneWaySync {
	return &OneWaySync{
		source: source,
		dest:   dest,
	}
}

// ShouldSync determines if a file should be synced based on action
func (s *OneWaySync) ShouldSync(action models.Action) bool {
	switch action {
	case models.ActionCopy, models.ActionUpdate:
		return true
	default:
		return false
	}
}

// CopyFile copies a file from source to destination
func (s *OneWaySync) CopyFile(ctx context.Context, relativePath string, size int64) error {
	// Read from source
	reader, err := s.source.Read(ctx, relativePath)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}
	defer reader.Close()

	// Get source metadata to preserve timestamps and permissions
	sourceInfo, err := s.source.Stat(ctx, relativePath)
	if err != nil {
		return fmt.Errorf("failed to get source metadata: %w", err)
	}

	// Write to destination with metadata preservation
	if err := s.dest.Write(ctx, relativePath, reader, size, sourceInfo); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

	return nil
}
