package protocol

type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ToolSet struct {
	Tools []ToolDef `json:"tools"`
}

var builtinToolEquivalents = map[string][]string{
	"read_file":      {"filesystem_read", "fs_read", "file_read", "readFile"},
	"write_file":     {"filesystem_write", "fs_write", "file_write", "writeFile"},
	"list_directory": {"filesystem_list", "fs_ls", "dir_list", "listDir"},
	"search_files":   {"filesystem_search", "fs_search", "file_search", "searchFiles"},
	"create_file":    {"filesystem_create", "fs_create", "file_create", "createFile"},
	"delete_file":    {"filesystem_delete", "fs_delete", "file_delete", "deleteFile"},
	"shell_exec":     {"terminal_exec", "bash_exec", "run_command", "execute_command", "shell"},
	"web_search":     {"search_web", "webSearch", "google_search", "bing_search"},
	"web_fetch":      {"fetch_url", "http_get", "url_fetch", "webFetch"},
	"git_status":     {"vcs_status", "scm_status"},
	"git_diff":       {"vcs_diff", "scm_diff"},
	"git_log":        {"vcs_log", "scm_log"},
}

func DedupTools(mcpTools []ToolDef, builtinTools []ToolDef) []ToolDef {
	mcpNames := make(map[string]bool)
	for _, t := range mcpTools {
		mcpNames[t.Name] = true
	}

	aliasesCovered := make(map[string]bool)
	for mcpName := range mcpNames {
		for builtinName, aliases := range builtinToolEquivalents {
			if aliasesCovered[builtinName] {
				continue
			}
			if mcpName == builtinName {
				aliasesCovered[builtinName] = true
				continue
			}
			for _, alias := range aliases {
				if mcpName == alias {
					aliasesCovered[builtinName] = true
					break
				}
			}
		}
	}

	result := make([]ToolDef, 0, len(builtinTools))
	for _, bt := range builtinTools {
		if aliasesCovered[bt.Name] {
			continue
		}
		result = append(result, bt)
	}

	result = append(result, mcpTools...)
	return result
}

// DedupToolsInRequest 在请求体层面按「工具名」去重：仅当已存在等价的 MCP 工具时，
// 才移除内建工具。原始工具对象（input_schema / function 外壳 / cache_control 等）
// 全部原样保留，不做任何重建，避免丢失 schema 细节。
func DedupToolsInRequest(body map[string]any) map[string]any {
	toolsRaw, ok := body["tools"]
	if !ok {
		return body
	}

	toolsList, ok := toolsRaw.([]interface{})
	if !ok || len(toolsList) == 0 {
		return body
	}

	// 收集被 MCP 工具覆盖的内建工具名（只做名称级匹配）。
	covered := make(map[string]bool)
	hasMCP := false
	for _, raw := range toolsList {
		toolMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := toolName(toolMap)
		if name == "" || !isMCPTool(name) {
			continue
		}
		hasMCP = true
		for builtinName, aliases := range builtinToolEquivalents {
			if name == builtinName {
				covered[builtinName] = true
				continue
			}
			for _, alias := range aliases {
				if name == alias {
					covered[builtinName] = true
					break
				}
			}
		}
	}
	if !hasMCP || len(covered) == 0 {
		return body
	}

	kept := make([]interface{}, 0, len(toolsList))
	seen := make(map[string]bool, len(toolsList))
	for _, raw := range toolsList {
		toolMap, ok := raw.(map[string]any)
		if !ok {
			kept = append(kept, raw)
			continue
		}
		name := toolName(toolMap)
		if name != "" && seen[name] {
			continue // 同名工具只保留首次出现
		}
		if covered[name] && !isMCPTool(name) {
			continue // 已被等价 MCP 工具覆盖的内建工具
		}
		if name != "" {
			seen[name] = true
		}
		kept = append(kept, toolMap)
	}
	if len(kept) == len(toolsList) {
		return body
	}

	result := make(map[string]any, len(body))
	for k, v := range body {
		result[k] = v
	}
	result["tools"] = kept
	return result
}

// toolName 提取工具名，兼容 OpenAI 的 {"type":"function","function":{"name":...}} 外壳。
func toolName(tool map[string]any) string {
	if name, ok := tool["name"].(string); ok && name != "" {
		return name
	}
	if fn, ok := tool["function"].(map[string]any); ok {
		if name, ok := fn["name"].(string); ok {
			return name
		}
	}
	return ""
}

func isMCPTool(name string) bool {
	prefixes := []string{"mcp_", "mcp-", "filesystem_", "fs_", "vcs_", "scm_", "terminal_", "search_", "fetch_"}
	for _, p := range prefixes {
		if len(name) > len(p) && name[:len(p)] == p {
			return true
		}
	}
	_, isBuiltin := builtinToolEquivalents[name]
	if isBuiltin {
		return false
	}
	for _, aliases := range builtinToolEquivalents {
		for _, alias := range aliases {
			if name == alias {
				return true
			}
		}
	}
	return false
}
