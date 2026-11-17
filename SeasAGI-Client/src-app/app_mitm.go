package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/mitm"
	"github.com/SeasAGI/SeasAGI-Client/internal/network"
)

// TestMITMDomain 测试指定域名通过 MITM 代理后的连通性。
// 返回状态码、延迟(ms)、是否被拦截、错误信息。
func (a *App) TestMITMDomain(domain string) map[string]any {
	result := map[string]any{
		"domain":      domain,
		"reachable":   false,
		"intercepted": false,
		"status_code": 0,
		"latency_ms":  0,
		"error":       "",
	}

	if a.mitmMgr == nil {
		result["error"] = "MITM manager not initialized"
		return result
	}

	status := a.mitmMgr.GetStatus()
	if status.State != mitm.StateRunning {
		result["error"] = "MITM proxy not running"
		return result
	}

	// 检查域名是否在拦截规则中
	intercepted := a.mitmMgr.GetRules()
	isIntercepted := false
	for _, d := range intercepted {
		if d == domain {
			isIntercepted = true
			break
		}
	}
	result["intercepted"] = isIntercepted

	// 通过本地代理发起 HTTPS 请求测试连通性
	// 使用 MITM CA 证书池验证 TLS，避免 "tls: failed to verify certificate" 错误
	proxyURL, _ := neturl.Parse(fmt.Sprintf("http://127.0.0.1:%d", status.ProxyPort))

	tlsConfig := &tls.Config{}
	if caPool := a.mitmMgr.CertPool(); caPool != nil {
		tlsConfig.RootCAs = caPool
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: tlsConfig,
		},
	}

	targetURL := fmt.Sprintf("https://%s/", domain)
	start := time.Now()
	resp, err := client.Get(targetURL)
	latency := time.Since(start).Milliseconds()
	result["latency_ms"] = latency

	if err != nil {
		result["error"] = err.Error()
		return result
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	result["reachable"] = true
	result["status_code"] = resp.StatusCode
	return result
}

// GetMITMEnvHint 返回当前 Shell 环境下的代理环境变量设置/取消命令。
func (a *App) GetMITMEnvHint() map[string]string {
	if a.mitmMgr == nil {
		return nil
	}
	port := a.mitmMgr.GetStatus().ProxyPort
	if port == 0 {
		port = 8080
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", port)
	hint := mitm.DetectShellEnv(proxyAddr)
	return map[string]string{
		"shell":       hint.Shell,
		"export_cmds": hint.ExportCmds,
		"unset_cmds":  hint.UnsetCmds,
	}
}

// === UI-Batch 5: MITM Target + Notion/Obsidian ===

// GetMITMTargets 返回所有 MITM 目标预设（含动态拉取的企业端目标）。
func (a *App) GetMITMTargets() []map[string]any {
	targets := mitm.MergedTargets()
	result := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		hosts := make([]string, 0, len(t.Hosts))
		hosts = append(hosts, t.Hosts...)
		models := make([]map[string]any, 0, len(t.DefaultModels))
		for _, m := range t.DefaultModels {
			models = append(models, map[string]any{"id": m.ID, "name": m.Name})
		}
		endpoints := make([]string, 0, len(t.EndpointPatterns))
		endpoints = append(endpoints, t.EndpointPatterns...)
		result = append(result, map[string]any{
			"id":                t.ID,
			"name":              t.Name,
			"icon":              t.Icon,
			"color":             t.Color,
			"hosts":             hosts,
			"port":              t.Port,
			"endpoint_patterns": endpoints,
			"default_models":    models,
			"viability":         t.Viability,
		})
	}
	return result
}

