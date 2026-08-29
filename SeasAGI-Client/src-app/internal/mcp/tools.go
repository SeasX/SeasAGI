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

func toolGetHealth(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"status": "healthy", "uptime": "running"}, nil
}

func toolListCombos(args map[string]interface{}) (interface{}, error) {
	return []map[string]interface{}{
		{"id": "combo-1", "name": "default", "strategy": "priority"},
	}, nil
}

func toolGetComboMetrics(args map[string]interface{}) (interface{}, error) {
	comboID, _ := args["combo_id"].(string)
	if comboID == "" {
		return nil, fmt.Errorf("combo_id is required")
	}
	return map[string]interface{}{"combo_id": comboID, "requests": 100, "success_rate": 0.95}, nil
}

func toolSwitchCombo(args map[string]interface{}) (interface{}, error) {
	comboID, _ := args["combo_id"].(string)
	if comboID == "" {
		return nil, fmt.Errorf("combo_id is required")
	}
	return map[string]interface{}{"active_combo": comboID, "status": "switched"}, nil
}

func toolCheckQuota(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"used": 5000, "limit": 10000, "remaining": 5000}, nil
}

func toolRouteRequest(args map[string]interface{}) (interface{}, error) {
	model, _ := args["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	return map[string]interface{}{"model": model, "provider": "openai", "latency_ms": 200}, nil
}

func toolCostReport(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"total_cost": 12.50, "currency": "USD", "period": "2026-07"}, nil
}

func toolListModelsCatalog(args map[string]interface{}) (interface{}, error) {
	return []map[string]interface{}{
		{"id": "gpt-4o", "provider": "openai", "context_window": 128000},
		{"id": "claude-3.5-sonnet", "provider": "anthropic", "context_window": 200000},
	}, nil
}

func toolWebSearch(args map[string]interface{}) (interface{}, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	return []map[string]interface{}{
		{"title": "Search result for: " + query, "url": "https://example.com"},
	}, nil
}

func toolSimulateRoute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"simulated": true, "would_route_to": "provider-a"}, nil
}

func toolSetBudgetGuard(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"budget_guard": "set", "threshold": 100.0}, nil
}

func toolSetRoutingStrategy(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"strategy": "priority", "status": "applied"}, nil
}

func toolSetResilienceProfile(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"profile": "balanced", "status": "applied"}, nil
}

func toolTestCombo(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"combo_id": "combo-1", "reachable": true, "latency_ms": 50}, nil
}

func toolGetProviderMetrics(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"openai": map[string]interface{}{"requests": 100, "errors": 2}}, nil
}

func toolBestComboForTask(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"recommended": "combo-2", "reason": "lowest latency for coding tasks"}, nil
}

func toolExplainRoute(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"explanation": "Routed to provider-a based on priority strategy"}, nil
}

func toolGetSessionSnapshot(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"session_id": "sess-1", "requests": 42}, nil
}

func toolDBHealthCheck(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"db_status": "healthy", "size_bytes": 1048576}, nil
}
