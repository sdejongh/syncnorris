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