// FetchMITMTargetsFromEnterprise 从企业服务端动态拉取 MITM 目标列表。
// 拉取成功后与本地预设合并（企业端优先），后续 GetMITMTargets 调用即返回合并结果。
func (a *App) FetchMITMTargetsFromEnterprise() map[string]any {
	token := ""
	if a.authSvc.IsLoggedIn() {
		token = a.authSvc.GetPlatformToken()
	}
	if err := mitm.FetchTargetsFromEnterprise(a.ctx, token); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

// GetMITMRoutingConfig 返回 MITM 接管请求的路由映射配置。
func (a *App) GetMITMRoutingConfig() map[string]any {
	cfg := a.configSvc.GetMITMRoutingConfig()
	rules := make([]map[string]any, 0, len(cfg.Rules))
	for _, r := range cfg.Rules {
		rules = append(rules, map[string]any{
			"id":            r.ID,
			"enabled":       r.Enabled,
			"priority":      r.Priority,
			"target_id":     r.TargetID,
			"endpoint_path": r.EndpointPath,
			"request_model": r.RequestModel,
			"action":        r.Action,
			"combo_name":    r.ComboName,
			"model":         r.Model,
			"constraints":   r.Constraints,
			"source":        r.Source,
		})
	}
	return map[string]any{
		"enabled":            cfg.Enabled,
		"default_action":     cfg.DefaultAction,
		"default_combo_name": cfg.DefaultCombo,
		"default_model":      cfg.DefaultModel,
		"security_mode":      cfg.SecurityMode,
		"rules":              rules,
	}
}

// GetMITMEnterpriseRules 返回企业端下发的 MITM 路由规则（只读、置底）。
func (a *App) GetMITMEnterpriseRules() []map[string]any {
	entRules := mitm.EnterpriseRoutingRules()
	rules := make([]map[string]any, 0, len(entRules))
	for _, r := range entRules {
		rules = append(rules, map[string]any{
			"id":            r.ID,
			"enabled":       r.Enabled,
			"priority":      r.Priority,
			"target_id":     r.TargetID,
			"endpoint_path": r.EndpointPath,
			"request_model": r.RequestModel,
			"action":        r.Action,
			"combo_name":    r.ComboName,
			"model":         r.Model,
			"constraints":   r.Constraints,
			"source":        r.Source,
		})
	}
	return rules
}

// FetchMITMRoutingFromEnterprise 从企业服务端拉取默认 MITM 路由规则（与 FetchMITMTargetsFromEnterprise 同风格）。
func (a *App) FetchMITMRoutingFromEnterprise() map[string]any {
	token := ""
	if a.authSvc.IsLoggedIn() {
		token = a.authSvc.GetPlatformToken()
	}
	if err := mitm.FetchRoutingRulesFromEnterprise(a.ctx, token); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "count": len(mitm.EnterpriseRoutingRules())}
}

// SetMITMRoutingConfig 设置 MITM 接管请求的路由映射配置（校验失败返回错误，不落盘）。
func (a *App) SetMITMRoutingConfig(cfg map[string]any) error {
	parsed := config.MITMRoutingConfig{}
	if v, ok := cfg["enabled"]; ok {
		parsed.Enabled, _ = v.(bool)
	}
	if v, ok := cfg["default_action"].(string); ok {
		parsed.DefaultAction = v
	}
	if v, ok := cfg["default_combo_name"].(string); ok {
		parsed.DefaultCombo = v
	}
	if v, ok := cfg["default_model"].(string); ok {
		parsed.DefaultModel = v
	}
	if v, ok := cfg["security_mode"].(string); ok {
		parsed.SecurityMode = v
	}
	if raw, ok := cfg["rules"].([]any); ok {
		parsed.Rules = make([]config.MITMRoutingRule, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			rule := config.MITMRoutingRule{}
			if v, ok := m["id"].(string); ok {
				rule.ID = v
			}
			if v, ok := m["enabled"].(bool); ok {
				rule.Enabled = v
			}
			if v, ok := m["priority"]; ok {
				rule.Priority, _ = toInt(v)
			}
			if v, ok := m["target_id"].(string); ok {
				rule.TargetID = v
			}
			if v, ok := m["endpoint_path"].(string); ok {
				rule.EndpointPath = v
			}
			if v, ok := m["request_model"].(string); ok {
				rule.RequestModel = v
			}
			if v, ok := m["action"].(string); ok {
				rule.Action = v
			}
			if v, ok := m["combo_name"].(string); ok {
				rule.ComboName = v
			}
			if v, ok := m["model"].(string); ok {
				rule.Model = v
			}
			if v, ok := m["constraints"].(map[string]any); ok {
				rule.Constraints = v
			}
			parsed.Rules = append(parsed.Rules, rule)
		}
	}
	return a.configSvc.SetMITMRoutingConfig(parsed)
}

