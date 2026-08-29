package skills

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Skill struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourceURL   string `json:"source_url,omitempty"`
	InstallPath string `json:"install_path"`
	InstallType string `json:"install_type"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type Service struct {
	mu     sync.RWMutex
	skills []Skill
	path   string
}

func NewService() *Service {
	homeDir, _ := os.UserHomeDir()
	path := filepath.Join(homeDir, ".seasagi", "skills.json")
	svc := &Service{
		skills: make([]Skill, 0),
		path:   path,
	}
	svc.load()
	return svc
}

func (s *Service) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var loaded []Skill
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	s.skills = loaded
}

func (s *Service) persist() {
	data, err := json.MarshalIndent(s.skills, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(s.path), 0755)
	tmpFile := s.path + ".tmp"
	_ = os.WriteFile(tmpFile, data, 0644)
	_ = os.Rename(tmpFile, s.path)
}

func (s *Service) ListSkills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Skill, len(s.skills))
	copy(result, s.skills)
	return result
}

func (s *Service) InstallFromGitHub(repoURL, name string) error {
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	os.MkdirAll(skillsDir, 0755)

	installPath := filepath.Join(skillsDir, name)

	if _, err := os.Stat(installPath); err == nil {
		_ = os.RemoveAll(installPath)
	}

	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, installPath)
	if err := cmd.Run(); err != nil {
		return err
	}

	skill := Skill{
		Name:        name,
		Source:      "github",
		SourceURL:   repoURL,
		InstallPath: installPath,
		InstallType: "clone",
		Enabled:     true,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.skills {
		if existing.Name == name {
			s.skills[i] = skill
			s.persist()
			return nil
		}
	}
	s.skills = append(s.skills, skill)
	s.persist()
	return nil
}

func (s *Service) InstallFromLocal(srcPath, name string) error {
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	os.MkdirAll(skillsDir, 0755)

	installPath := filepath.Join(skillsDir, name)

	cmd := exec.Command("ln", "-sf", srcPath, installPath)
	if err := cmd.Run(); err != nil {
		cmd2 := exec.Command("cp", "-r", srcPath, installPath)
		if err2 := cmd2.Run(); err2 != nil {
			return err2
		}
	}

	skill := Skill{
		Name:        name,
		Source:      "local",
		SourceURL:   srcPath,
		InstallPath: installPath,
		InstallType: "symlink",
		Enabled:     true,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.skills {
		if existing.Name == name {
			s.skills[i] = skill
			s.persist()
			return nil
		}
	}
	s.skills = append(s.skills, skill)
	s.persist()
	return nil
}

func (s *Service) UninstallSkill(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.skills {
		if existing.Name == name {
			if existing.InstallPath != "" {
				if strings.HasPrefix(existing.InstallType, "symlink") {
					_ = os.Remove(existing.InstallPath)
				} else {
					_ = os.RemoveAll(existing.InstallPath)
				}
			}
			s.skills = append(s.skills[:i], s.skills[i+1:]...)
			s.persist()
			return nil
		}
	}
	return nil
}

func (s *Service) ToggleSkill(name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.skills {
		if existing.Name == name {
			s.skills[i].Enabled = enabled
			s.persist()
			return nil
		}
	}
	return nil
}

func (s *Service) ScanSkillsDir() ([]Skill, error) {
	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".claude", "skills")

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var found []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		found = append(found, Skill{
			Name:        name,
			Source:      "local",
			InstallPath: filepath.Join(skillsDir, name),
			InstallType: "existing",
			Enabled:     true,
		})
	}
	return found, nil
}
