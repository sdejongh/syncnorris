package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		Mode: models.ModeOneWay, ComparisonMethod: models.CompareNameSize,
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
		Mode: models.ModeOneWay, ComparisonMethod: models.CompareNameSize,
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
		Mode: models.ModeOneWay, ComparisonMethod: models.CompareNameSize,
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
		Mode: models.ModeOneWay, ComparisonMethod: models.CompareNameSize,
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

func TestPipelineSoftPauseStopsNewTasksButFinishesInFlight(t *testing.T) {
	// 50 × 4 MB files on tmpfs.  QueueSize=2 forces the scanner goroutine to
	// block on send after the first 2 tasks, so the 10 ms timer can fire while
	// the scanner is waiting for a queue slot.  Workers copy ~1.4 ms/file on
	// tmpfs, so ~14 files are done when the pause lands, leaving ~36 unqueued.
	bigContent := strings.Repeat("x", 4<<20) // 4 MB
	files := make(map[string]string)
	for i := 0; i < 50; i++ {
		files[fmt.Sprintf("f%02d.txt", i)] = bigContent
	}
	src, dst := setupSourceAndDest(t, files)
	source, _ := storage.NewLocal(src)
	dest, _ := storage.NewLocal(dst)
	defer source.Close()
	defer dest.Close()

	pause := make(chan struct{})

	op := &models.SyncOperation{
		ID: "t", SourcePath: src, DestPath: dst,
		Mode: models.ModeOneWay, ComparisonMethod: models.CompareNameSize,
		MaxWorkers: 2, BufferSize: 4096,
	}
	cfg := DefaultPipelineConfig()
	cfg.PauseSoft = pause
	cfg.QueueSize = 2 // tiny queue forces the scanner to block, giving pause time to fire
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
		t.Error("expected pause to stop scanner before all files were processed")
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
		Mode: models.ModeOneWay, ComparisonMethod: models.CompareNameSize,
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
