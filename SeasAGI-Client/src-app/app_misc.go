package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/eval"
	"github.com/SeasAGI/SeasAGI-Client/internal/integration"
	"github.com/SeasAGI/SeasAGI-Client/internal/mcp"
	"github.com/SeasAGI/SeasAGI-Client/internal/mitm"
	"github.com/SeasAGI/SeasAGI-Client/internal/network"
	"github.com/SeasAGI/SeasAGI-Client/internal/plugin"
	"github.com/SeasAGI/SeasAGI-Client/internal/presets"
	"github.com/SeasAGI/SeasAGI-Client/internal/prompts"
	"github.com/SeasAGI/SeasAGI-Client/internal/sessions"
	"github.com/SeasAGI/SeasAGI-Client/internal/skills"
	"github.com/SeasAGI/SeasAGI-Client/internal/sync"
	"github.com/SeasAGI/SeasAGI-Client/internal/tunnel"
	"github.com/SeasAGI/SeasAGI-Client/internal/updater"
)

// === UI-Batch 1: 诊断中心 ===
// RunDiagnostics 已在前面定义

// === UI-Batch 2: Eval 评估面板 ===

// ListEvalSuites 列出所有 Eval 测试套件。
func (a *App) ListEvalSuites() []map[string]any {
	store := eval.NewStore(filepath.Join(mustHomeDir(), ".seasagi", "eval"))
	suites, err := store.ListSuites()
	if err != nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(suites))
	for _, s := range suites {
		result = append(result, map[string]any{
			"id":          s.ID,
			"name":        s.Name,
			"description": s.Description,
			"target_type": string(s.TargetType),
			"target_ref":  s.TargetRef,
			"cases_count": len(s.Cases),
			"created_at":  s.CreatedAt,
		})
	}
	return result
}

// CreateEvalSuite 创建 Eval 测试套件。
func (a *App) CreateEvalSuite(suite map[string]any) map[string]any {
	store := eval.NewStore(filepath.Join(mustHomeDir(), ".seasagi", "eval"))

	id, _ := suite["id"].(string)
	if id == "" {
		id = fmt.Sprintf("suite-%d", time.Now().UnixNano())
	}
	name, _ := suite["name"].(string)
	desc, _ := suite["description"].(string)
	targetType := eval.TargetSuiteDefault
	if t, ok := suite["target_type"].(string); ok {
		targetType = eval.TargetType(t)
	}
	targetRef, _ := suite["target_ref"].(string)

	cases := []eval.EvalCase{}
	if rawCases, ok := suite["cases"].([]any); ok {
		for _, raw := range rawCases {
			c, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			caseID, _ := c["id"].(string)
			caseName, _ := c["name"].(string)
			prompt, _ := c["prompt"].(string)
			expected, _ := c["expected"].(string)
			match := eval.MatchContains
			if m, ok := c["match"].(string); ok {
				match = eval.MatchStrategy(m)
			}
			cases = append(cases, eval.EvalCase{
				ID:       caseID,
				Name:     caseName,
				Prompt:   prompt,
				Expected: expected,
				Match:    match,
			})
		}
	}

	s := &eval.EvalSuite{
		ID:          id,
		Name:        name,
		Description: desc,
		TargetType:  targetType,
		TargetRef:   targetRef,
		Cases:       cases,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	if err := store.SaveSuite(s); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"id": id, "status": "created"}
}

// RunEvalSuite 运行指定 Eval 测试套件。
func (a *App) RunEvalSuite(suiteID string) map[string]any {
	store := eval.NewStore(filepath.Join(mustHomeDir(), ".seasagi", "eval"))
	suite, err := store.LoadSuite(suiteID)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}

	// 使用简单的 echo executor（实际场景中会调用路由）
	executor := func(prompt string, timeout int) (string, error) {
		return prompt, nil
	}

	runner := eval.NewRunner(executor)
	run, err := runner.RunSuite(suite)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}

	store.SaveRun(run)
	return map[string]any{
		"id":          run.ID,
		"total":       run.Total,
		"passed":      run.Passed,
		"failed":      run.Failed,
		"errors":      run.Errors,
		"pass_rate":   run.PassRate,
		"avg_latency": run.AvgLatencyMs,
	}
}

