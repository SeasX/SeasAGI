package main

import (
	"encoding/json"
	"fmt"

	"github.com/SeasAGI/SeasAGI-Client/internal/mcp"
)

// BindMCPDataSource 把桌面端已有的运行时数据与配置方法接入 MCP Gateway Server，
// 让 MCP 工具返回真实数据而不是硬编码占位值。由 main.go 在构造 App 后调用。
func (a *App) BindMCPDataSource() {
	a.mcpServer.SetDataSource(&appMCPDataSource{app: a})
}

// appMCPDataSource 是 mcp.DataSource 的桌面端实现，全部委托给 App 上已有的方法。
type appMCPDataSource struct {
	app *App
}

func toMapSlice(v interface{}) []map[string]interface{} {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func intArg(args map[string]interface{}, key string) (int, bool) {
	switch v := args[key].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n), true
		}
	}
	return 0, false
}

func (d *appMCPDataSource) Health() (map[string]interface{}, error) {
	return d.app.GetRuntimeStatus(), nil
}

func (d *appMCPDataSource) ListCombos() ([]map[string]interface{}, error) {
	return toMapSlice(d.app.ListModelCombos()), nil
}

func (d *appMCPDataSource) ComboMetrics(comboID string) (map[string]interface{}, error) {
	metrics := d.app.GetComboRouteMetrics()
	if comboID == "" {
		return map[string]interface{}{"metrics": metrics}, nil
	}
	filtered := make([]map[string]interface{}, 0, 1)
	for _, m := range metrics {
		if name, _ := m["combo_name"].(string); name == comboID {
			filtered = append(filtered, m)
		}
	}
	return map[string]interface{}{"combo_id": comboID, "metrics": filtered}, nil
}

func (d *appMCPDataSource) SwitchCombo(comboID string) error {
	return d.app.SetDefaultComboName(comboID)
}

func (d *appMCPDataSource) QuotaStatus() (map[string]interface{}, error) {
	usage := d.app.GetUsageSummary()
	rateLimit := d.app.GetRateLimitConfig()
	return map[string]interface{}{
		"usage":                  usage,
		"monthly_cost_limit_usd": rateLimit["monthly_cost_limit_usd"],
		"month_cost_usd":         usage["month_cost_usd"],
		"rate_limit_enabled":     rateLimit["enabled"],
		"rate_limit_default_rpm": rateLimit["default_rpm"],
	}, nil
}

func (d *appMCPDataSource) RoutePreview(model string) (map[string]interface{}, error) {
	return d.app.SimulateIntentRouting("", model), nil
}

func (d *appMCPDataSource) CostReport() (map[string]interface{}, error) {
	return map[string]interface{}{
		"summary": d.app.GetUsageSummary(),
		"daily":   toMapSlice(d.app.GetDailyUsage(30)),
	}, nil
}

func (d *appMCPDataSource) ModelsCatalog() ([]map[string]interface{}, error) {
	return d.app.FetchModelCatalog(), nil
}

func (d *appMCPDataSource) ProviderMetrics() ([]map[string]interface{}, error) {
	return d.app.GetProviderHealthSummary(), nil
}

func (d *appMCPDataSource) BestComboForTask(task string) (map[string]interface{}, error) {
	return d.app.PreviewComboOptimization("value_first", task), nil
}

func (d *appMCPDataSource) ExplainRoute(model string) (map[string]interface{}, error) {
	return d.app.SimulateIntentRouting("", model), nil
}

func (d *appMCPDataSource) SessionSnapshot() (map[string]interface{}, error) {
	return map[string]interface{}{
		"app":     d.app.GetAppInfo(),
		"runtime": d.app.GetRuntimeStatus(),
		"usage":   d.app.GetUsageSummary(),
	}, nil
}

func (d *appMCPDataSource) DBHealthCheck() (map[string]interface{}, error) {
	return d.app.RunDiagnostics(), nil
}

func (d *appMCPDataSource) SetBudgetGuard(args map[string]interface{}) (map[string]interface{}, error) {
	limit, ok := args["monthly_cost_limit_usd"].(float64)
	if !ok {
		return nil, fmt.Errorf("monthly_cost_limit_usd is required")
	}
	cfg := d.app.GetRateLimitConfig()
	cfg["monthly_cost_limit_usd"] = limit
	if err := d.app.SetRateLimitConfig(cfg); err != nil {
		return nil, err
	}
	return map[string]interface{}{"monthly_cost_limit_usd": limit}, nil
}

func (d *appMCPDataSource) SetRoutingStrategy(args map[string]interface{}) (map[string]interface{}, error) {
	strategy, _ := args["strategy"].(string)
	if strategy == "" {
		return nil, fmt.Errorf("strategy is required")
	}
	stickyUses, _ := intArg(args, "sticky_uses")
	if err := d.app.UpdateRoutingSettings(strategy, stickyUses); err != nil {
		return nil, err
	}
	return map[string]interface{}{"strategy": strategy, "sticky_uses": stickyUses}, nil
}

func (d *appMCPDataSource) SetResilienceProfile(args map[string]interface{}) (map[string]interface{}, error) {
	cfg := d.app.GetOptimizationConfig()
	for _, key := range []string{"penalty_decay_sec", "health_check_sec", "health_max_failures", "cooldown_sec", "sticky_ttl_sec"} {
		if v, ok := intArg(args, key); ok {
			cfg[key] = v
		}
	}
	for _, key := range []string{"cooldown_enabled", "sticky_enabled", "penalty_enabled", "health_check_enabled"} {
		if v, ok := args[key].(bool); ok {
			cfg[key] = v
		}
	}
	if mode, ok := args["mode"].(string); ok && mode != "" {
		cfg["mode"] = mode
	}
	if err := d.app.SetOptimizationConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (d *appMCPDataSource) TestCombo(comboID string) (map[string]interface{}, error) {
	combo := d.app.GetComboByName(comboID)
	if combo.Name == "" {
		return nil, fmt.Errorf("combo not found: %s", comboID)
	}
	result := map[string]interface{}{
		"combo_id":   comboID,
		"combo_name": combo.Name,
		"steps":      len(combo.Steps),
	}
	if len(combo.Steps) > 0 && combo.Steps[0].ChannelID != "" {
		probe, err := d.app.TestCustomChannel(combo.Steps[0].ChannelID)
		result["channel_id"] = combo.Steps[0].ChannelID
		result["probe"] = probe
		if err != nil {
			result["probe_error"] = err.Error()
		}
	}
	return result, nil
}

var _ mcp.DataSource = (*appMCPDataSource)(nil)
