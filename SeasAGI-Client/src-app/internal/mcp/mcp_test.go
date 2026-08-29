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

func TestMCPToolRoute(t *testing.T) {
	server := NewGatewayServer()

	result, err := server.CallTool("route_request", map[string]interface{}{
		"model": "gpt-4o",
	})
	if err != nil {
		t.Fatalf("route_request 调用失败: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("结果应为 map")
	}
	if resultMap["model"] != "gpt-4o" {
		t.Fatal("model 不匹配")
	}

	// 缺少 model 参数应报错
	_, err = server.CallTool("route_request", map[string]interface{}{})
	if err == nil {
		t.Fatal("缺少 model 参数应报错")
	}
}

func TestMCPToolQuota(t *testing.T) {
	server := NewGatewayServer()

	result, err := server.CallTool("check_quota", map[string]interface{}{})
	if err != nil {
		t.Fatalf("check_quota 调用失败: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("结果应为 map")
	}
	if resultMap["remaining"] == nil {
		t.Fatal("应包含 remaining 字段")
	}
}

func TestMCPToolCache(t *testing.T) {
	server := NewGatewayServer()

	// 测试 list_models_catalog（核心 tool 之一）
	result, err := server.CallTool("list_models_catalog", map[string]interface{}{})
	if err != nil {
		t.Fatalf("list_models_catalog 调用失败: %v", err)
	}

	resultList, ok := result.([]map[string]interface{})
	if !ok {
		t.Fatal("结果应为列表")
	}
	if len(resultList) == 0 {
		t.Fatal("应至少有 1 个模型")
	}
}

func TestMCPAuditLog(t *testing.T) {
	server := NewGatewayServer()

	// 成功调用
	server.CallTool("get_health", map[string]interface{}{})
	// 失败调用
	server.CallTool("nonexistent_tool", map[string]interface{}{})

	log := server.GetAuditLog()
	if len(log) != 2 {
		t.Fatalf("应有 2 条审计日志，实际 %d", len(log))
	}

	// 第一条应成功
	if !log[0].Success {
		t.Fatal("第一条日志应成功")
	}

	// 第二条应失败
	if log[1].Success {
		t.Fatal("第二条日志应失败")
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
