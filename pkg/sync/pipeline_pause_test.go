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
