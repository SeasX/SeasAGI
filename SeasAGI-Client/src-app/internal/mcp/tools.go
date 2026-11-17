package mcp

import "fmt"

// MCPTool MCP tool 定义。
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Handler     func(args map[string]interface{}) (interface{}, error)
}

// DataSource 为 MCP 工具提供本机真实数据与配置写入能力。
//
// 实现由桌面端 App 注入（见 src-app/app_mcp.go）。未注入时，依赖数据的工具
// 返回明确错误，避免外部 agent 基于硬编码的假数据做决策。
type DataSource interface {
	Health() (map[string]interface{}, error)
	ListCombos() ([]map[string]interface{}, error)
	ComboMetrics(comboID string) (map[string]interface{}, error)
	SwitchCombo(comboID string) error
	QuotaStatus() (map[string]interface{}, error)
	RoutePreview(model string) (map[string]interface{}, error)
	CostReport() (map[string]interface{}, error)
	ModelsCatalog() ([]map[string]interface{}, error)
	ProviderMetrics() ([]map[string]interface{}, error)
	BestComboForTask(task string) (map[string]interface{}, error)
	ExplainRoute(model string) (map[string]interface{}, error)
	SessionSnapshot() (map[string]interface{}, error)
	DBHealthCheck() (map[string]interface{}, error)
	SetBudgetGuard(args map[string]interface{}) (map[string]interface{}, error)
	SetRoutingStrategy(args map[string]interface{}) (map[string]interface{}, error)
	SetResilienceProfile(args map[string]interface{}) (map[string]interface{}, error)
	TestCombo(comboID string) (map[string]interface{}, error)
}

// registerBuiltinTools 注册核心 19 个 MCP tool。
func (s *GatewayServer) registerBuiltinTools() {
	tools := []MCPTool{
		{
			Name:        "get_health",
			Description: "获取系统健康状态",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolGetHealth,
		},
		{
			Name:        "list_combos",
			Description: "列出所有 Combo 路由配置",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolListCombos,
		},
		{
			Name:        "get_combo_metrics",
			Description: "获取 Combo 性能指标",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"combo_id": map[string]interface{}{"type": "string"}}},
			Handler:     s.toolGetComboMetrics,
		},
		{
			Name:        "switch_combo",
			Description: "切换当前活跃 Combo",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"combo_id": map[string]interface{}{"type": "string"}}, "required": []string{"combo_id"}},
			Handler:     s.toolSwitchCombo,
		},
		{
			Name:        "check_quota",
			Description: "检查 API key 配额使用",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolCheckQuota,
		},
		{
			Name:        "route_request",
			Description: "模拟路由决策",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"model": map[string]interface{}{"type": "string"}}, "required": []string{"model"}},
			Handler:     s.toolRouteRequest,
		},
		{
			Name:        "cost_report",
			Description: "生成成本报告",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolCostReport,
		},
		{
			Name:        "list_models_catalog",
			Description: "列出可用模型目录",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolListModelsCatalog,
		},
		{
			Name:        "web_search",
			Description: "执行网络搜索（需要外部搜索服务凭据）",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}}, "required": []string{"query"}},
			Handler:     s.toolWebSearch,
		},
		{
			Name:        "simulate_route",
			Description: "模拟路由请求（不实际调用）",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"model": map[string]interface{}{"type": "string"}}},
			Handler:     s.toolSimulateRoute,
		},
		{
			Name:        "set_budget_guard",
			Description: "设置预算阈值告警",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"monthly_cost_limit_usd": map[string]interface{}{"type": "number"}}},
			Handler:     s.toolSetBudgetGuard,
		},
		{
			Name:        "set_routing_strategy",
			Description: "设置路由策略",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"strategy": map[string]interface{}{"type": "string"}, "sticky_uses": map[string]interface{}{"type": "number"}}},
			Handler:     s.toolSetRoutingStrategy,
		},
		{
			Name:        "set_resilience_profile",
			Description: "设置弹性策略配置",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolSetResilienceProfile,
		},
		{
			Name:        "test_combo",
			Description: "测试 Combo 连通性",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"combo_id": map[string]interface{}{"type": "string"}}, "required": []string{"combo_id"}},
			Handler:     s.toolTestCombo,
		},
		{
			Name:        "get_provider_metrics",
			Description: "获取 Provider 性能指标",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolGetProviderMetrics,
		},
		{
			Name:        "best_combo_for_task",
			Description: "推荐最适合任务的 Combo",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"task": map[string]interface{}{"type": "string"}}},
			Handler:     s.toolBestComboForTask,
		},
		{
			Name:        "explain_route",
			Description: "解释路由决策原因",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"model": map[string]interface{}{"type": "string"}}},
			Handler:     s.toolExplainRoute,
		},
		{
			Name:        "get_session_snapshot",
			Description: "获取当前会话快照",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolGetSessionSnapshot,
		},
		{
			Name:        "db_health_check",
			Description: "数据库健康检查",
			InputSchema: map[string]interface{}{"type": "object"},
			Handler:     s.toolDBHealthCheck,
		},
	}

	for _, tool := range tools {
		s.RegisterTool(tool)
	}
}

