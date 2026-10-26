package mcp

import "fmt"

// MCPTool MCP tool 定义。
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Handler     func(args map[string]interface{}) (interface{}, error)
}

// registerBuiltinTools 注册核心 20 个 MCP tool。
func (s *GatewayServer) registerBuiltinTools() {
	tools := []MCPTool{
		{
			Name:        "get_health",
			Description: "获取系统健康状态",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolGetHealth,
		},
		{
			Name:        "list_combos",
			Description: "列出所有 Combo 路由配置",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolListCombos,
		},
		{
			Name:        "get_combo_metrics",
			Description: "获取 Combo 性能指标",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"combo_id": map[string]interface{}{"type": "string"}}},
			Handler:     toolGetComboMetrics,
		},
		{
			Name:        "switch_combo",
			Description: "切换当前活跃 Combo",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"combo_id": map[string]interface{}{"type": "string"}}, "required": []string{"combo_id"}},
			Handler:     toolSwitchCombo,
		},
		{
			Name:        "check_quota",
			Description: "检查 API key 配额使用",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolCheckQuota,
		},
		{
			Name:        "route_request",
			Description: "模拟路由决策",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"model": map[string]interface{}{"type": "string"}}, "required": []string{"model"}},
			Handler:     toolRouteRequest,
		},
		{
			Name:        "cost_report",
			Description: "生成成本报告",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolCostReport,
		},
		{
			Name:        "list_models_catalog",
			Description: "列出可用模型目录",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolListModelsCatalog,
		},
		{
			Name:        "web_search",
			Description: "执行网络搜索",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}}, "required": []string{"query"}},
			Handler:     toolWebSearch,
		},
		{
			Name:        "simulate_route",
			Description: "模拟路由请求（不实际调用）",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolSimulateRoute,
		},
		{
			Name:        "set_budget_guard",
			Description: "设置预算阈值告警",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolSetBudgetGuard,
		},
		{
			Name:        "set_routing_strategy",
			Description: "设置路由策略",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolSetRoutingStrategy,
		},
		{
			Name:        "set_resilience_profile",
			Description: "设置弹性策略配置",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolSetResilienceProfile,
		},
		{
			Name:        "test_combo",
			Description: "测试 Combo 连通性",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolTestCombo,
		},
		{
			Name:        "get_provider_metrics",
			Description: "获取 Provider 性能指标",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolGetProviderMetrics,
		},
		{
			Name:        "best_combo_for_task",
			Description: "推荐最适合任务的 Combo",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolBestComboForTask,
		},
		{
			Name:        "explain_route",
			Description: "解释路由决策原因",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolExplainRoute,
		},
		{
			Name:        "get_session_snapshot",
			Description: "获取当前会话快照",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolGetSessionSnapshot,
		},
		{
			Name:        "db_health_check",
			Description: "数据库健康检查",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     toolDBHealthCheck,
		},
	}

	for _, tool := range tools {
		s.RegisterTool(tool)
	}
}

// --- Tool handler 实现 ---
//
// 这些工具目前尚未接入真实的路由/用量/健康数据源。返回显式 unimplemented 错误，
// 避免外部 agent 基于硬编码的假健康度、假配额做出错误决策（CL-S9）。

func unimplemented(tool string) (interface{}, error) {
	return nil, fmt.Errorf("%s is not implemented yet; query the desktop UI or local HTTP API instead", tool)
}

func toolGetHealth(args map[string]interface{}) (interface{}, error) {
	return unimplemented("get_health")
}

func toolListCombos(args map[string]interface{}) (interface{}, error) {
	return unimplemented("list_combos")
}

func toolGetComboMetrics(args map[string]interface{}) (interface{}, error) {
	return unimplemented("get_combo_metrics")
}

func toolSwitchCombo(args map[string]interface{}) (interface{}, error) {
	return unimplemented("switch_combo")
}

func toolCheckQuota(args map[string]interface{}) (interface{}, error) {
	return unimplemented("check_quota")
}

func toolRouteRequest(args map[string]interface{}) (interface{}, error) {
	return unimplemented("route_request")
}

func toolCostReport(args map[string]interface{}) (interface{}, error) {
	return unimplemented("cost_report")
}

func toolListModelsCatalog(args map[string]interface{}) (interface{}, error) {
	return unimplemented("list_models_catalog")
}

func toolWebSearch(args map[string]interface{}) (interface{}, error) {
	return unimplemented("web_search")
}

func toolSimulateRoute(args map[string]interface{}) (interface{}, error) {
	return unimplemented("simulate_route")
}

func toolSetBudgetGuard(args map[string]interface{}) (interface{}, error) {
	return unimplemented("set_budget_guard")
}

func toolSetRoutingStrategy(args map[string]interface{}) (interface{}, error) {
	return unimplemented("set_routing_strategy")
}

func toolSetResilienceProfile(args map[string]interface{}) (interface{}, error) {
	return unimplemented("set_resilience_profile")
}

func toolTestCombo(args map[string]interface{}) (interface{}, error) {
	return unimplemented("test_combo")
}

func toolGetProviderMetrics(args map[string]interface{}) (interface{}, error) {
	return unimplemented("get_provider_metrics")
}

func toolBestComboForTask(args map[string]interface{}) (interface{}, error) {
	return unimplemented("best_combo_for_task")
}

func toolExplainRoute(args map[string]interface{}) (interface{}, error) {
	return unimplemented("explain_route")
}

func toolGetSessionSnapshot(args map[string]interface{}) (interface{}, error) {
	return unimplemented("get_session_snapshot")
}

func toolDBHealthCheck(args map[string]interface{}) (interface{}, error) {
	return unimplemented("db_health_check")
}
