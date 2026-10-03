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
