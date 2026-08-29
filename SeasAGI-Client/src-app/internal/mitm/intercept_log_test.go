package mitm

import (
	"sync"
	"testing"
)

func TestRingBufferLog(t *testing.T) {
	logger := NewInterceptLogger(5)
	for i := 0; i < 3; i++ {
		logger.Log(InterceptEntry{Method: "GET", Host: "example.com", Path: "/test", Status: 200})
	}
	recent := logger.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(recent))
	}
	for _, entry := range recent {
		if entry.Host != "example.com" {
			t.Errorf("expected host example.com, got %s", entry.Host)
		}
	}
}

func TestRingBufferOverflow(t *testing.T) {
	logger := NewInterceptLogger(3)
	for i := 0; i < 5; i++ {
		logger.Log(InterceptEntry{Method: "GET", Host: "example.com", Path: "/test", Status: i + 100})
	}
	recent := logger.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries (cap), got %d", len(recent))
	}
	// 应该是最后 3 条：status 102, 103, 104
	expected := []int{102, 103, 104}
	for i, entry := range recent {
		if entry.Status != expected[i] {
			t.Errorf("entry %d: expected status %d, got %d", i, expected[i], entry.Status)
		}
	}
}

func TestRingBufferConcurrent(t *testing.T) {
	logger := NewInterceptLogger(100)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Log(InterceptEntry{Method: "GET", Host: "example.com", Path: "/test", Status: n})
		}(i)
	}
	wg.Wait()
	recent := logger.Recent(100)
	if len(recent) != 100 {
		t.Fatalf("expected 100 entries, got %d", len(recent))
	}
}

func TestRingBufferRecentZero(t *testing.T) {
	logger := NewInterceptLogger(5)
	recent := logger.Recent(0)
	if len(recent) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(recent))
	}
}

func TestRingBufferRecentMoreThanAvailable(t *testing.T) {
	logger := NewInterceptLogger(10)
	logger.Log(InterceptEntry{Method: "GET", Host: "example.com", Path: "/test", Status: 200})
	recent := logger.Recent(5)
	if len(recent) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(recent))
	}
}
