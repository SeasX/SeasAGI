package mitm

import (
	"runtime"
	"testing"
)

func TestDetectShellBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not applicable on windows")
	}
	t.Setenv("SHELL", "/bin/bash")
	hint := DetectShellEnv("127.0.0.1:8080")
	if hint.Shell != "bash" {
		t.Errorf("expected shell bash, got %s", hint.Shell)
	}
	if hint.ExportCmds == "" {
		t.Error("export cmds should not be empty")
	}
	if hint.UnsetCmds == "" {
		t.Error("unset cmds should not be empty")
	}
	if !contains(hint.ExportCmds, "127.0.0.1:8080") {
		t.Errorf("export cmds should contain proxy addr: %s", hint.ExportCmds)
	}
}

func TestDetectShellZsh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not applicable on windows")
	}
	t.Setenv("SHELL", "/bin/zsh")
	hint := DetectShellEnv("127.0.0.1:8080")
	if hint.Shell != "zsh" {
		t.Errorf("expected shell zsh, got %s", hint.Shell)
	}
}

func TestDetectShellFish(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not applicable on windows")
	}
	t.Setenv("SHELL", "/usr/bin/fish")
	hint := DetectShellEnv("127.0.0.1:8080")
	if hint.Shell != "fish" {
		t.Errorf("expected shell fish, got %s", hint.Shell)
	}
	if !contains(hint.ExportCmds, "set -x") {
		t.Errorf("fish export should use 'set -x': %s", hint.ExportCmds)
	}
}

func TestDetectShellUnknown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not applicable on windows")
	}
	t.Setenv("SHELL", "/usr/bin/unknownshell")
	hint := DetectShellEnv("127.0.0.1:8080")
	// 未知 shell 应回退到 bash 语法
	if hint.Shell != "bash" {
		t.Errorf("expected fallback to bash, got %s", hint.Shell)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
