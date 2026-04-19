# GUI Pause/Resume Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add pause/resume capability for one-way sync jobs in the GUI, surviving crashes and app restarts, with completion-log-based fast resume.

**Architecture:** New `pkg/sync/job` package handles job persistence (JSON metadata + TSV completion log) and crash detection via heartbeat. Pipeline accepts an optional `*Job`, skips already-completed files via stat+mtime check, and appends to the completion log as work progresses. GUI detects interrupted jobs on startup via banner and exposes soft/hard pause buttons.

**Tech Stack:** Go 1.24, Gio UI, `github.com/gofrs/flock` (new dep), existing `pkg/storage`/`pkg/sync`/`internal/gui` packages.

**Reference spec:** [2026-04-19-gui-pause-resume-design.md](../specs/2026-04-19-gui-pause-resume-design.md)

---

## File Structure

**New files:**
- `pkg/sync/job/job.go` — `Job` type, status, JSON serialization
- `pkg/sync/job/job_test.go` — unit tests
- `pkg/sync/job/completion.go` — `CompletionLog` append-only TSV
- `pkg/sync/job/completion_test.go`
- `pkg/sync/job/store.go` — `JobStore` create/list/get/delete/active
- `pkg/sync/job/store_test.go`
- `pkg/sync/job/heartbeat.go` — heartbeat ticker
- `pkg/sync/job/heartbeat_test.go`
- `internal/gui/lock.go` — instance lock wrapper around `gofrs/flock`
- `internal/gui/lock_test.go`
- `internal/gui/banner.go` — resume banner layout

**Modified files:**
- `pkg/storage/local.go:104-146` — atomic write via `.partial` + rename
- `pkg/storage/local_test.go` — new tests for atomic write
- `pkg/sync/pipeline.go:22-108` — Pipeline gets optional `*job.Job`, `PauseSoft`/`PauseHard` channels
- `pkg/sync/pipeline.go:278-334` — skip completed files in `scanSourceAndQueue`
- `pkg/sync/pipeline.go:628-650` — append to completion log after successful copy/update
- `pkg/sync/engine.go` — pass `Job` through from operation config
- `internal/gui/runner.go` — thread `*job.Job` through `RunOptions` and `runSync`
- `internal/gui/app.go` — state machine (Idle/Running/Paused), pause/resume/abandon wiring, startup detection
- `internal/gui/layout.go` — replace Run/Cancel with state-aware buttons, integrate banner
- `go.mod` — add `github.com/gofrs/flock`
- `CLAUDE.md`, `README.md`, `CHANGELOG.md` — documentation

---

## Phase 1: Foundation — Job Persistence Package

### Task 1: Create `Job` type with JSON roundtrip

**Files:**
- Create: `pkg/sync/job/job.go`
- Create: `pkg/sync/job/job_test.go`

- [ ] **Step 1: Write the failing test**

`pkg/sync/job/job_test.go`:
```go
package job

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJobJSONRoundtrip(t *testing.T) {
	original := &Job{
		ID:          "a3f9b2e1",
		Source:      "/src",
		Destination: "/dest",
		Options: Options{
			Mode:            "oneway",
			Comparator:      "hash",
			Workers:         5,
			BandwidthLimit:  0,
			DeleteOrphans:   false,
			ExcludePatterns: []string{".git", "*.tmp"},
		},
		Status:        StatusRunning,
		CreatedAt:     time.Date(2026, 4, 19, 10, 15, 0, 0, time.UTC),
		Heartbeat:     time.Date(2026, 4, 19, 10, 22, 34, 0, time.UTC),
		CompletionLog: "/tmp/a3f9b2e1.log",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Status != StatusRunning {
		t.Errorf("Status mismatch: got %q", decoded.Status)
	}
	if len(decoded.Options.ExcludePatterns) != 2 {
		t.Errorf("ExcludePatterns len mismatch: got %d", len(decoded.Options.ExcludePatterns))
	}
	if !decoded.Heartbeat.Equal(original.Heartbeat) {
		t.Errorf("Heartbeat mismatch")
	}
}

func TestJobStatusConstants(t *testing.T) {
	if StatusRunning != "running" {
		t.Errorf("StatusRunning got %q", StatusRunning)
	}
	if StatusPaused != "paused" {
		t.Errorf("StatusPaused got %q", StatusPaused)
	}
	if StatusCompleted != "completed" {
		t.Errorf("StatusCompleted got %q", StatusCompleted)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sync/job/ -run TestJob -v`
Expected: FAIL (package does not exist)

- [ ] **Step 3: Implement the `Job` type**

`pkg/sync/job/job.go`:
```go
// Package job provides persistence and lifecycle management for sync jobs,
// enabling pause/resume of one-way syncs across app restarts.
package job

import "time"

type Status string

const (
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
)

// Options captures the user-selected run configuration we need to
// restore on resume. Mirrors internal/gui.RunOptions but lives here
// to avoid a dependency on the GUI package.
type Options struct {
	Mode            string   `json:"mode"`
	Comparator      string   `json:"comparator"`
	Workers         int      `json:"workers"`
	BufferSize      int      `json:"buffer_size"`
	BandwidthLimit  int64    `json:"bandwidth_limit"`
	DeleteOrphans   bool     `json:"delete_orphans"`
	DryRun          bool     `json:"dry_run"`
	ExcludePatterns []string `json:"exclude_patterns"`
}

type Job struct {
	ID            string    `json:"id"`
	Source        string    `json:"source"`
	Destination   string    `json:"destination"`
	Options       Options   `json:"options"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Heartbeat     time.Time `json:"heartbeat"`
	CompletionLog string    `json:"completion_log"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sync/job/ -run TestJob -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/job/job.go pkg/sync/job/job_test.go
git commit -m "feat(job): add Job type with JSON serialization"
```

---

### Task 2: Implement `CompletionLog` append-only TSV

**Files:**
- Create: `pkg/sync/job/completion.go`
- Create: `pkg/sync/job/completion_test.go`

- [ ] **Step 1: Write the failing tests**

`pkg/sync/job/completion_test.go`:
```go
package job

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompletionLogAppendAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	cl, err := NewCompletionLog(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	entries := []CompletionEntry{
		{Path: "a/b.txt", Size: 100, MTime: time.Unix(1000, 0), Hash: "abc123"},
		{Path: "c/d.png", Size: 2048, MTime: time.Unix(2000, 0), Hash: ""},
		{Path: "weird\ttab.txt", Size: 50, MTime: time.Unix(3000, 0), Hash: "-"}, // tab in path must be rejected or escaped
	}

	if err := cl.Append(entries[0]); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := cl.Append(entries[1]); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	// Tab in path: expect an error to keep format simple
	if err := cl.Append(entries[2]); err == nil {
		t.Error("expected error for tab in path")
	}

	if err := cl.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	loaded, err := LoadCompletionLog(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if loaded["a/b.txt"].Size != 100 {
		t.Errorf("size mismatch")
	}
	if loaded["c/d.png"].Hash != "" {
		t.Errorf("expected empty hash for dash marker, got %q", loaded["c/d.png"].Hash)
	}
}

func TestCompletionLogTolerateMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.log")

	// Handcraft a file with one good and two bad lines
	content := "good/file.txt\t100\t1000000000\t-\nnot_enough_fields\nalso/good.txt\t200\t2000000000\tdeadbeef\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCompletionLog(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 good entries, got %d", len(loaded))
	}
}

func TestCompletionLogMissingFileReturnsEmpty(t *testing.T) {
	loaded, err := LoadCompletionLog(filepath.Join(t.TempDir(), "nope.log"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected empty map, got %d entries", len(loaded))
	}
}

func TestCompletionLogBatchFlush(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "batch.log")

	cl, err := NewCompletionLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	for i := 0; i < 150; i++ {
		if err := cl.Append(CompletionEntry{
			Path:  filepath.Join("f", string(rune('a'+i%26))),
			Size:  int64(i),
			MTime: time.Unix(int64(i), 0),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Force flush
	if err := cl.Flush(); err != nil {
		t.Fatal(err)
	}

	// Read raw, count lines
	data, _ := os.ReadFile(path)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines < 150 {
		t.Errorf("expected at least 150 lines flushed, got %d", lines)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sync/job/ -run TestCompletionLog -v`
