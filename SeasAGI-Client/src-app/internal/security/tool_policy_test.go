package security

import "testing"

func TestToolPolicyDisabled(t *testing.T) {
	p := NewToolPolicy()
	result := p.Evaluate([]string{"any_tool", "dangerous_tool"})
	if !result.Allowed {
		t.Fatal("disabled policy should allow all tools")
	}
	if len(result.Denied) != 0 {
		t.Fatal("disabled policy should not deny any tools")
	}
}

func TestToolPolicyAllowlist(t *testing.T) {
	p := NewToolPolicy()
	p.SetMode(ToolPolicyAllowlist)
	p.SetAllowlist([]string{"safe_tool", "read_file"})

	result := p.Evaluate([]string{"safe_tool", "dangerous_tool"})
	if result.Allowed {
		t.Fatal("allowlist should deny unlisted tools")
	}
	if len(result.Denied) != 1 || result.Denied[0] != "dangerous_tool" {
		t.Fatalf("expected dangerous_tool denied, got %v", result.Denied)
	}

	// 全部在白名单中
	result = p.Evaluate([]string{"safe_tool", "read_file"})
	if !result.Allowed {
		t.Fatal("allowlist should allow listed tools")
	}
}

func TestToolPolicyDenylist(t *testing.T) {
	p := NewToolPolicy()
	p.SetMode(ToolPolicyDenylist)
	p.SetDenylist([]string{"rm_rf", "exec_shell"})

	result := p.Evaluate([]string{"safe_tool", "rm_rf"})
	if result.Allowed {
		t.Fatal("denylist should deny listed tools")
	}
	if len(result.Denied) != 1 || result.Denied[0] != "rm_rf" {
		t.Fatalf("expected rm_rf denied, got %v", result.Denied)
	}

	// 不在黑名单中
	result = p.Evaluate([]string{"safe_tool"})
	if !result.Allowed {
		t.Fatal("denylist should allow unlisted tools")
	}
}

func TestToolPolicyCaseInsensitive(t *testing.T) {
	p := NewToolPolicy()
	p.SetMode(ToolPolicyAllowlist)
	p.SetAllowlist([]string{"Safe_Tool"})

	result := p.Evaluate([]string{"safe_tool", "SAFE_TOOL"})
	if !result.Allowed {
		t.Fatalf("allowlist should be case insensitive, denied: %v", result.Denied)
	}
}

func TestToolPolicyExtractTools(t *testing.T) {
	body := map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "get_weather",
				},
			},
			map[string]interface{}{
				"type": "code_interpreter",
			},
		},
		"functions": []interface{}{
			map[string]interface{}{"name": "old_format_tool"},
		},
		"tool_choice": map[string]interface{}{
			"function": map[string]interface{}{"name": "forced_tool"},
		},
	}

	names := ExtractToolNames(body)
	expected := map[string]bool{"get_weather": true, "code_interpreter": true, "old_format_tool": true, "forced_tool": true}
	if len(names) != len(expected) {
		t.Fatalf("expected %d tools, got %d: %v", len(expected), len(names), names)
	}
	for _, n := range names {
		if !expected[n] {
			t.Fatalf("unexpected tool name: %s", n)
		}
	}
}

func TestToolPolicyRuntimeOverride(t *testing.T) {
	p := NewToolPolicy()
	// 默认 disabled → 全部允许
	result := p.Evaluate([]string{"any_tool"})
	if !result.Allowed {
		t.Fatal("disabled should allow all")
	}

	// 运行时切换到 denylist
	p.SetMode(ToolPolicyDenylist)
	p.SetDenylist([]string{"blocked"})
	result = p.Evaluate([]string{"blocked"})
	if result.Allowed {
		t.Fatal("denylist should block listed tool")
	}

	// 运行时切换回 disabled
	p.SetMode(ToolPolicyDisabled)
	result = p.Evaluate([]string{"blocked"})
	if !result.Allowed {
		t.Fatal("disabled should allow all after override back")
	}
}

func TestToolPolicyValidateRequest(t *testing.T) {
	p := NewToolPolicy()
	p.SetMode(ToolPolicyDenylist)
	p.SetDenylist([]string{"exec_shell"})

	body := map[string]interface{}{
		"tools": []interface{}{
			map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "exec_shell",
				},
			},
		},
	}

	result := p.ValidateToolsInRequest(body)
	if result.Allowed {
		t.Fatal("should deny exec_shell from request body")
	}
}