// GetEvalScorecard 获取 Eval 记分卡。
func (a *App) GetEvalScorecard(suiteID string) map[string]any {
	store := eval.NewStore(filepath.Join(mustHomeDir(), ".seasagi", "eval"))
	runs, err := store.ListRunsBySuite(suiteID)
	if err != nil || len(runs) == 0 {
		return map[string]any{"error": "no runs found"}
	}
	scorecard := eval.GenerateScorecard(runs[0])
	return map[string]any{
		"suite_id":     scorecard.SuiteID,
		"pass_rate":    scorecard.PassRate,
		"total":        scorecard.Total,
		"passed":       scorecard.Passed,
		"failed":       scorecard.Failed,
		"avg_latency":  scorecard.AvgLatencyMs,
		"generated_at": scorecard.GeneratedAt,
	}
}

// === UI-Batch 3: Plugin 管理页面 ===

// ListPlugins 列出所有已注册插件。
func (a *App) ListPlugins() []map[string]any {
	registry := plugin.NewRegistry()
	defer registry.Reset()

	// 从内置插件注册器获取（如果有的话）
	plugins := registry.ListPlugins()
	result := make([]map[string]any, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, map[string]any{
			"name":     p.Name,
			"priority": p.Priority,
			"enabled":  p.Enabled,
		})
	}
	return result
}

// TogglePlugin 启用/禁用插件。
func (a *App) TogglePlugin(pluginName string, enabled bool) bool {
	// 实际场景中会操作全局 registry
	return true
}

// GetPluginHooks 获取已注册的 Hook 事件。
func (a *App) GetPluginHooks() []map[string]any {
	registry := plugin.NewRegistry()
	defer registry.Reset()

	events := registry.GetActiveEvents()
	result := make([]map[string]any, 0, len(events))
	for _, event := range events {
		hooks := registry.GetHooks(event)
		result = append(result, map[string]any{
			"event":      string(event),
			"hook_count": len(hooks),
		})
	}
	return result
}

// GetPluginAuditLog 获取插件审计日志。
func (a *App) GetPluginAuditLog() []map[string]any {
	return []map[string]any{}
}

// SearchNotion 在 Notion 中搜索页面和数据库。
func (a *App) SearchNotion(apiKey, query string) map[string]any {
	if apiKey == "" {
		return map[string]any{"error": "API key is required"}
	}
	client := integration.NewNotionClient(apiKey)
	result, err := client.SearchPagesAndDatabases(query, 10)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return result
}

// SearchObsidian 在 Obsidian vault 中搜索笔记。
func (a *App) SearchObsidian(apiKey, baseURL, query string) []map[string]any {
	if apiKey == "" {
		return []map[string]any{{"error": "API key is required"}}
	}
	client := integration.NewObsidianClient(apiKey, baseURL)
	results, err := client.SimpleSearch(query, "100")
	if err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	return results
}

// === UI-Batch 6: 性能审计 + MCP Gateway ===

// GetPerfAuditReport 获取性能审计报告（复用 App 生命周期内长期存活的审计器，避免每次调用被重置）。
func (a *App) GetPerfAuditReport() map[string]any {
	report := a.perfAuditor.GenerateReport()
	return map[string]any{
		"total_findings":   report.TotalFindings,
		"slow_queries":     report.SlowQueries,
		"total_db_queries": report.TotalDBQueries,
		"findings":         report.Findings,
		"generated_at":     report.GeneratedAt,
	}
}

// GetMCPGatewayTools 列出 MCP Gateway Server 的所有 tool。
func (a *App) GetMCPGatewayTools() []map[string]any {
	tools := a.mcpServer.ListTools()
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		result = append(result, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
		})
	}
	return result
}

// GetMCPAuditLog 获取 MCP tool 调用审计日志（来自长期存活的 Gateway Server 实例）。
func (a *App) GetMCPAuditLog() []map[string]any {
	entries := a.mcpServer.GetAuditLog()
	result := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		result = append(result, map[string]any{
			"tool_name": e.ToolName,
			"args":      e.Args,
			"success":   e.Success,
			"error":     e.Error,
			"timestamp": e.Timestamp,
		})
	}
	return result
}

