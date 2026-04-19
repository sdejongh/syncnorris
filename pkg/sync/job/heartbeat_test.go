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
