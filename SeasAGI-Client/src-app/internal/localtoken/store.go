package localtoken

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const tokenLength = 32

type Store struct {
	db *sql.DB
}

func NewStore() (*Store, error) {
	path, err := defaultDBPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) GetOrCreate() (string, error) {
	token, err := s.Get()
	if err == nil && token != "" {
		return token, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	token = generateToken()
	return token, s.upsert(token)
}

func (s *Store) Get() (string, error) {
	var token string
	err := s.db.QueryRow(`SELECT token FROM local_tokens WHERE token_name = 'gateway'`).Scan(&token)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) Reset() (string, error) {
	token := generateToken()
	return token, s.upsert(token)
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS local_tokens (
			token_name TEXT PRIMARY KEY,
			token TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (s *Store) upsert(token string) error {
	_, err := s.db.Exec(`
		INSERT INTO local_tokens (token_name, token, updated_at)
		VALUES ('gateway', ?, ?)
		ON CONFLICT(token_name) DO UPDATE SET token=excluded.token, updated_at=excluded.updated_at
	`, token, time.Now().UTC().Format(time.RFC3339))
	return err
}

func defaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "SeasAGI", "local.db"), nil
}

func generateToken() string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, tokenLength)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}
