package job

import (
	"testing"
	"time"
)

func TestJobStoreCreateAndGet(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)

	opts := Options{Mode: "oneway", Comparator: "hash", Workers: 5}
	j, err := store.Create("/src", "/dest", opts)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if j.ID == "" {
		t.Error("expected non-empty ID")
	}
	if j.Status != StatusRunning {
		t.Errorf("expected status running, got %q", j.Status)
	}

	got, err := store.Get(j.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != j.ID || got.Source != "/src" {
		t.Errorf("get returned wrong job")
	}
}

func TestJobStoreListAndDelete(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)

	_, _ = store.Create("/src1", "/dest1", Options{Mode: "oneway"})
	j2, _ := store.Create("/src2", "/dest2", Options{Mode: "oneway"})

	jobs, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}

	if err := store.Delete(j2.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	jobs, _ = store.List()
	if len(jobs) != 1 {
		t.Errorf("expected 1 job after delete, got %d", len(jobs))
	}
}

func TestJobStoreActiveNoJob(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)

	j, err := store.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if j != nil {
		t.Error("expected nil active job")
	}
}

func TestJobStoreActiveReturnsPaused(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)

	j, _ := store.Create("/src", "/dest", Options{Mode: "oneway"})
	if err := store.SetStatus(j.ID, StatusPaused); err != nil {
		t.Fatal(err)
	}

	active, err := store.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active == nil || active.ID != j.ID {
		t.Error("expected paused job to be returned as active")
	}
	if active.Status != StatusPaused {
		t.Errorf("expected status paused, got %q", active.Status)
	}
}

func TestJobStoreActiveDetectsCrash(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)
	store.StaleThreshold = 1 * time.Second

	j, _ := store.Create("/src", "/dest", Options{Mode: "oneway"})
	// Simulate heartbeat older than stale threshold
	j.Heartbeat = time.Now().Add(-10 * time.Second)
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}

	active, err := store.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active == nil {
		t.Fatal("expected crashed job to be returned")
	}
	// Active() should normalize crashed jobs to paused
	if active.Status != StatusPaused {
		t.Errorf("expected crashed job normalized to paused, got %q", active.Status)
	}
}

func TestJobStoreActiveIgnoresFreshRunning(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)

	j, _ := store.Create("/src", "/dest", Options{Mode: "oneway"})
	_ = j
	// Job has fresh heartbeat (just created); Active() should treat as
	// currently-running and NOT offer for recovery (another instance logic
	// is handled by the app lock, not here).
	active, err := store.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Errorf("expected nil for fresh running job, got %v", active)
	}
}
