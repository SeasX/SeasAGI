package prompts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type PromptPreset struct {
	Name        string `json:"name"`
	Content     string `json:"content"`
	TargetApp   string `json:"target_app"`
	IsDefault   bool   `json:"is_default"`
	Description string `json:"description,omitempty"`
}

type Service struct {
	mu      sync.RWMutex
	presets []PromptPreset
	path    string
}

func NewService() *Service {
	homeDir, _ := os.UserHomeDir()
	path := filepath.Join(homeDir, ".seasagi", "prompt_presets.json")
	svc := &Service{
		presets: make([]PromptPreset, 0),
		path:    path,
	}
	svc.load()
	return svc
}

func (s *Service) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var loaded []PromptPreset
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	s.presets = loaded
}

func (s *Service) persist() {
	data, err := json.MarshalIndent(s.presets, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(s.path), 0755)
	tmpFile := s.path + ".tmp"
	_ = os.WriteFile(tmpFile, data, 0644)
	_ = os.Rename(tmpFile, s.path)
}

func (s *Service) ListPresets() []PromptPreset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]PromptPreset, len(s.presets))
	copy(result, s.presets)
	return result
}

func (s *Service) SavePreset(preset PromptPreset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.presets {
		if existing.Name == preset.Name && existing.TargetApp == preset.TargetApp {
			s.presets[i] = preset
			s.persist()
			s.syncToApp(preset.TargetApp)
			return nil
		}
	}
	s.presets = append(s.presets, preset)
	s.persist()
	s.syncToApp(preset.TargetApp)
	return nil
}

func (s *Service) DeletePreset(name, targetApp string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.presets {
		if existing.Name == name && existing.TargetApp == targetApp {
			s.presets = append(s.presets[:i], s.presets[i+1:]...)
			s.persist()
			return nil
		}
	}
	return nil
}

func (s *Service) ApplyPreset(name, targetApp string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, preset := range s.presets {
		if preset.Name == name && preset.TargetApp == targetApp {
			return s.writeToAppFile(targetApp, preset.Content)
		}
	}
	return nil
}

func (s *Service) ReadCurrentPrompt(targetApp string) (string, error) {
	homeDir, _ := os.UserHomeDir()
	var filePath string
	switch targetApp {
	case "claude":
		filePath = filepath.Join(homeDir, "CLAUDE.md")
	case "codex":
		filePath = filepath.Join(homeDir, "AGENTS.md")
	case "gemini":
		filePath = filepath.Join(homeDir, "GEMINI.md")
	default:
		return "", nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", nil
	}
	return string(data), nil
}

func (s *Service) writeToAppFile(targetApp, content string) error {
	homeDir, _ := os.UserHomeDir()
	var filePath string
	switch targetApp {
	case "claude":
		filePath = filepath.Join(homeDir, "CLAUDE.md")
	case "codex":
		filePath = filepath.Join(homeDir, "AGENTS.md")
	case "gemini":
		filePath = filepath.Join(homeDir, "GEMINI.md")
	default:
		return nil
	}
	return os.WriteFile(filePath, []byte(content), 0644)
}

func (s *Service) syncToApp(targetApp string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, preset := range s.presets {
		if preset.TargetApp == targetApp && preset.IsDefault {
			_ = s.writeToAppFile(targetApp, preset.Content)
			return
		}
	}
}