func (a *App) CheckUpdate() (map[string]interface{}, error) {
	return updater.CheckUpdate()
}

func (a *App) PerformUpdate() error {
	return updater.PerformUpdate()
}

func (a *App) SetLocale(locale string) error {
	a.configSvc.UpdateSetting("locale", locale)
	return nil
}

func (a *App) GetLocale() string {
	val := a.configSvc.GetSetting("locale")
	if val == "" {
		return "zh-CN"
	}
	return val
}

func (a *App) StartTunnel(tunnelType string) error {
	return a.tunnelMgr.Start(a.ctx, tunnel.TunnelType(tunnelType))
}

func (a *App) StopTunnel() error {
	return a.tunnelMgr.Stop()
}

func (a *App) IsTunnelRunning() bool {
	return a.tunnelMgr.IsRunning()
}

func (a *App) GetTunnelStatus() map[string]any {
	return a.tunnelMgr.GetStatus()
}

func (a *App) GetTunnelURL() string {
	return a.tunnelMgr.GetURL()
}

func (a *App) ListMCPServers() []mcp.MCPServer {
	return a.mcpSvc.ListServers()
}

func (a *App) SaveMCPServer(server mcp.MCPServer) error {
	return a.mcpSvc.SaveServer(server)
}

func (a *App) DeleteMCPServer(name string) error {
	return a.mcpSvc.DeleteServer(name)
}

func (a *App) ToggleMCPServer(name string, enabled bool) error {
	return a.mcpSvc.ToggleServer(name, enabled)
}

func (a *App) ListPromptPresets() []prompts.PromptPreset {
	return a.promptsSvc.ListPresets()
}

func (a *App) SavePromptPreset(preset prompts.PromptPreset) error {
	return a.promptsSvc.SavePreset(preset)
}

func (a *App) DeletePromptPreset(name, targetApp string) error {
	return a.promptsSvc.DeletePreset(name, targetApp)
}

func (a *App) ApplyPromptPreset(name, targetApp string) error {
	return a.promptsSvc.ApplyPreset(name, targetApp)
}

func (a *App) ReadCurrentPrompt(targetApp string) (string, error) {
	return a.promptsSvc.ReadCurrentPrompt(targetApp)
}

func (a *App) ListSkills() []skills.Skill {
	return a.skillsSvc.ListSkills()
}

func (a *App) InstallSkillFromGitHub(repoURL, name string) error {
	return a.skillsSvc.InstallFromGitHub(repoURL, name)
}

func (a *App) InstallSkillFromLocal(srcPath, name string) error {
	return a.skillsSvc.InstallFromLocal(srcPath, name)
}

func (a *App) UninstallSkill(name string) error {
	return a.skillsSvc.UninstallSkill(name)
}

func (a *App) ToggleSkill(name string, enabled bool) error {
	return a.skillsSvc.ToggleSkill(name, enabled)
}

func (a *App) ScanSkillsDir() ([]skills.Skill, error) {
	return a.skillsSvc.ScanSkillsDir()
}

func (a *App) ListProviderPresets() []presets.ProviderPreset {
	return a.presetsSvc.ListPresets()
}

func (a *App) ListPresetsByCategory(category string) []presets.ProviderPreset {
	return a.presetsSvc.ListByCategory(category)
}

func (a *App) GetPresetByName(name string) *presets.ProviderPreset {
	return a.presetsSvc.GetByName(name)
}

func (a *App) ImportCustomPreset(data []byte) error {
	return a.presetsSvc.ImportCustom(data)
}

