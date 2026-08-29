package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Session struct {
	ID        string    `json:"id"`
	App       string    `json:"app"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Model     string    `json:"model,omitempty"`
	MessageCount int   `json:"message_count"`
	Path      string    `json:"path"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model,omitempty"`
}

type Service struct {
	mu sync.RWMutex
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) ListSessions(app string, limit int) ([]Session, error) {
	homeDir, _ := os.UserHomeDir()
	var sessionsDir string

	switch app {
	case "claude":
		sessionsDir = filepath.Join(homeDir, ".claude", "projects")
	case "codex":
		sessionsDir = filepath.Join(homeDir, ".codex", "sessions")
	case "gemini":
		sessionsDir = filepath.Join(homeDir, ".gemini", "sessions")
	default:
		return nil, nil
	}

	var sessions []Session
	err := filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".json") || strings.HasSuffix(info.Name(), ".jsonl") {
			session := Session{
				ID:        strings.TrimSuffix(info.Name(), filepath.Ext(info.Name())),
				App:       app,
				Title:     strings.TrimSuffix(info.Name(), filepath.Ext(info.Name())),
				UpdatedAt: info.ModTime(),
				CreatedAt: info.ModTime(),
				Path:      path,
			}
			session = s.enrichSession(session, path)
			sessions = append(sessions, session)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

func (s *Service) enrichSession(session Session, path string) Session {
	data, err := os.ReadFile(path)
	if err != nil {
		return session
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return session
	}

	if title, ok := raw["title"].(string); ok && title != "" {
		session.Title = title
	}
	if model, ok := raw["model"].(string); ok {
		session.Model = model
	}
	if created, ok := raw["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			session.CreatedAt = t
		}
	}
	if messages, ok := raw["messages"].([]interface{}); ok {
		session.MessageCount = len(messages)
	}
	return session
}

func (s *Service) GetSession(app, sessionID string) (*Session, error) {
	homeDir, _ := os.UserHomeDir()
	var sessionsDir string

	switch app {
	case "claude":
		sessionsDir = filepath.Join(homeDir, ".claude", "projects")
	case "codex":
		sessionsDir = filepath.Join(homeDir, ".codex", "sessions")
	case "gemini":
		sessionsDir = filepath.Join(homeDir, ".gemini", "sessions")
	default:
		return nil, nil
	}

	var foundPath string
	err := filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		baseName := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		if baseName == sessionID {
			foundPath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return nil, err
	}

	if foundPath == "" {
		return nil, nil
	}

	session := &Session{
		ID:   sessionID,
		App:  app,
		Path: foundPath,
	}
	*session = s.enrichSession(*session, foundPath)
	return session, nil
}

func (s *Service) GetMessages(app, sessionID string) ([]Message, error) {
	session, err := s.GetSession(app, sessionID)
	if err != nil || session == nil {
		return nil, err
	}

	data, err := os.ReadFile(session.Path)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var messages []Message
	if rawMessages, ok := raw["messages"].([]interface{}); ok {
		for _, m := range rawMessages {
			if msg, ok := m.(map[string]interface{}); ok {
				message := Message{}
				if role, ok := msg["role"].(string); ok {
					message.Role = role
				}
				if content, ok := msg["content"].(string); ok {
					message.Content = content
				}
				if model, ok := msg["model"].(string); ok {
					message.Model = model
				}
				messages = append(messages, message)
			}
		}
	}

	return messages, nil
}

func (s *Service) SearchSessions(app, query string, limit int) ([]Session, error) {
	sessions, err := s.ListSessions(app, 0)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var filtered []Session
	for _, session := range sessions {
		if strings.Contains(strings.ToLower(session.Title), query) ||
			strings.Contains(strings.ToLower(session.Model), query) {
			filtered = append(filtered, session)
		}
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

func (s *Service) DeleteSession(app, sessionID string) error {
	session, err := s.GetSession(app, sessionID)
	if err != nil || session == nil {
		return err
	}
	return os.Remove(session.Path)
}