// --- Tool handler 实现 ---

func (s *GatewayServer) requireDataSource() (DataSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.dataSource == nil {
		return nil, fmt.Errorf("mcp data source not configured: start the desktop app so local routing/usage data is available")
	}
	return s.dataSource, nil
}

func stringArg(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func (s *GatewayServer) toolGetHealth(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.Health()
}

func (s *GatewayServer) toolListCombos(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.ListCombos()
}

func (s *GatewayServer) toolGetComboMetrics(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.ComboMetrics(stringArg(args, "combo_id"))
}

func (s *GatewayServer) toolSwitchCombo(args map[string]interface{}) (interface{}, error) {
	comboID := stringArg(args, "combo_id")
	if comboID == "" {
		return nil, fmt.Errorf("combo_id is required")
	}
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	if err := ds.SwitchCombo(comboID); err != nil {
		return nil, err
	}
	return map[string]interface{}{"status": "ok", "active_combo": comboID}, nil
}

func (s *GatewayServer) toolCheckQuota(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.QuotaStatus()
}

func (s *GatewayServer) toolRouteRequest(args map[string]interface{}) (interface{}, error) {
	model := stringArg(args, "model")
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.RoutePreview(model)
}

func (s *GatewayServer) toolCostReport(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.CostReport()
}

func (s *GatewayServer) toolListModelsCatalog(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.ModelsCatalog()
}

// toolWebSearch 依赖外部搜索服务，桌面端未内置搜索凭据，因此显式拒绝。
func (s *GatewayServer) toolWebSearch(args map[string]interface{}) (interface{}, error) {
	if stringArg(args, "query") == "" {
		return nil, fmt.Errorf("query is required")
	}
	return nil, fmt.Errorf("web_search requires an external search provider credential; configure it in the desktop app instead")
}

func (s *GatewayServer) toolSimulateRoute(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.RoutePreview(stringArg(args, "model"))
}

func (s *GatewayServer) toolSetBudgetGuard(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.SetBudgetGuard(args)
}

func (s *GatewayServer) toolSetRoutingStrategy(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.SetRoutingStrategy(args)
}

func (s *GatewayServer) toolSetResilienceProfile(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.SetResilienceProfile(args)
}

func (s *GatewayServer) toolTestCombo(args map[string]interface{}) (interface{}, error) {
	comboID := stringArg(args, "combo_id")
	if comboID == "" {
		return nil, fmt.Errorf("combo_id is required")
	}
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.TestCombo(comboID)
}

func (s *GatewayServer) toolGetProviderMetrics(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.ProviderMetrics()
}

func (s *GatewayServer) toolBestComboForTask(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.BestComboForTask(stringArg(args, "task"))
}

func (s *GatewayServer) toolExplainRoute(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.ExplainRoute(stringArg(args, "model"))
}

func (s *GatewayServer) toolGetSessionSnapshot(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.SessionSnapshot()
}

func (s *GatewayServer) toolDBHealthCheck(args map[string]interface{}) (interface{}, error) {
	ds, err := s.requireDataSource()
	if err != nil {
		return nil, err
	}
	return ds.DBHealthCheck()
}