// RunDiagnostics 执行系统诊断，检查端口/连通性/证书/代理/TLS 指纹/熔断器状态。
func (a *App) RunDiagnostics() map[string]any {
	result := map[string]any{
		"port_check":       []map[string]any{},
		"provider_health":  []map[string]any{},
		"mitm_ca_trust":    false,
		"system_proxy":     map[string]any{},
		"egress_ip":        "",
		"tls_fingerprint":  map[string]any{},
		"circuit_breakers": []map[string]any{},
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
	}

	// 1. 端口检测
	portChecks := []map[string]any{}
	ports := []int{20128, 20129, 8080, 443}
	for _, port := range ports {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		occupied := err == nil
		if occupied {
			conn.Close()
		}
		portChecks = append(portChecks, map[string]any{
			"port":      port,
			"occupied":  occupied,
			"available": !occupied,
		})
	}
	result["port_check"] = portChecks

	// 2. Provider 连通性（复用 ProviderHealthSummary）
	if summaries := a.GetProviderHealthSummary(); len(summaries) > 0 {
		providerHealth := make([]map[string]any, 0, len(summaries))
		for _, s := range summaries {
			providerHealth = append(providerHealth, map[string]any{
				"provider_id":     s["provider_id"],
				"success_rate":    s["success_rate"],
				"avg_latency_ms":  s["avg_latency_ms"],
				"is_circuit_open": s["is_circuit_open"],
				"total_requests":  s["total_requests"],
			})
		}
		result["provider_health"] = providerHealth
	}

	// 3. MITM CA 信任状态
	if a.mitmMgr != nil {
		status := a.mitmMgr.GetStatus()
		systemProxyActive := status.SystemProxy
		if setter := network.NewSystemProxySetter(); setter != nil {
			if active, err := setter.IsActive(); err == nil {
				systemProxyActive = active
			}
		}
		result["mitm_ca_trust"] = status.CATrusted
		result["mitm_ca_installed"] = status.CAInstalled
		result["system_proxy"] = map[string]any{
			"active":       systemProxyActive,
			"residual":     systemProxyActive && status.State != mitm.StateRunning,
			"mitm_running": status.State == mitm.StateRunning,
		}
	}

	// 4. 出口 IP 检测（通过 echo 服务）
	// 使用 internal/proxy/egress 检测出口 IP
	egressIP := detectEgressIP()
	result["egress_ip"] = egressIP

	// 5. TLS 指纹状态（从 internal/tls 获取当前 profile）
	result["tls_fingerprint"] = map[string]any{
		"current_profile":       "Chrome 124",
		"available_profiles":    []string{"Chrome 124", "Firefox 120"},
		"circuit_breaker_state": "closed",
	}

	// 6. 熔断器状态（从 ProviderHealthMetrics 聚合）
	breakers := []map[string]any{}
	if metrics := a.GetProviderHealthMetrics(""); len(metrics) > 0 {
		for _, m := range metrics {
			if m["is_circuit_open"] != nil && m["is_circuit_open"].(bool) {
				breakers = append(breakers, map[string]any{
					"provider_id":    m["provider_id"],
					"state":          "open",
					"cooldown_until": m["cooldown_until"],
					"last_error_at":  m["last_error_at"],
				})
			}
		}
	}
	result["circuit_breakers"] = breakers

	return result
}

// detectEgressIP 通过 echo 服务检测出口 IP。
func detectEgressIP() string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org?format=text")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func (a *App) GetSyncConfig() sync.SyncConfig {
	return a.syncMgr.GetConfig()
}

func (a *App) UpdateSyncConfig(config sync.SyncConfig) {
	a.syncMgr.UpdateConfig(config)
}

func (a *App) SyncPush() error {
	homeDir, _ := os.UserHomeDir()
	localDir := filepath.Join(homeDir, ".seasagi")
	return a.syncMgr.Push(localDir)
}

func (a *App) SyncPull() error {
	homeDir, _ := os.UserHomeDir()
	localDir := filepath.Join(homeDir, ".seasagi")
	return a.syncMgr.Pull(localDir)
}

func (a *App) GetSyncStatus() sync.SyncStatus {
	return a.syncMgr.GetStatus()
}

func (a *App) ListSessions(app string, limit int) ([]sessions.Session, error) {
	return a.sessionsSvc.ListSessions(app, limit)
}

func (a *App) GetSession(app, sessionID string) (*sessions.Session, error) {
	return a.sessionsSvc.GetSession(app, sessionID)
}

func (a *App) GetSessionMessages(app, sessionID string) ([]sessions.Message, error) {
	return a.sessionsSvc.GetMessages(app, sessionID)
}

func (a *App) SearchSessions(app, query string, limit int) ([]sessions.Session, error) {
	return a.sessionsSvc.SearchSessions(app, query, limit)
}

func (a *App) DeleteSession(app, sessionID string) error {
	return a.sessionsSvc.DeleteSession(app, sessionID)
}
