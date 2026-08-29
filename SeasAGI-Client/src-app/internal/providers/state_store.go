package providers

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StateStore persists PenaltyManager and CooldownManager state to SQLite.
// This allows penalty/cooldown entries to survive gateway restarts.
type StateStore struct {
	mu     sync.Mutex
	db     *sql.DB
	closed bool
}

// NewStateStore opens (or creates) a SQLite database at the given path
// and initializes the penalty/cooldown tables.
func NewStateStore(dbPath string) (*StateStore, error) {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("state store: resolve home: %w", err)
		}
		dbPath = filepath.Join(home, "Library", "Application Support", "SeasAGI", "state.db")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("state store: mkdir: %w", err)
	}

	db, err := openSQLite(dbPath)
	if err != nil {
		return nil, fmt.Errorf("state store: open: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &StateStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state store: migrate: %w", err)
	}
	return store, nil
}

func (s *StateStore) migrate() error {
	_, err := s.db.Exec(penaltySchemaSQL)
	return err
}

func (s *StateStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

// SaveAll persists both penalty and cooldown state.
// This should be called periodically (e.g. every 30s) and on shutdown.
func (s *StateStore) SaveAll(pm *PenaltyManager, cm *CooldownManager) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := pm.PersistToDB(s.db); err != nil {
		return fmt.Errorf("save penalty: %w", err)
	}
	if err := cm.PersistToDB(s.db); err != nil {
		return fmt.Errorf("save cooldown: %w", err)
	}
	return nil
}

// LoadAll restores both penalty and cooldown state from the database.
// This should be called once during gateway startup.
func (s *StateStore) LoadAll(pm *PenaltyManager, cm *CooldownManager) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := pm.RestoreFromDB(s.db); err != nil {
		return fmt.Errorf("load penalty: %w", err)
	}
	if err := cm.RestoreFromDB(s.db); err != nil {
		return fmt.Errorf("load cooldown: %w", err)
	}
	return nil
}

// StartAutoSave launches a background goroutine that periodically saves state.
// Returns a stop function that performs a final save and stops the loop.
func (s *StateStore) StartAutoSave(pm *PenaltyManager, cm *CooldownManager, interval time.Duration) func() {
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.SaveAll(pm, cm)
			case <-stopCh:
				_ = s.SaveAll(pm, cm)
				return
			}
		}
	}()
	return func() { close(stopCh) }
}
