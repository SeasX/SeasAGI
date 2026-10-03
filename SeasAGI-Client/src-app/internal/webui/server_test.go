package webui

import (
	"context"
	"embed"
	"testing"
)

func TestURL(t *testing.T) {
	s := NewServer(embed.FS{}, 17600)
	if got := s.URL(); got != "http://127.0.0.1:17600" {
		t.Fatalf("URL() = %q, want %q", got, "http://127.0.0.1:17600")
	}
}

func TestStopBeforeStartIsSafe(t *testing.T) {
	s := NewServer(embed.FS{}, 0)
	if s.IsRunning() {
		t.Fatal("expected server not running before Start")
	}
	s.Stop()
	if s.IsRunning() {
		t.Fatal("expected server not running after Stop")
	}
}

func TestStartAndStop(t *testing.T) {
	s := NewServer(embed.FS{}, 0) // port 0 => OS picks a free port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !s.IsRunning() {
		t.Fatal("expected server to be running after Start")
	}
	s.Stop()
	if s.IsRunning() {
		t.Fatal("expected server to stop after Stop")
	}
}