Expected: FAIL (types not defined)

- [ ] **Step 3: Implement `CompletionLog`**

`pkg/sync/job/completion.go`:
```go
package job

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CompletionEntry struct {
	Path  string
	Size  int64
	MTime time.Time
	Hash  string // empty if no hash is available
}

// CompletionLog is an append-only TSV log of completed files. Writes are
// buffered; callers should call Flush() or Close() to ensure durability.
// Format per line: <path>\t<size>\t<mtime_unix_nanos>\t<hash_or_dash>\n
type CompletionLog struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	buf      *bufio.Writer
	pending  int
	maxBatch int
}

const completionBatchSize = 100

func NewCompletionLog(path string) (*CompletionLog, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open completion log: %w", err)
	}
	return &CompletionLog{
		path:     path,
		file:     f,
		buf:      bufio.NewWriter(f),
		maxBatch: completionBatchSize,
	}, nil
}

func (c *CompletionLog) Append(e CompletionEntry) error {
	if strings.ContainsAny(e.Path, "\t\n") {
		return fmt.Errorf("path contains tab or newline: %q", e.Path)
	}
	hash := e.Hash
	if hash == "" {
		hash = "-"
	}
	line := fmt.Sprintf("%s\t%d\t%d\t%s\n", e.Path, e.Size, e.MTime.UnixNano(), hash)

	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.buf.WriteString(line); err != nil {
		return err
	}
	c.pending++
	if c.pending >= c.maxBatch {
		return c.flushLocked()
	}
	return nil
}

func (c *CompletionLog) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushLocked()
}

func (c *CompletionLog) flushLocked() error {
	if err := c.buf.Flush(); err != nil {
		return err
	}
	if err := c.file.Sync(); err != nil {
		return err
	}
	c.pending = 0
	return nil
}

func (c *CompletionLog) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.file == nil {
		return nil
	}
	if err := c.buf.Flush(); err != nil {
		_ = c.file.Close()
		c.file = nil
		return err
	}
	err := c.file.Close()
	c.file = nil
	return err
}

// LoadCompletionLog reads a log file into memory. Malformed lines are
// silently skipped. Missing files return an empty map.
func LoadCompletionLog(path string) (map[string]CompletionEntry, error) {
	result := make(map[string]CompletionEntry)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			entry, ok := parseCompletionLine(strings.TrimRight(line, "\n"))
			if ok {
				result[entry.Path] = entry
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func parseCompletionLine(line string) (CompletionEntry, bool) {
	parts := strings.Split(line, "\t")
	if len(parts) != 4 {
		return CompletionEntry{}, false
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return CompletionEntry{}, false
	}
	mtimeNs, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return CompletionEntry{}, false
	}
	hash := parts[3]
	if hash == "-" {
		hash = ""
	}
	return CompletionEntry{
		Path:  parts[0],
		Size:  size,
		MTime: time.Unix(0, mtimeNs),
		Hash:  hash,
	}, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sync/job/ -run TestCompletionLog -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/job/completion.go pkg/sync/job/completion_test.go
git commit -m "feat(job): add append-only TSV CompletionLog"
```

---

### Task 3: Implement `JobStore`

**Files:**
- Create: `pkg/sync/job/store.go`
- Create: `pkg/sync/job/store_test.go`

- [ ] **Step 1: Write the failing tests**

`pkg/sync/job/store_test.go`:
```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sync/job/ -run TestJobStore -v`
Expected: FAIL

- [ ] **Step 3: Implement `JobStore`**

`pkg/sync/job/store.go`:
```go
package job

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// JobStore manages job files in a directory. One file per job (<id>.json),
// plus a <id>.log for the completion log alongside.
type JobStore struct {
	dir string
	// StaleThreshold is the heartbeat age beyond which a "running" job is
	// considered crashed. Defaults to 10s.
	StaleThreshold time.Duration
}

func NewJobStore(dir string) *JobStore {
	return &JobStore{dir: dir, StaleThreshold: 10 * time.Second}
}

func (s *JobStore) Create(source, dest string, opts Options) (*Job, error) {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir job store: %w", err)
	}
	id := uuid.NewString()
	now := time.Now()
	j := &Job{
		ID:            id,
		Source:        source,
		Destination:   dest,
		Options:       opts,
		Status:        StatusRunning,
		CreatedAt:     now,
		Heartbeat:     now,
		CompletionLog: filepath.Join(s.dir, id+".log"),
	}
	if err := s.Save(j); err != nil {
		return nil, err
	}
	return j, nil
}

func (s *JobStore) metadataPath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func (s *JobStore) Save(j *Job) error {
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	tmp := s.metadataPath(j.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write job tmp: %w", err)
	}
	if err := os.Rename(tmp, s.metadataPath(j.ID)); err != nil {
		return fmt.Errorf("rename job file: %w", err)
	}
	return nil
}

func (s *JobStore) Get(id string) (*Job, error) {
	data, err := os.ReadFile(s.metadataPath(id))
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("unmarshal job: %w", err)
	}
	return &j, nil
}

func (s *JobStore) Delete(id string) error {
	if err := os.Remove(s.metadataPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(filepath.Join(s.dir, id+".log")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *JobStore) List() ([]*Job, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var jobs []*Job
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		j, err := s.Get(id)
		if err != nil {
			// Tolerate corrupt files.
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// SetStatus updates the Status field of the stored job.
func (s *JobStore) SetStatus(id string, status Status) error {
	j, err := s.Get(id)
	if err != nil {
		return err
	}
	j.Status = status
	return s.Save(j)
}

// UpdateHeartbeat refreshes the heartbeat timestamp on the stored job.
func (s *JobStore) UpdateHeartbeat(id string) error {
	j, err := s.Get(id)
	if err != nil {
		return err
	}
	j.Heartbeat = time.Now()
	return s.Save(j)
}

// Active returns the job the app should offer to resume at startup, or nil.
// A job is returned when it is explicitly paused, or when it is still marked
// running but has a stale heartbeat (crash detection). In the crash case,
// the returned job has Status normalized to Paused (and saved).
func (s *JobStore) Active() (*Job, error) {
	jobs, err := s.List()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, j := range jobs {
		switch j.Status {
		case StatusPaused:
			return j, nil
		case StatusRunning:
			if now.Sub(j.Heartbeat) > s.StaleThreshold {
				j.Status = StatusPaused
				if err := s.Save(j); err != nil {
					return nil, err
				}
				return j, nil
			}
		}
	}
	return nil, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sync/job/ -run TestJobStore -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/job/store.go pkg/sync/job/store_test.go
git commit -m "feat(job): add JobStore with crash detection"
```

