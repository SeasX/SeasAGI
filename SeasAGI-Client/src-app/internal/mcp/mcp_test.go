package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPServerStdio(t *testing.T) {
	server := NewGatewayServer()

	// 模拟 JSON-RPC initialize 请求
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
	}

	input, _ := json.Marshal(request)
	output := &bytes.Buffer{}

	err := server.HandleStdio(bytes.NewReader(input), output)
	if err != nil {
		t.Fatalf("stdio 处理失败: %v", err)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if response["jsonrpc"] != "2.0" {
		t.Fatal("jsonrpc 版本不匹配")
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("应包含 result")
	}

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("应包含 serverInfo")
	}
	if serverInfo["name"] != "seasagi-mcp" {
		t.Fatal("server name 不正确")
	}
}

func TestMCPServerSSE(t *testing.T) {
	server := NewGatewayServer()

	// SSE 传输本质上是 HTTP，这里测试 JSON-RPC handler 的兼容性
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	}

	response := server.handleJSONRPC(request)

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Fatal("应包含 result")
	}

	tools, ok := result["tools"].([]map[string]interface{})
	if !ok {
		t.Fatal("应包含 tools 列表")
	}

	if len(tools) < 19 {
		t.Fatalf("应至少有 19 个 tool，实际 %d", len(tools))
	}
}

// CL-S9：内置工具尚未接入真实数据源时必须显式返回 unimplemented，
// 不能返回硬编码的假健康度/假配额让外部 agent 误判。
func TestMCPBuiltinToolsReturnUnimplemented(t *testing.T) {
	server := NewGatewayServer()

	for _, name := range []string{"route_request", "check_quota", "list_models_catalog", "get_health"} {
		_, err := server.CallTool(name, map[string]interface{}{"model": "gpt-4o"})
		if err == nil {
			t.Fatalf("%s 应返回 unimplemented 错误", name)
		}
		if !strings.Contains(err.Error(), "not implemented") {
			t.Fatalf("%s 错误信息应说明未实现，实际: %v", name, err)
		}
	}

	// 未知工具仍应返回 tool not found。
	_, err := server.CallTool("nonexistent_tool", map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "tool not found") {
		t.Fatalf("未知工具应返回 tool not found，实际: %v", err)
	}
}

func TestMCPAuditLog(t *testing.T) {
	server := NewGatewayServer()

	// 已注册但未实现的工具
	server.CallTool("get_health", map[string]interface{}{})
	// 未知工具
	server.CallTool("nonexistent_tool", map[string]interface{}{})

	log := server.GetAuditLog()
	if len(log) != 2 {
		t.Fatalf("应有 2 条审计日志，实际 %d", len(log))
	}

	if log[0].ToolName != "get_health" || log[0].Success {
		t.Fatalf("第一条应为 get_health 且失败，实际 %+v", log[0])
	}
	if log[0].Error == "" {
		t.Fatal("失败调用应记录错误信息")
	}

	if log[1].ToolName != "nonexistent_tool" || log[1].Success {
		t.Fatalf("第二条应为 nonexistent_tool 且失败，实际 %+v", log[1])
	}
	if !strings.Contains(log[1].Error, "not found") {
		t.Fatal("错误信息应包含 'not found'")
	}
}

func TestMCPToolListContains19(t *testing.T) {
	server := NewGatewayServer()
	tools := server.ListTools()

	if len(tools) != 19 {
		t.Fatalf("应有 19 个核心 tool，实际 %d", len(tools))
	}

	// 验证关键 tool 存在
	expectedTools := []string{
		"get_health", "list_combos", "switch_combo", "check_quota",
		"route_request", "cost_report", "list_models_catalog",
	}
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Fatalf("缺少核心 tool: %s", name)
		}
	}
}
