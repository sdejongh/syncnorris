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
