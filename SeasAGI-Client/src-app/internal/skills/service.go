package skills

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// skillNamePattern 限制技能名只能是单段安全文件名，禁止路径分隔符与隐藏前缀。
var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// validateSkillName 校验技能名，拒绝 "."、".." 以及任何可用作路径穿越的名称。
func validateSkillName(name string) error {
	name = strings.TrimSpace(name)
	if name == "." || name == ".." || !skillNamePattern.MatchString(name) {
		return fmt.Errorf("invalid skill name: %q", name)
	}
	return nil
}

// resolveInstallPath 在 skillsDir 下解析安装路径，并复核结果不会逃逸出该目录。
func resolveInstallPath(skillsDir, name string) (string, error) {
	if err := validateSkillName(name); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	p := filepath.Join(skillsDir, name)
	rel, err := filepath.Rel(skillsDir, p)
	if err != nil {
		return "", fmt.Errorf("resolve install path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install path escapes skills dir: %q", name)
	}
	return p, nil
}

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
	// 技能清单可能包含本地路径等隐私信息，目录/文件仅限当前用户访问。
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return
	}
	tmpFile := s.path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return
	}
	_ = os.Chmod(tmpFile, 0o600)
	_ = os.Rename(tmpFile, s.path)
}

func (s *Service) ListSkills() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Skill, len(s.skills))
	copy(result, s.skills)
	return result
}

// validateGitHubRepoURL 只接受 https://github.com/<owner>/<repo>[.git] 形式的仓库地址，
// 拒绝其它协议、主机与 userinfo，避免把任意输入交给 git 或触发凭证泄露。
func validateGitHubRepoURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("repo url is empty")
	}
	if strings.HasPrefix(rawURL, "-") {
		return fmt.Errorf("invalid repo url: %q", rawURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid repo url: %w", err)
	}
	if u.User != nil {
		return fmt.Errorf("repo url must not contain userinfo")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("repo url must use https: %s", rawURL)
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return fmt.Errorf("only github.com repositories are allowed: %s", u.Host)
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return fmt.Errorf("repo url must contain owner and repo: %s", rawURL)
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("invalid repo url path: %s", rawURL)
		}
	}
	return nil
}

func (s *Service) InstallFromGitHub(repoURL, name string) error {
	repoURL = strings.TrimSpace(repoURL)
	if err := validateGitHubRepoURL(repoURL); err != nil {
		return err
	}

	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	// 先完成名称/路径校验，校验失败时不得创建技能目录。
	installPath, err := resolveInstallPath(skillsDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(installPath); err == nil {
		_ = os.RemoveAll(installPath)
	}

	// "--" 终止选项解析，防止仓库地址或路径被 git 当作参数。
	cmd := exec.Command("git", "clone", "--depth", "1", "--", repoURL, installPath)
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
	srcPath = strings.TrimSpace(srcPath)
	if !filepath.IsAbs(srcPath) {
		return fmt.Errorf("local skill path must be absolute: %q", srcPath)
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat local skill path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local skill path is not a directory: %s", srcPath)
	}

	homeDir, _ := os.UserHomeDir()
	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	// 先完成名称/路径校验，校验失败时不得创建技能目录。
	installPath, err := resolveInstallPath(skillsDir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}

	// "--" 终止选项解析，防止源路径或目标被当作命令行选项。
	cmd := exec.Command("ln", "-sf", "--", srcPath, installPath)
	if err := cmd.Run(); err != nil {
		cmd2 := exec.Command("cp", "-r", "--", srcPath, installPath)
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