// SetMITMManager 注入 MITM Manager 实例。
func (a *App) SetMITMManager(mgr *mitm.Manager) {
	a.mitmMgr = mgr
}

// StartMITM 启动 MITM 透明代理。
func (a *App) StartMITM() error {
	if a.mitmMgr == nil {
		return fmt.Errorf("MITM manager not initialized")
	}
	return a.mitmMgr.Start(a.ctx)
}

// StopMITM 停止 MITM 透明代理。
func (a *App) StopMITM() error {
	if a.mitmMgr == nil {
		return nil
	}
	return a.mitmMgr.Stop()
}

// IsMITMRunning 检查 MITM 是否正在运行。
func (a *App) IsMITMRunning() bool {
	if a.mitmMgr == nil {
		return false
	}
	return a.mitmMgr.GetStatus().State == mitm.StateRunning
}

// GetMITMStatus 返回 MITM 运行状态。
func (a *App) GetMITMStatus() map[string]any {
	if a.mitmMgr == nil {
		return map[string]any{
			"state":                 "stopped",
			"error":                 "not initialized",
			"system_proxy":          false,
			"system_proxy_active":   false,
			"residual_system_proxy": false,
		}
	}
	s := a.mitmMgr.GetStatus()
	systemProxyActive := s.SystemProxy
	if setter := network.NewSystemProxySetter(); setter != nil {
		if active, err := setter.IsActive(); err == nil {
			systemProxyActive = active
		}
	}
	return map[string]any{
		"state":                 string(s.State),
		"proxy_port":            s.ProxyPort,
		"ca_installed":          s.CAInstalled,
		"ca_trusted":            s.CATrusted,
		"rules_count":           s.RulesCount,
		"system_proxy":          s.SystemProxy,
		"system_proxy_owned":    s.SystemProxyOwned,
		"system_proxy_active":   systemProxyActive,
		"residual_system_proxy": systemProxyActive && s.State != mitm.StateRunning,
		"last_error":            s.LastError,
	}
}

// GetMITMRules 返回当前拦截域名列表。
func (a *App) GetMITMRules() []string {
	if a.mitmMgr == nil {
		return []string{}
	}
	return a.mitmMgr.GetRules()
}

// AddMITMRule 动态新增拦截域名。
func (a *App) AddMITMRule(domain string) error {
	if a.mitmMgr == nil {
		return fmt.Errorf("MITM manager not initialized")
	}
	return a.mitmMgr.AddRule(domain)
}

// RemoveMITMRule 动态移除拦截域名。
func (a *App) RemoveMITMRule(domain string) error {
	if a.mitmMgr == nil {
		return fmt.Errorf("MITM manager not initialized")
	}
	return a.mitmMgr.RemoveRule(domain)
}

// GetMITMRecentIntercepts 返回最近 n 条拦截日志。
func (a *App) GetMITMRecentIntercepts(n int) []map[string]any {
	if a.mitmMgr == nil {
		return nil
	}
	entries := a.mitmMgr.GetRecentIntercepts(n)
	result := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		result = append(result, map[string]any{
			"time":        e.Time,
			"method":      e.Method,
			"host":        e.Host,
			"path":        e.Path,
			"status":      e.Status,
			"intercepted": e.Intercepted,
			"duration_ms": e.DurationMs,
		})
	}
	return result
}
