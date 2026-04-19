//go:build !nogui

package gui

import (
	"path/filepath"
	"testing"
)

func TestInstanceLockAcquireAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.lock")
	lock, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if lock == nil {
		t.Fatal("nil lock")
	}

	// Second acquire on same path must fail
	lock2, err := AcquireInstanceLock(path)
	if err == nil {
		_ = lock2.Release()
		t.Fatal("expected second acquire to fail")
	}

	if err := lock.Release(); err != nil {
		t.Errorf("release: %v", err)
	}

	// After release, acquire again must succeed
	lock3, err := AcquireInstanceLock(path)
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	_ = lock3.Release()
}
