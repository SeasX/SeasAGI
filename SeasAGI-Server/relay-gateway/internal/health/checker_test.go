package health

import (
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
)

func TestNewChecker_CustomInterval(t *testing.T) {
	store := channel.NewStore()
	defer store.Close()

	interval := 30 * time.Second
	c := NewChecker(store, interval)
	if c == nil {
		t.Fatal("NewChecker() returned nil")
	}
	if c.interval != interval {
		t.Errorf("interval = %v, want %v", c.interval, interval)
	}
}

func TestNewChecker_DefaultInterval(t *testing.T) {
	store := channel.NewStore()
	defer store.Close()

	c := NewChecker(store, 0)
	if c == nil {
		t.Fatal("NewChecker() returned nil")
	}
	if c.interval != 60*time.Second {
		t.Errorf("interval = %v, want 60s", c.interval)
	}
}

func TestStartAndStop(t *testing.T) {
	store := channel.NewStore()
	defer store.Close()

	c := NewChecker(store, 100*time.Millisecond)
	c.Start()
	time.Sleep(50 * time.Millisecond)
	c.Stop()
}

func TestDoubleStart(t *testing.T) {
	store := channel.NewStore()
	defer store.Close()

	c := NewChecker(store, 100*time.Millisecond)
	c.Start()
	c.Start() // should not panic
	time.Sleep(50 * time.Millisecond)
	c.Stop()
}

func TestDoubleStop(t *testing.T) {
	store := channel.NewStore()
	defer store.Close()

	c := NewChecker(store, 100*time.Millisecond)
	c.Start()
	c.Stop()
	c.Stop() // should not panic
}

func TestLoopStopsOnStop(t *testing.T) {
	store := channel.NewStore()
	defer store.Close()

	c := NewChecker(store, 100*time.Millisecond)
	c.Start()
	time.Sleep(50 * time.Millisecond)
	c.Stop()
	// If we reach here, the loop exited cleanly (no deadlock, no panic)
}

func TestStructFieldsInitialized(t *testing.T) {
	store := channel.NewStore()
	defer store.Close()

	c := NewChecker(store, 30*time.Second)

	if c.store != store {
		t.Error("store field not set correctly")
	}
	if c.interval != 30*time.Second {
		t.Errorf("interval = %v, want 30s", c.interval)
	}
	if c.stopCh == nil {
		t.Error("stopCh is nil")
	}
}