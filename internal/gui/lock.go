//go:build !nogui

package gui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// InstanceLock prevents multiple concurrent GUI instances.
type InstanceLock struct {
	l *flock.Flock
}

// AcquireInstanceLock tries to take a non-blocking exclusive lock on path.
// Returns an error if the lock is already held.
func AcquireInstanceLock(path string) (*InstanceLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	l := flock.New(path)
	locked, err := l.TryLock()
	if err != nil {
		return nil, fmt.Errorf("try lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("another instance is already running")
	}
	return &InstanceLock{l: l}, nil
}

// Release unlocks and releases the instance lock.
func (i *InstanceLock) Release() error {
	return i.l.Unlock()
}