---

### Task 4: Implement heartbeat ticker

**Files:**
- Create: `pkg/sync/job/heartbeat.go`
- Create: `pkg/sync/job/heartbeat_test.go`

- [ ] **Step 1: Write the failing test**

`pkg/sync/job/heartbeat_test.go`:
```go
package job

import (
	"testing"
	"time"
)

func TestHeartbeatTicksUntilStopped(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)
	j, err := store.Create("/src", "/dest", Options{Mode: "oneway"})
	if err != nil {
		t.Fatal(err)
	}

	hb := NewHeartbeat(store, j.ID, 50*time.Millisecond)
	hb.Start()

	time.Sleep(200 * time.Millisecond)

	hb.Stop()

	got, err := store.Get(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	age := time.Since(got.Heartbeat)
	if age > 500*time.Millisecond {
		t.Errorf("heartbeat not recent enough: age=%v", age)
	}
}

func TestHeartbeatStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store := NewJobStore(dir)
	j, _ := store.Create("/src", "/dest", Options{Mode: "oneway"})

	hb := NewHeartbeat(store, j.ID, 50*time.Millisecond)
	hb.Start()
	hb.Stop()
	hb.Stop() // must not panic
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sync/job/ -run TestHeartbeat -v`
Expected: FAIL

- [ ] **Step 3: Implement heartbeat**

`pkg/sync/job/heartbeat.go`:
```go
package job

import (
	"sync"
	"time"
)

// Heartbeat periodically updates a job's Heartbeat field until stopped.
type Heartbeat struct {
	store    *JobStore
	jobID    string
	interval time.Duration

	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

func NewHeartbeat(store *JobStore, jobID string, interval time.Duration) *Heartbeat {
	return &Heartbeat{store: store, jobID: jobID, interval: interval}
}

func (h *Heartbeat) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stop != nil {
		return // already running
	}
	h.stop = make(chan struct{})
	h.done = make(chan struct{})
	go h.loop(h.stop, h.done)
}

func (h *Heartbeat) loop(stop, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_ = h.store.UpdateHeartbeat(h.jobID) // best effort
		}
	}
}

func (h *Heartbeat) Stop() {
	h.mu.Lock()
	stop := h.stop
	done := h.done
	h.stop = nil
	h.done = nil
	h.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	<-done
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sync/job/ -run TestHeartbeat -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/job/heartbeat.go pkg/sync/job/heartbeat_test.go
git commit -m "feat(job): add heartbeat ticker"
```

---

## Phase 2: Atomic Writes in Storage Backend

### Task 5: Switch `Local.Write` to atomic partial+rename

**Files:**
- Modify: `pkg/storage/local.go:103-146`
- Modify: `pkg/storage/local_test.go` (add new test cases)

- [ ] **Step 1: Write the failing test**

Add to `pkg/storage/local_test.go`:
```go
func TestLocalWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	local, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello world")
	rc := io.NopCloser(bytes.NewReader(data))
	if err := local.Write(context.Background(), "sub/file.txt", rc, int64(len(data)), nil); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "sub/file.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: got %q", got)
	}

	// No leftover .partial
	entries, _ := os.ReadDir(filepath.Join(dir, "sub"))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".partial") {
			t.Errorf("partial file left behind: %s", e.Name())
		}
	}
}

func TestLocalWriteContextCancellationCleansUpPartial(t *testing.T) {
	dir := t.TempDir()
	local, err := NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Reader that blocks until context cancel
	ctx, cancel := context.WithCancel(context.Background())
	blockingReader := &slowReader{delay: 50 * time.Millisecond, total: 1024}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err = local.Write(ctx, "big.bin", io.NopCloser(blockingReader), 1024, nil)
	if err == nil {
		t.Fatal("expected error on context cancel")
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".partial") {
			t.Errorf("partial file not cleaned up: %s", e.Name())
		}
		if e.Name() == "big.bin" {
			t.Errorf("final file should not exist after cancel")
		}
	}
}

// slowReader emits bytes slowly so we can cancel mid-read.
type slowReader struct {
	delay time.Duration
	total int
	read  int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.read >= r.total {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	n := len(p)
	if r.read+n > r.total {
		n = r.total - r.read
	}
	r.read += n
	return n, nil
}
```

Add imports at top of file if missing: `"bytes"`, `"io"`, `"strings"`, `"time"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/storage/ -run TestLocalWrite -v`
Expected: FAIL (no partial file pattern yet, cancel test fails because current impl creates file at final path)

- [ ] **Step 3: Rewrite `Local.Write` with partial+rename and context awareness**

Replace `pkg/storage/local.go:103-146` with:
```go
// PartialSuffix is appended to file names while they are being written.
// Consumers (e.g. the pause/resume logic) may use a job-scoped suffix
// by setting PartialSuffixOverride before calling Write.
const defaultPartialSuffix = ".syncnorris.partial"

// PartialSuffixOverride, when non-empty on the Local instance, replaces
// the default partial suffix for this backend. Callers using jobs set
// this to ".syncnorris-<jobid8>.partial".
func (l *Local) SetPartialSuffix(s string) { l.partialSuffix = s }

// Write creates or overwrites a file atomically via a .partial temp file
// followed by a rename. If the context is cancelled mid-write, the partial
// file is removed and the destination path is left untouched.
func (l *Local) Write(ctx context.Context, path string, reader io.Reader, size int64, metadata *FileInfo) error {
	fullPath := filepath.Join(l.rootPath, path)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	partialSuffix := l.partialSuffix
	if partialSuffix == "" {
		partialSuffix = defaultPartialSuffix
	}
	partialPath := fullPath + partialSuffix

	file, err := os.Create(partialPath)
	if err != nil {
		return fmt.Errorf("failed to create partial file: %w", err)
	}

	// Wrap reader with context-aware reader so io.Copy stops on cancel
	cancelReader := &ctxReader{ctx: ctx, r: reader}

	written, copyErr := io.Copy(file, cancelReader)
	closeErr := file.Close()

	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partialPath)
		if copyErr != nil {
			return fmt.Errorf("failed to write file: %w", copyErr)
		}
		return fmt.Errorf("failed to close partial file: %w", closeErr)
	}
	if written != size {
		_ = os.Remove(partialPath)
		return fmt.Errorf("incomplete write: expected %d bytes, wrote %d", size, written)
	}

	// Atomic rename into final path
	if err := os.Rename(partialPath, fullPath); err != nil {
		_ = os.Remove(partialPath)
		return fmt.Errorf("failed to finalize file: %w", err)
	}

	if metadata != nil {
		if !metadata.ModTime.IsZero() {
			if err := os.Chtimes(fullPath, metadata.ModTime, metadata.ModTime); err != nil {
				return fmt.Errorf("failed to set modification time: %w", err)
			}
		}
		if metadata.Permissions != 0 {
			if err := os.Chmod(fullPath, os.FileMode(metadata.Permissions)); err != nil {
				return fmt.Errorf("failed to set permissions: %w", err)
			}
		}
	}
	return nil
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
	}
	return c.r.Read(p)
}
```

