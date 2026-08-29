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

func DedupToolsInRequest(body map[string]any) map[string]any {
	toolsRaw, ok := body["tools"]
	if !ok {
		return body
	}

	toolsList, ok := toolsRaw.([]interface{})
	if !ok || len(toolsList) == 0 {
		return body
	}

	allTools := make([]ToolDef, 0, len(toolsList))
	for _, t := range toolsList {
		toolMap, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := toolMap["name"].(string)
		desc, _ := toolMap["description"].(string)
		params, _ := toolMap["parameters"].(map[string]any)
		allTools = append(allTools, ToolDef{
			Name:        name,
			Description: desc,
			Parameters:  params,
		})
	}

	mcpTools := make([]ToolDef, 0)
	builtinTools := make([]ToolDef, 0)
	for _, t := range allTools {
		if isMCPTool(t.Name) {
			mcpTools = append(mcpTools, t)
		} else {
			builtinTools = append(builtinTools, t)
		}
	}

	if len(mcpTools) == 0 {
		return body
	}

	deduped := DedupTools(mcpTools, builtinTools)

	result := make(map[string]any, len(body))
	for k, v := range body {
		result[k] = v
	}

	dedupedRaw := make([]interface{}, 0, len(deduped))
	for _, t := range deduped {
		toolMap := map[string]any{
			"name":        t.Name,
			"description": t.Description,
		}
		if t.Parameters != nil {
			toolMap["parameters"] = t.Parameters
		}
		dedupedRaw = append(dedupedRaw, toolMap)
	}
	result["tools"] = dedupedRaw

	return result
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
