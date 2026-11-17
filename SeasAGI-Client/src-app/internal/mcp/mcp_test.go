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

// fakeDataSource 用内存数据验证工具确实读取注入的数据源。
type fakeDataSource struct {
	healthCalls int
	switched    string
}

func (f *fakeDataSource) Health() (map[string]interface{}, error) {
	f.healthCalls++
	return map[string]interface{}{"gateway_running": true, "active_channels": 3}, nil
}

func (f *fakeDataSource) ListCombos() ([]map[string]interface{}, error) {
	return []map[string]interface{}{{"name": "balanced", "strategy": "fallback"}}, nil
}

func (f *fakeDataSource) ComboMetrics(comboID string) (map[string]interface{}, error) {
	return map[string]interface{}{"combo_id": comboID, "total_requests": 42}, nil
}

func (f *fakeDataSource) SwitchCombo(comboID string) error {
	f.switched = comboID
	return nil
}

func (f *fakeDataSource) QuotaStatus() (map[string]interface{}, error) {
	return map[string]interface{}{"month_cost_usd": 1.25}, nil
}

func (f *fakeDataSource) RoutePreview(model string) (map[string]interface{}, error) {
	return map[string]interface{}{"model": model, "selected_channel": "ch-1"}, nil
}

func (f *fakeDataSource) CostReport() (map[string]interface{}, error) {
	return map[string]interface{}{"month_cost_usd": 1.25, "days": 30}, nil
}

func (f *fakeDataSource) ModelsCatalog() ([]map[string]interface{}, error) {
	return []map[string]interface{}{{"model": "gpt-4o", "provider": "openai"}}, nil
}

func (f *fakeDataSource) ProviderMetrics() ([]map[string]interface{}, error) {
	return []map[string]interface{}{{"provider": "openai", "healthy": true}}, nil
}

func (f *fakeDataSource) BestComboForTask(task string) (map[string]interface{}, error) {
	return map[string]interface{}{"task": task, "recommended": "balanced"}, nil
}

func (f *fakeDataSource) ExplainRoute(model string) (map[string]interface{}, error) {
	return map[string]interface{}{"model": model, "reason": "lowest penalty"}, nil
}

func (f *fakeDataSource) SessionSnapshot() (map[string]interface{}, error) {
	return map[string]interface{}{"version": "test"}, nil
}

func (f *fakeDataSource) DBHealthCheck() (map[string]interface{}, error) {
	return map[string]interface{}{"status": "ok"}, nil
}

func (f *fakeDataSource) SetBudgetGuard(args map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"monthly_cost_limit_usd": args["monthly_cost_limit_usd"]}, nil
}

func (f *fakeDataSource) SetRoutingStrategy(args map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"strategy": args["strategy"]}, nil
}

func (f *fakeDataSource) SetResilienceProfile(args map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "ok"}, nil
}

func (f *fakeDataSource) TestCombo(comboID string) (map[string]interface{}, error) {
	return map[string]interface{}{"combo_id": comboID, "reachable": true}, nil
}

func TestMCPBuiltinToolsUseDataSource(t *testing.T) {
	server := NewGatewayServer()
	ds := &fakeDataSource{}
	server.SetDataSource(ds)

	result, err := server.CallTool("get_health", map[string]interface{}{})
	if err != nil {
		t.Fatalf("get_health 应成功: %v", err)
	}
	health, ok := result.(map[string]interface{})
	if !ok || health["gateway_running"] != true {
		t.Fatalf("get_health 应返回注入的数据，实际 %+v", result)
	}
	if ds.healthCalls != 1 {
		t.Fatalf("数据源应被调用一次，实际 %d", ds.healthCalls)
	}

	if _, err := server.CallTool("route_request", map[string]interface{}{"model": "gpt-4o"}); err != nil {
		t.Fatalf("route_request 应成功: %v", err)
	}

	if _, err := server.CallTool("switch_combo", map[string]interface{}{"combo_id": "balanced"}); err != nil {
		t.Fatalf("switch_combo 应成功: %v", err)
	}
	if ds.switched != "balanced" {
		t.Fatalf("switch_combo 应写入数据源，实际 %q", ds.switched)
	}
}

// 未注入数据源时必须显式报错，不能返回硬编码的假数据（CL-S9）。
func TestMCPWithoutDataSource(t *testing.T) {
	server := NewGatewayServer()

	for _, name := range []string{"get_health", "check_quota", "list_models_catalog"} {
		_, err := server.CallTool(name, map[string]interface{}{})
		if err == nil {
			t.Fatalf("%s 在未注入数据源时应返回错误", name)
		}
		if !strings.Contains(err.Error(), "data source not configured") {
			t.Fatalf("%s 错误信息应说明数据源未配置，实际: %v", name, err)
		}
	}

	// 参数校验先于数据源检查。
	if _, err := server.CallTool("switch_combo", map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "combo_id is required") {
		t.Fatalf("switch_combo 缺少参数时应报错，实际: %v", err)
	}

	// web_search 依赖外部服务，必须显式拒绝。
	if _, err := server.CallTool("web_search", map[string]interface{}{"query": "x"}); err == nil || !strings.Contains(err.Error(), "external search provider") {
		t.Fatalf("web_search 应显式拒绝，实际: %v", err)
	}

	// 未知工具仍应返回 tool not found。
	_, err := server.CallTool("nonexistent_tool", map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "tool not found") {
		t.Fatalf("未知工具应返回 tool not found，实际: %v", err)
	}
}

func TestMCPAuditLog(t *testing.T) {
	server := NewGatewayServer()

	// 已注册但未注入数据源的工具
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
