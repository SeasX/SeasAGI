package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return &Service{
		skills: []Skill{
			{Name: "alpha", Source: "github", InstallPath: filepath.Join(t.TempDir(), "alpha"), InstallType: "clone", Enabled: true},
			{Name: "beta", Source: "local", InstallPath: filepath.Join(t.TempDir(), "beta"), InstallType: "symlink", Enabled: false},
		},
		path: filepath.Join(t.TempDir(), "skills.json"),
	}
}

func TestListSkillsReturnsCopy(t *testing.T) {
	s := newTestService(t)
	got := s.ListSkills()
	if len(got) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(got))
	}
	got[0].Name = "mutated"
	if s.ListSkills()[0].Name != "alpha" {
		t.Fatal("ListSkills must return a copy, not internal slice")
	}
}

func TestToggleSkillPersists(t *testing.T) {
	s := newTestService(t)
	if err := s.ToggleSkill("beta", true); err != nil {
		t.Fatalf("ToggleSkill error: %v", err)
	}
	// Reload from disk to verify persistence.
	reloaded := &Service{path: s.path}
	reloaded.load()
	for _, sk := range reloaded.skills {
		if sk.Name == "beta" && !sk.Enabled {
			t.Fatal("expected beta to be enabled after toggle + reload")
		}
	}
	if err := s.ToggleSkill("missing", true); err != nil {
		t.Fatalf("ToggleSkill on missing name should be a no-op, got %v", err)
	}
}

func TestUninstallSkillRemovesEntry(t *testing.T) {
	s := newTestService(t)
	if err := s.UninstallSkill("alpha"); err != nil {
		t.Fatalf("UninstallSkill error: %v", err)
	}
	remaining := s.ListSkills()
	if len(remaining) != 1 || remaining[0].Name != "beta" {
		t.Fatalf("expected only beta to remain, got %+v", remaining)
	}
	if _, err := os.Stat(filepath.Join(s.path)); err != nil {
		t.Fatalf("expected skills file persisted: %v", err)
	}
	if err := s.UninstallSkill("nope"); err != nil {
		t.Fatalf("UninstallSkill on missing name should be a no-op, got %v", err)
	}
}

func TestNewServiceLoadsWithoutFile(t *testing.T) {
	// NewService reads the real user config path; a missing file must not panic.
	svc := NewService()
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	_ = svc.ListSkills()
}

func TestValidateSkillName(t *testing.T) {
	for _, name := range []string{"my-skill_1", "a", "A0", "skill.v2"} {
		if err := validateSkillName(name); err != nil {
			t.Errorf("expected %q to be valid, got %v", name, err)
		}
	}
	for _, name := range []string{"../escape", "a/b", "-x", ".", "..", "", "a b"} {
		if err := validateSkillName(name); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestResolveInstallPathRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveInstallPath(dir, "../escape"); err == nil {
		t.Fatal("expected path traversal name to be rejected")
	}
	p, err := resolveInstallPath(dir, "my-skill_1")
	if err != nil {
		t.Fatalf("resolveInstallPath: %v", err)
	}
	if want := filepath.Join(dir, "my-skill_1"); p != want {
		t.Errorf("install path = %q, want %q", p, want)
	}
}

// TestInstallFromGitHubRejectsInvalidRepoURL 确保非 github / 非 https / userinfo
// 与以 "-" 开头的输入都在创建任何目录之前被拒绝。
func TestInstallFromGitHubRejectsInvalidRepoURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := NewService()

	cases := []string{
		"http://github.com/a/b",
		"https://evil.com/a/b",
		"file:///etc",
		"-x",
		"https://user:pass@github.com/a/b",
		"https://github.com/only-owner",
	}
	for _, repo := range cases {
		if err := svc.InstallFromGitHub(repo, "evil-skill"); err == nil {
			t.Fatalf("expected %q to be rejected", repo)
		}
		if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "evil-skill")); !os.IsNotExist(err) {
			t.Fatalf("install dir must not be created for %q", repo)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatal("skills dir must not be created when validation fails")
	}
}

// TestInstallFromGitHubRejectsInvalidNameWithoutCreatingDir 确保路径穿越名称
// 不会创建技能目录。
func TestInstallFromGitHubRejectsInvalidNameWithoutCreatingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := NewService()

	if err := svc.InstallFromGitHub("https://github.com/owner/repo", "../escape"); err == nil {
		t.Fatal("expected path traversal name to be rejected")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatal("skills dir must not be created when validation fails")
	}
}
