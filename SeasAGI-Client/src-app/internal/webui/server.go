package webui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync"

	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
)

type Server struct {
	mu     sync.Mutex
	server *http.Server
	port   int
	assets embed.FS
}

func NewServer(assets embed.FS, port int) *Server {
	return &Server{
		assets: assets,
		port:   port,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, err := fs.Sub(s.assets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem: %w", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":    "SeasAGI Web UI",
			"version": "0.1.5",
			"mode":    "web",
		})
	})

	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	server := &http.Server{
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
	s.server = server

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	go func() {
		logging.Infof("SeasAGI Web UI listening on http://127.0.0.1:%d", s.port)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Errorf("Web UI server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
}

func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.server != nil
}

func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}