Add `partialSuffix string` field to the `Local` struct (top of file).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/storage/ -v`
Expected: PASS (all existing + new tests)

- [ ] **Step 5: Commit**

```bash
git add pkg/storage/local.go pkg/storage/local_test.go
git commit -m "feat(storage): atomic writes via .partial + rename with context cancel cleanup"
```

---

## Phase 3: Pipeline Integration

### Task 6: Wire optional `*Job` and pause channels into `PipelineConfig`

**Files:**
- Modify: `pkg/sync/pipeline.go:22-108`

- [ ] **Step 1: Write a failing integration test**

Create `pkg/sync/pipeline_pause_test.go`:
```go
package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sdejongh/syncnorris/pkg/compare"
	"github.com/sdejongh/syncnorris/pkg/logging"
	"github.com/sdejongh/syncnorris/pkg/models"
	"github.com/sdejongh/syncnorris/pkg/storage"
	"github.com/sdejongh/syncnorris/pkg/sync/job"
)

func setupSourceAndDest(t *testing.T, files map[string]string) (srcPath, destPath string) {
	t.Helper()
	src := t.TempDir()
	dst := t.TempDir()
	for name, content := range files {
		full := filepath.Join(src, name)
		_ = os.MkdirAll(filepath.Dir(full), 0755)
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return src, dst
}

func TestPipelineAcceptsJobConfig(t *testing.T) {
	src, dst := setupSourceAndDest(t, map[string]string{"a.txt": "hello"})
	source, _ := storage.NewLocal(src)
	dest, _ := storage.NewLocal(dst)
	defer source.Close()
	defer dest.Close()

	storeDir := t.TempDir()
	js := job.NewJobStore(storeDir)
	j, _ := js.Create(src, dst, job.Options{Mode: "oneway", Comparator: "namesize", Workers: 2})

	op := &models.SyncOperation{
		ID: "test", SourcePath: src, DestPath: dst,
		Mode: models.SyncOneWay, ComparisonMethod: models.CompareNameSize,
		MaxWorkers: 2, BufferSize: 4096,
	}
	cmp := compare.NewCompositeComparator(false, 4096)

	cfg := DefaultPipelineConfig()
	cfg.Job = j
	cfg.JobStore = js
	p := NewPipeline(source, dest, cmp, nil, logging.NewNullLogger(), op, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report, err := p.Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if report.Stats.FilesCopied.Load() != 1 {
		t.Errorf("expected 1 file copied, got %d", report.Stats.FilesCopied.Load())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sync/ -run TestPipelineAcceptsJobConfig -v`
Expected: FAIL (PipelineConfig.Job does not exist)

- [ ] **Step 3: Extend `PipelineConfig` and `Pipeline`**

Edit `pkg/sync/pipeline.go`. First, add import:
```go
"github.com/sdejongh/syncnorris/pkg/sync/job"
```

Extend `PipelineConfig` (lines ~58-62):
```go
type PipelineConfig struct {
	MaxWorkers int
	QueueSize  int
	// Job is optional. When set, the pipeline loads the completion log,
	// skips already-completed files, and appends to the log on success.
	Job       *job.Job
	JobStore  *job.JobStore
	// PauseSoft, when closed, causes the scanner to stop queueing new
	// tasks while in-flight workers finish their current file.
	PauseSoft <-chan struct{}
	// PauseHard, when closed, cancels the context and rolls back in-flight
	// .partial files.
	PauseHard <-chan struct{}
}
```

Extend `Pipeline` struct (lines 22-56):
```go
type Pipeline struct {
	// ... existing fields ...
	job           *job.Job
	jobStore      *job.JobStore
	pauseSoft     <-chan struct{}
	pauseHard     <-chan struct{}
	completionLog *job.CompletionLog
	doneSet       map[string]job.CompletionEntry
}
```

Extend `NewPipeline` (lines 72-109):
```go
func NewPipeline(
	source, dest storage.Backend,
	comparator compare.Comparator,
	formatter output.Formatter,
	logger logging.Logger,
	operation *models.SyncOperation,
	config PipelineConfig,
) *Pipeline {
	// ... existing validation + rateLimiter setup ...
	return &Pipeline{
		source:      source,
		dest:        dest,
		comparator:  comparator,
		formatter:   formatter,
		logger:      logger,
		operation:   operation,
		taskQueue:   make(chan *FileTask, config.QueueSize),
		queueSize:   config.QueueSize,
		destFiles:   make(map[string]*storage.FileInfo),
		destDirs:    make(map[string]*storage.FileInfo),
		activeFiles: make(map[string]int),
		results:     make([]*FileTask, 0),
		rateLimiter: rateLimiter,
		job:         config.Job,
		jobStore:    config.JobStore,
		pauseSoft:   config.PauseSoft,
		pauseHard:   config.PauseHard,
		doneSet:     make(map[string]job.CompletionEntry),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/sync/ -run TestPipelineAcceptsJobConfig -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/pipeline.go pkg/sync/pipeline_pause_test.go
git commit -m "feat(sync): wire optional Job and pause channels into PipelineConfig"
```

---

### Task 7: Preload completion log at pipeline start + skip scan phase

**Files:**
- Modify: `pkg/sync/pipeline.go:112-213` (in `Run`) and `:278-334` (in `scanSourceAndQueue`)

- [ ] **Step 1: Write a failing test**

Add to `pkg/sync/pipeline_pause_test.go`:
```go
func TestPipelineSkipsAlreadyCompletedFiles(t *testing.T) {
	src, dst := setupSourceAndDest(t, map[string]string{
		"a.txt": "alpha",
		"b.txt": "beta",
		"c.txt": "gamma",
	})
	source, _ := storage.NewLocal(src)
	dest, _ := storage.NewLocal(dst)
	defer source.Close()
	defer dest.Close()

	storeDir := t.TempDir()
	js := job.NewJobStore(storeDir)
	j, _ := js.Create(src, dst, job.Options{Mode: "oneway", Comparator: "namesize", Workers: 2})

	// Simulate that b.txt was already completed in a previous run
	cl, _ := job.NewCompletionLog(j.CompletionLog)
	srcStat, _ := os.Stat(filepath.Join(src, "b.txt"))
	_ = cl.Append(job.CompletionEntry{
		Path:  "b.txt",
		Size:  srcStat.Size(),
		MTime: srcStat.ModTime(),
	})
	_ = cl.Close()

	op := &models.SyncOperation{
		ID: "test", SourcePath: src, DestPath: dst,
		Mode: models.SyncOneWay, ComparisonMethod: models.CompareNameSize,
		MaxWorkers: 2, BufferSize: 4096,
	}
	cmp := compare.NewCompositeComparator(false, 4096)

	cfg := DefaultPipelineConfig()
	cfg.Job = j
	cfg.JobStore = js
	p := NewPipeline(source, dest, cmp, nil, logging.NewNullLogger(), op, cfg)

	report, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// a.txt and c.txt copied, b.txt skipped
	if report.Stats.FilesCopied.Load() != 2 {
		t.Errorf("expected 2 copied, got %d", report.Stats.FilesCopied.Load())
	}
	// Destination must not contain b.txt because we skipped it entirely
	if _, err := os.Stat(filepath.Join(dst, "b.txt")); !os.IsNotExist(err) {
		t.Errorf("b.txt should not exist in dest (skipped), but does")
	}
}

func TestPipelineRecopiesWhenSourceChangedSinceComplete(t *testing.T) {
	src, dst := setupSourceAndDest(t, map[string]string{"a.txt": "old"})
	source, _ := storage.NewLocal(src)
	dest, _ := storage.NewLocal(dst)
	defer source.Close()
	defer dest.Close()

	storeDir := t.TempDir()
	js := job.NewJobStore(storeDir)
	j, _ := js.Create(src, dst, job.Options{Mode: "oneway", Comparator: "namesize", Workers: 1})

	// Record completion with STALE metadata (different size/mtime than current)
	cl, _ := job.NewCompletionLog(j.CompletionLog)
	_ = cl.Append(job.CompletionEntry{
		Path:  "a.txt",
		Size:  999, // wrong size
		MTime: time.Unix(1, 0),
	})
	_ = cl.Close()

	op := &models.SyncOperation{
		ID: "test", SourcePath: src, DestPath: dst,
		Mode: models.SyncOneWay, ComparisonMethod: models.CompareNameSize,
		MaxWorkers: 1, BufferSize: 4096,
	}
	cfg := DefaultPipelineConfig()
	cfg.Job = j
	cfg.JobStore = js
	p := NewPipeline(source, dest, compare.NewCompositeComparator(false, 4096), nil, logging.NewNullLogger(), op, cfg)

	report, _ := p.Run(context.Background())
	if report.Stats.FilesCopied.Load() != 1 {
		t.Errorf("expected 1 copied (stale completion), got %d", report.Stats.FilesCopied.Load())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sync/ -run "TestPipelineSkipsAlreadyCompletedFiles|TestPipelineRecopiesWhenSourceChangedSinceComplete" -v`
Expected: FAIL

- [ ] **Step 3: Implement preload + skip logic**

In `pkg/sync/pipeline.go`, modify `Run` to load the completion log right after `NewPipeline`-style initialization. Add right after the `cancel` deferred (around line 135):
```go
// Preload completion log if a job is attached
if p.job != nil {
	loaded, err := job.LoadCompletionLog(p.job.CompletionLog)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn(ctx, "failed to load completion log, starting fresh", logging.Fields{"err": err.Error()})
		}
		loaded = make(map[string]job.CompletionEntry)
	}
	p.doneSet = loaded

	cl, err := job.NewCompletionLog(p.job.CompletionLog)
	if err != nil {
		return nil, fmt.Errorf("open completion log: %w", err)
	}
	p.completionLog = cl
	defer func() { _ = p.completionLog.Close() }()
}
```

Add `"fmt"` to imports if not already present.

Modify `scanSourceAndQueue` (line 278). At the top of the callback, after the `f.IsDir` check and exclude check, add the done-set skip logic:
```go
// Skip files already completed in a previous run of this job
if p.job != nil {
	if entry, done := p.doneSet[f.RelativePath]; done {
		srcInfo, err := p.source.Stat(ctx, f.RelativePath)
		if err == nil && srcInfo.Size == entry.Size && srcInfo.ModTime.UnixNano() == entry.MTime.UnixNano() {
			report.Stats.FilesSkipped.Add(1)
			if p.logger != nil {
				p.logger.Debug(ctx, "File skipped (previously completed)", logging.Fields{"path": f.RelativePath})
			}
			return nil
		}
		// Stale completion: fall through to normal flow
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sync/ -run TestPipeline -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/pipeline.go pkg/sync/pipeline_pause_test.go
git commit -m "feat(sync): skip previously completed files on resume via completion log"
```

---

### Task 8: Append to completion log after successful copy/update

**Files:**
- Modify: `pkg/sync/pipeline.go:495-651` (`copyFile`) and `:653-818` (`updateFile`)

- [ ] **Step 1: Write a failing test**

Add to `pkg/sync/pipeline_pause_test.go`:
```go
func TestPipelineWritesCompletionLog(t *testing.T) {
	src, dst := setupSourceAndDest(t, map[string]string{"a.txt": "x", "b.txt": "y"})
	source, _ := storage.NewLocal(src)
	dest, _ := storage.NewLocal(dst)
	defer source.Close()
	defer dest.Close()

	storeDir := t.TempDir()
	js := job.NewJobStore(storeDir)
	j, _ := js.Create(src, dst, job.Options{Mode: "oneway", Comparator: "namesize"})

	op := &models.SyncOperation{
		ID: "test", SourcePath: src, DestPath: dst,
		Mode: models.SyncOneWay, ComparisonMethod: models.CompareNameSize,
		MaxWorkers: 1, BufferSize: 4096,
	}
	cfg := DefaultPipelineConfig()
	cfg.Job = j
	cfg.JobStore = js
	p := NewPipeline(source, dest, compare.NewCompositeComparator(false, 4096), nil, logging.NewNullLogger(), op, cfg)

	if _, err := p.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	loaded, err := job.LoadCompletionLog(j.CompletionLog)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 completion entries, got %d", len(loaded))
	}
	if _, ok := loaded["a.txt"]; !ok {
		t.Error("a.txt missing from completion log")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/sync/ -run TestPipelineWritesCompletionLog -v`
Expected: FAIL (log stays empty)

- [ ] **Step 3: Append on success**

Add a helper method to `Pipeline` in `pkg/sync/pipeline.go`:
```go
func (p *Pipeline) recordCompletion(task *FileTask, sourceModTime time.Time) {
	if p.completionLog == nil {
		return
	}
	entry := job.CompletionEntry{
		Path:  task.RelativePath,
		Size:  task.Size,
		MTime: sourceModTime,
		Hash:  task.SourceHash, // may be empty; see step 3b
	}
	if err := p.completionLog.Append(entry); err != nil && p.logger != nil {
		p.logger.Warn(context.Background(), "failed to append completion log", logging.Fields{
			"path": task.RelativePath,
			"err":  err.Error(),
		})
	}
}
```

In `copyFile` just before the final `p.formatter.Progress(output.ProgressUpdate{Type: "file_complete",...})` block (around line 642 after `p.addResult(task)`), insert:
```go
p.recordCompletion(task, sourceInfo.ModTime)
```

Do the same in `updateFile` (mirror structure). `sourceInfo` is local to each function so pass the `ModTime` you already have.

**Step 3b: Carry source hash when the comparator computes one.** Add a `SourceHash` field to `FileTask` in `pkg/sync/task.go`:
```go
type FileTask struct {
	// ... existing fields ...
	SourceHash string
}
```

In the `processTask` flow (around line 436 where comparator returns `Comparison`), if the comparison carries a `SourceHash` (many comparators do; see `compare.Comparison.SourceHash`), copy it:
```go
if comparison != nil && comparison.SourceHash != "" {
	task.SourceHash = comparison.SourceHash
}
```

If `compare.Comparison` does not yet expose `SourceHash`, skip setting it — `SourceHash` stays empty and the log entry uses a `-` placeholder, which is fine.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sync/ -run TestPipeline -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/pipeline.go pkg/sync/task.go pkg/sync/pipeline_pause_test.go
git commit -m "feat(sync): append to completion log after successful copy/update"
```

---

### Task 9: Implement soft/hard pause channels

**Files:**
- Modify: `pkg/sync/pipeline.go:278-334` (scanner) and `:337-352` (worker)

- [ ] **Step 1: Write failing tests**

Add to `pkg/sync/pipeline_pause_test.go`:
```go
func TestPipelineSoftPauseStopsNewTasksButFinishesInFlight(t *testing.T) {
	// Enough files that the scanner will still be producing when pause fires
	files := make(map[string]string)
	for i := 0; i < 50; i++ {
		files[fmt.Sprintf("f%02d.txt", i)] = "content"
	}
	src, dst := setupSourceAndDest(t, files)
	source, _ := storage.NewLocal(src)
	dest, _ := storage.NewLocal(dst)
	defer source.Close()
	defer dest.Close()

	pause := make(chan struct{})

	op := &models.SyncOperation{
		ID: "t", SourcePath: src, DestPath: dst,
		Mode: models.SyncOneWay, ComparisonMethod: models.CompareNameSize,
		MaxWorkers: 2, BufferSize: 4096,
	}
	cfg := DefaultPipelineConfig()
	cfg.PauseSoft = pause
	p := NewPipeline(source, dest, compare.NewCompositeComparator(false, 4096), nil, logging.NewNullLogger(), op, cfg)

	// Trigger soft pause after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(pause)
	}()

	report, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	copied := int(report.Stats.FilesCopied.Load())
	if copied == 0 {
		t.Error("expected some files copied before pause")
	}
	if copied == 50 {
		t.Error("expected pause to stop scanner before all files")
	}
}

func TestPipelineHardPauseCancelsImmediately(t *testing.T) {
	files := make(map[string]string)
	for i := 0; i < 20; i++ {
		files[fmt.Sprintf("f%02d.txt", i)] = "content"
	}
	src, dst := setupSourceAndDest(t, files)
	source, _ := storage.NewLocal(src)
	dest, _ := storage.NewLocal(dst)
	defer source.Close()
	defer dest.Close()

	hard := make(chan struct{})

	op := &models.SyncOperation{
		ID: "t", SourcePath: src, DestPath: dst,
		Mode: models.SyncOneWay, ComparisonMethod: models.CompareNameSize,
		MaxWorkers: 2, BufferSize: 4096,
	}
	cfg := DefaultPipelineConfig()
	cfg.PauseHard = hard
	p := NewPipeline(source, dest, compare.NewCompositeComparator(false, 4096), nil, logging.NewNullLogger(), op, cfg)

	go func() {
		time.Sleep(5 * time.Millisecond)
		close(hard)
	}()

	_, _ = p.Run(context.Background()) // accept the error

	// No .partial files should remain anywhere
	_ = filepath.Walk(dst, func(path string, info os.FileInfo, err error) error {
		if info != nil && !info.IsDir() && strings.Contains(info.Name(), ".partial") {
			t.Errorf("partial file remains after hard pause: %s", path)
		}
		return nil
	})
}
```

Add needed imports (`fmt`, `strings`) to the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/sync/ -run "TestPipelineSoftPause|TestPipelineHardPause" -v`
Expected: FAIL

- [ ] **Step 3: Implement soft pause — stop scanner on `pauseSoft` close**

In `pkg/sync/pipeline.go`, near the top of `Run` after creating the cancellable context, add a goroutine that monitors `pauseHard` and cancels the context:
```go
if p.pauseHard != nil {
	go func() {
		select {
		case <-p.pauseHard:
			cancel()
		case <-ctx.Done():
		}
	}()
}
```

Modify `scanSourceAndQueue` to check `pauseSoft` inside the Walk callback, very early (after the directory and exclude checks):
```go
if p.pauseSoft != nil {
	select {
	case <-p.pauseSoft:
		// Soft pause: stop producing. Return a sentinel error to end the walk.
		return errSoftPaused
	default:
	}
}
```

At package level in `pipeline.go`:
```go
var errSoftPaused = errors.New("soft pause requested")
```

Add `"errors"` to imports if needed.

Adjust the caller of `scanSourceAndQueue` in `Run` (around line 200) to treat `errSoftPaused` as a clean stop, not a failure:
```go
scanErr := p.scanSourceAndQueue(ctx, report)
if scanErr != nil && !errors.Is(scanErr, errSoftPaused) {
	// existing error path
	report.Status = models.StatusFailed
	return report, scanErr
}
```

The queue closure and wait logic already handles workers draining the remaining buffered tasks.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/sync/ -run TestPipeline -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/sync/pipeline.go pkg/sync/pipeline_pause_test.go
git commit -m "feat(sync): implement soft (scanner-stop) and hard (context-cancel) pause"
```

---

## Phase 4: Instance Lock

### Task 10: Add `gofrs/flock` dependency and instance lock helper

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/gui/lock.go`
- Create: `internal/gui/lock_test.go`

- [ ] **Step 1: Add dependency**

Run:
```bash
go get github.com/gofrs/flock@latest
go mod tidy
```

- [ ] **Step 2: Write the failing test**

`internal/gui/lock_test.go`:
```go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/gui/ -run TestInstanceLock -v`
Expected: FAIL

- [ ] **Step 4: Implement the lock helper**

`internal/gui/lock.go`:
```go
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

func (i *InstanceLock) Release() error {
	return i.l.Unlock()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/gui/ -run TestInstanceLock -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/gui/lock.go internal/gui/lock_test.go
git commit -m "feat(gui): add single-instance lock via gofrs/flock"
```

---

## Phase 5: GUI Integration

### Task 11: Thread `*Job` through the GUI runner

**Files:**
- Modify: `internal/gui/runner.go:20-117`

- [ ] **Step 1: Extend `RunOptions` and `runSync`**

Edit `internal/gui/runner.go`:

Add import:
```go
"github.com/sdejongh/syncnorris/pkg/sync/job"
```

Add to `RunOptions`:
```go
type RunOptions struct {
	// ... existing fields ...
	Job      *job.Job
	JobStore *job.JobStore
	PauseSoft chan struct{}
	PauseHard chan struct{}
}
```

Modify `runSync` to pass the job into the pipeline config. After building the engine, the current code calls `engine.Run(ctx)`. We need `sync.NewEngine` to accept a `PipelineConfig` (or similar) so the pause/job plumbing reaches the pipeline. Update `pkg/sync/engine.go:runPipeline` to accept a `PipelineConfig` override — add an `Options` setter on the engine.

Add to `pkg/sync/engine.go`:
```go
// SetPipelineConfig overrides the default pipeline config. Must be called
// before Run.
func (e *Engine) SetPipelineConfig(cfg PipelineConfig) {
	e.pipelineConfig = cfg
}
```

And a matching field `pipelineConfig PipelineConfig` on the `Engine` struct (default-initialize via `DefaultPipelineConfig()` in `NewEngine`). In `runPipeline`, replace the call site that builds config:
```go
cfg := e.pipelineConfig
if cfg.MaxWorkers == 0 {
	cfg = DefaultPipelineConfig()
}
p := NewPipeline(e.source, e.dest, e.comparator, e.formatter, e.logger, e.operation, cfg)
```

Back in `runner.go` `runSync`:
```go
engine := sync.NewEngine(source, dest, comparator, formatter, logging.NewNullLogger(), op)
cfg := sync.DefaultPipelineConfig()
cfg.Job = opts.Job
cfg.JobStore = opts.JobStore
cfg.PauseSoft = opts.PauseSoft
cfg.PauseHard = opts.PauseHard
engine.SetPipelineConfig(cfg)
report, err := engine.Run(ctx)
```

- [ ] **Step 2: Run existing tests to verify no regression**

Run: `go test ./pkg/sync/... ./internal/gui/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/gui/runner.go pkg/sync/engine.go
git commit -m "feat(gui): thread optional Job and pause channels through runner"
```

---

### Task 12: Add state machine and pause/resume/abandon logic to `appState`

**Files:**
- Modify: `internal/gui/app.go:30-97` (appState), `:130-169` (init), `:335-` (startOperation)

- [ ] **Step 1: Add state machine and job fields to `appState`**

Edit `internal/gui/app.go`:

Add imports:
```go
"github.com/sdejongh/syncnorris/pkg/sync/job"
```

Add to `appState`:
```go
type appState struct {
	// ... existing fields ...

	// Instance lock (nil if not acquired yet)
	instanceLock *InstanceLock

	// Job management
	jobStore   *job.JobStore
	activeJob  *job.Job
	heartbeat  *job.Heartbeat
	pauseSoft  chan struct{}
	pauseHard  chan struct{}

	// New buttons
	pauseSoftBtn widget.Clickable
	pauseHardBtn widget.Clickable
	resumeBtn    widget.Clickable
	abandonBtn   widget.Clickable

	// Resume banner dismiss/actions
	bannerResumeBtn   widget.Clickable
	bannerAbandonBtn  widget.Clickable

	// High-level state (replaces isRunning)
	runState runState
}

type runState int

const (
	stateIdle runState = iota
	stateRunning
	statePaused
)
```

Remove the existing `isRunning bool` field and replace all reads with `a.runState == stateRunning`.

In `init()` after settings load, set up the job store and scan for an interrupted job:
```go
cfgDir, err := os.UserConfigDir()
if err != nil {
	cfgDir = os.TempDir()
}
jobsDir := filepath.Join(cfgDir, "syncnorris", "jobs")
a.jobStore = job.NewJobStore(jobsDir)

if interrupted, _ := a.jobStore.Active(); interrupted != nil {
	a.activeJob = interrupted
	a.runState = statePaused
	// Restore params into widgets
	a.sourceEditor.SetText(interrupted.Source)
	a.destEditor.SetText(interrupted.Destination)
	a.compEnum.Value = interrupted.Options.Comparator
	a.workersEditor.SetText(strconv.Itoa(interrupted.Options.Workers))
	a.excludeEditor.SetText(strings.Join(interrupted.Options.ExcludePatterns, ","))
	a.dryRunCheck.Value = interrupted.Options.DryRun
	a.deleteCheck.Value = interrupted.Options.DeleteOrphans
	a.statusMsg = "Job interrupted — resume or abandon"
	a.statusLevel = "info"
}
```

Add imports `"os"`, `"path/filepath"`, `"strings"` if missing.

- [ ] **Step 2: Add startOperation variants**

Modify `startOperation(dryRun bool)` to create a new job before launching:
```go
func (a *appState) startOperation(dryRun bool) {
	opts := a.buildRunOptions(dryRun)

	// Create job metadata before launch
	j, err := a.jobStore.Create(opts.Source, opts.Dest, a.optionsForJob(opts))
	if err != nil {
		a.setStatus("failed to create job: "+err.Error(), "error")
		return
	}
	a.activeJob = j
	a.heartbeat = job.NewHeartbeat(a.jobStore, j.ID, 2*time.Second)
	a.heartbeat.Start()

	a.pauseSoft = make(chan struct{})
	a.pauseHard = make(chan struct{})
	opts.Job = j
	opts.JobStore = a.jobStore
	opts.PauseSoft = a.pauseSoft
	opts.PauseHard = a.pauseHard

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFn = cancel
	a.runState = stateRunning
	a.bandwidth.Reset()

	go func() {
		runSync(ctx, opts, a.events)
		a.events <- UIEvent{Type: eventJobFinished}
	}()
}
```

Extract a helper `buildRunOptions(dryRun bool) RunOptions` that mirrors the existing opts assembly.

Add `optionsForJob(opts RunOptions) job.Options`:
```go
func (a *appState) optionsForJob(opts RunOptions) job.Options {
	return job.Options{
		Mode:            opts.Mode,
		Comparator:      opts.Comparison,
		Workers:         opts.Workers,
		BufferSize:      opts.BufferSize,
		DeleteOrphans:   opts.Delete,
		DryRun:          opts.DryRun,
		ExcludePatterns: parseExcludes(opts.Excludes),
	}
}
```

Add the new event type in `internal/gui/formatter.go` or wherever `UIEvent.Type` constants live:
```go
eventJobFinished = "job_finished"
```

On receiving `eventJobFinished` in `consumeEvents`, transition to `statePaused` if a job is still active (user paused) or `stateIdle` if the run completed normally:
```go
case eventJobFinished:
	if a.heartbeat != nil {
		a.heartbeat.Stop()
		a.heartbeat = nil
	}
	if a.activeJob != nil {
		// Determine whether this was a pause or a completion.
		// Completion = pipeline ran to end without pause signals closed.
		// For now we rely on pauseSoft/pauseHard being closed.
		if a.pauseWasRequested() {
			a.runState = statePaused
			_ = a.jobStore.SetStatus(a.activeJob.ID, job.StatusPaused)
		} else {
			_ = a.jobStore.Delete(a.activeJob.ID)
			a.activeJob = nil
			a.runState = stateIdle
		}
	}
```

Add `pauseWasRequested()`:
```go
func (a *appState) pauseWasRequested() bool {
	select {
	case <-a.pauseSoft:
		return true
	default:
	}
	select {
	case <-a.pauseHard:
		return true
	default:
	}
	return false
}
```

- [ ] **Step 3: Add handlers for new buttons in `handleEvents`**

In `handleEvents`, add:
```go
if a.pauseSoftBtn.Clicked(gtx) && a.runState == stateRunning {
	close(a.pauseSoft)
}
if a.pauseHardBtn.Clicked(gtx) && a.runState == stateRunning {
	close(a.pauseHard)
}
if a.resumeBtn.Clicked(gtx) && a.runState == statePaused {
	a.resumeJob()
}
if a.abandonBtn.Clicked(gtx) && a.runState == statePaused {
	a.abandonJob()
}
if a.bannerResumeBtn.Clicked(gtx) && a.activeJob != nil {
	a.resumeJob()
}
if a.bannerAbandonBtn.Clicked(gtx) && a.activeJob != nil {
	a.abandonJob()
}
```

Add:
```go
func (a *appState) resumeJob() {
	if a.activeJob == nil {
		return
	}
	opts := a.buildRunOptions(a.activeJob.Options.DryRun)
	opts.Job = a.activeJob
	opts.JobStore = a.jobStore
	_ = a.jobStore.SetStatus(a.activeJob.ID, job.StatusRunning)
	a.heartbeat = job.NewHeartbeat(a.jobStore, a.activeJob.ID, 2*time.Second)
	a.heartbeat.Start()

	a.pauseSoft = make(chan struct{})
	a.pauseHard = make(chan struct{})
	opts.PauseSoft = a.pauseSoft
	opts.PauseHard = a.pauseHard

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFn = cancel
	a.runState = stateRunning

	go func() {
		runSync(ctx, opts, a.events)
		a.events <- UIEvent{Type: eventJobFinished}
	}()
}

func (a *appState) abandonJob() {
	if a.activeJob == nil {
		return
	}
	// Best-effort cleanup: walk destination for partial files with the job's suffix
	suffix := fmt.Sprintf(".syncnorris-%s.partial", a.activeJob.ID[:8])
	_ = filepath.Walk(a.activeJob.Destination, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), suffix) {
			_ = os.Remove(path)
		}
		return nil
	})
	_ = a.jobStore.Delete(a.activeJob.ID)
	a.activeJob = nil
	a.runState = stateIdle
	a.setStatus("Job abandoned", "info")
}
```

- [ ] **Step 4: Verify builds**

Run: `go build ./...`
Expected: success.

Run: `go test ./internal/gui/...`
Expected: PASS (no test added here yet, existing ones should still pass).

- [ ] **Step 5: Commit**

```bash
git add internal/gui/app.go internal/gui/runner.go internal/gui/formatter.go
git commit -m "feat(gui): add state machine for pause/resume/abandon"
```

---

### Task 13: Layout — state-aware buttons and resume banner

**Files:**
- Modify: `internal/gui/layout.go` (`layoutActions`, new `layoutResumeBanner`)
- Create: `internal/gui/banner.go`

- [ ] **Step 1: Write the banner layout**

`internal/gui/banner.go`:
```go
//go:build !nogui

package gui

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

// layoutResumeBanner draws a one-line banner offering to resume or abandon
// an interrupted job. Renders nothing when activeJob is nil or runState is
// not statePaused at startup.
func (a *appState) layoutResumeBanner(gtx layout.Context) layout.Dimensions {
	if a.activeJob == nil || a.runState != statePaused {
		return layout.Dimensions{}
	}
	bg := color.NRGBA{R: 0xFF, G: 0xF5, B: 0xCC, A: 0xFF}
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		_ = bg
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.Body1(a.theme, "A job was interrupted.").Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(material.Button(a.theme, &a.bannerResumeBtn, "Resume").Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(material.Button(a.theme, &a.bannerAbandonBtn, "Abandon").Layout),
		)
	})
}
```

- [ ] **Step 2: Update `layoutActions` to be state-aware**

Modify `internal/gui/layout.go` `layoutActions`:
```go
func (a *appState) layoutActions(gtx C) D {
	switch a.runState {
	case stateIdle:
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(material.Button(a.theme, &a.syncBtn, "Run").Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(material.Button(a.theme, &a.compareBtn, "Compare").Layout),
		)
	case stateRunning:
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(material.Button(a.theme, &a.pauseSoftBtn, "Pause (soft)").Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(material.Button(a.theme, &a.pauseHardBtn, "Pause (hard)").Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(material.Button(a.theme, &a.cancelBtn, "Cancel").Layout),
		)
	case statePaused:
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(material.Button(a.theme, &a.resumeBtn, "Resume").Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(material.Button(a.theme, &a.abandonBtn, "Abandon").Layout),
		)
	}
	return D{}
}
```

- [ ] **Step 3: Insert banner into top-level layout**

In `internal/gui/layout.go` at the top of the main `layout()` method:
```go
return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
	layout.Rigid(a.layoutResumeBanner),
	layout.Flexed(1, a.layoutMainContent), // existing horizontal split extracted into helper
)
```

If the current `layout` method already returns a flex, wrap the banner above it.

- [ ] **Step 4: Verify builds and visual smoke test**

Run: `go build -tags !nogui ./...`
Expected: success.

Run the GUI manually:
```bash
go run ./cmd/syncnorris gui
```
- Start a long sync (many files).
- Click "Pause (soft)" — scanner stops, workers finish in-flight, state transitions to Paused with Resume/Abandon visible.
- Click "Resume" — sync continues.
- Click "Pause (hard)" — immediate stop, no `.partial` files remain in dest.
- Close the app mid-sync, relaunch — banner appears, Resume works.

- [ ] **Step 5: Commit**

```bash
git add internal/gui/layout.go internal/gui/banner.go
git commit -m "feat(gui): state-aware action buttons and resume banner"
```

---

### Task 14: Wire the instance lock at GUI startup

**Files:**
- Modify: `internal/gui/app.go:100-128` (`Run`)

- [ ] **Step 1: Add the lock acquisition**

In `internal/gui/app.go` `Run()`, before `a.init()`:
```go
cfgDir, err := os.UserConfigDir()
if err != nil {
	cfgDir = os.TempDir()
}
lockPath := filepath.Join(cfgDir, "syncnorris", "app.lock")
lock, err := AcquireInstanceLock(lockPath)
if err != nil {
	return fmt.Errorf("syncnorris is already running: %w", err)
}
defer lock.Release()

a := &appState{
	events:       make(chan UIEvent, 500),
	instanceLock: lock,
}
```

Add imports `"os"`, `"path/filepath"`, `"fmt"` if missing.

- [ ] **Step 2: Manual test**

```bash
go run ./cmd/syncnorris gui &
go run ./cmd/syncnorris gui
```
Expected: second launch exits with error "syncnorris is already running".

- [ ] **Step 3: Commit**

```bash
git add internal/gui/app.go
git commit -m "feat(gui): enforce single instance via app.lock"
```

---

## Phase 6: Documentation and Release

### Task 15: Update docs and bump version

**Files:**
- Modify: `CLAUDE.md`, `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Update `CLAUDE.md`**

Add a new subsection in the GUI section documenting:
- Job persistence location (`~/.config/syncnorris/jobs/`)
- Instance lock (`~/.config/syncnorris/app.lock`)
- Atomic write pattern in `Local.Write` (write-to-`.partial`, rename)
- Mention that `scanSourceAndQueue` now respects a completion log when a `*Job` is provided

- [ ] **Step 2: Update `README.md`**

Add a new section "Pause / Resume" under the GUI usage chapter describing:
- Soft vs. hard pause behavior
- Automatic recovery after a crash or app close
- Completion log behavior: stat-based skip, hash fallback when available
- One-way sync only (bidirectional not yet supported)

- [ ] **Step 3: Update `CHANGELOG.md`**

Add under a new `## [v0.8.0] — 2026-04-19` heading:
```markdown
### Added
- GUI: Pause/Resume for one-way sync jobs with soft and hard pause modes
- GUI: Automatic detection of jobs interrupted by crash or app close
- Single-instance lock prevents concurrent GUI runs
- Atomic file writes via `.partial` + rename (improves safety for all runs)

### Changed
- `storage.Local.Write` now writes to a `.partial` suffix and renames on success
- `pkg/sync.PipelineConfig` accepts optional `Job`, `JobStore`, `PauseSoft`, `PauseHard`
```

- [ ] **Step 4: Manual release verification**

Run:
```bash
make build
make test
make lint
```
Expected: all green.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md README.md CHANGELOG.md
git commit -m "docs: document pause/resume feature and v0.8.0 release"
```

---

## Self-Review Notes

- **Spec coverage:**
  - Job type + persistence → Task 1
  - CompletionLog format (TSV append-only, batch flush, parser tolerant) → Task 2
  - JobStore with crash detection via heartbeat → Tasks 3, 4
  - Atomic write with `.partial` → Task 5
  - Pipeline integration (skip on resume, append on success) → Tasks 6, 7, 8
  - Soft/hard pause → Task 9
  - Instance lock → Task 10
  - GUI state machine + banner + buttons → Tasks 12, 13, 14
  - Documentation → Task 15
- **Placeholder scan:** no TBD/TODO. Each task has concrete code. The `compare.Comparison.SourceHash` check in Task 8 acknowledges the field may not exist yet and provides a safe fallback (empty hash → `-` in log).
- **Type consistency:** `Options`, `Status`, `CompletionEntry`, `Job`, `Heartbeat`, `JobStore` types are used consistently across tasks. `runState` / `stateIdle` / `stateRunning` / `statePaused` used consistently in Tasks 12–14.
- **Known follow-ups intentionally out of scope:**
  - Suffix-per-job on `Local.Write` (task 5 wires the override field but GUI never sets it yet; the default suffix works for hard-pause cleanup because it's still unique enough when only one job runs at a time)
  - Bidirectional pause
  - Hash fallback path (only populated when comparator returns a source hash)
