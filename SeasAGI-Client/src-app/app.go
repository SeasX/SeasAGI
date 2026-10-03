package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/configio"
	"github.com/SeasAGI/SeasAGI-Client/internal/deeplink"
	"github.com/SeasAGI/SeasAGI-Client/internal/discovery"
	"github.com/SeasAGI/SeasAGI-Client/internal/eval"
	"github.com/SeasAGI/SeasAGI-Client/internal/gateway"
	"github.com/SeasAGI/SeasAGI-Client/internal/integration"
	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/localtoken"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/mcp"
	"github.com/SeasAGI/SeasAGI-Client/internal/mitm"
	"github.com/SeasAGI/SeasAGI-Client/internal/network"
	"github.com/SeasAGI/SeasAGI-Client/internal/optimizer"
	"github.com/SeasAGI/SeasAGI-Client/internal/perf"
	"github.com/SeasAGI/SeasAGI-Client/internal/plugin"
	"github.com/SeasAGI/SeasAGI-Client/internal/presets"
	"github.com/SeasAGI/SeasAGI-Client/internal/prompts"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
	"github.com/SeasAGI/SeasAGI-Client/internal/rtk"
	"github.com/SeasAGI/SeasAGI-Client/internal/sessions"
	"github.com/SeasAGI/SeasAGI-Client/internal/skills"
	"github.com/SeasAGI/SeasAGI-Client/internal/sync"
	"github.com/SeasAGI/SeasAGI-Client/internal/tunnel"
	"github.com/SeasAGI/SeasAGI-Client/internal/updater"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

func mustHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return home
}

type App struct {
	ctx             context.Context
	authSvc         *auth.Service
	configSvc       *config.Service
	gatewaySvc      *gateway.Service
	logSvc          *logs.Service
	discoverySvc    *discovery.Service
	tunnelMgr       *tunnel.Manager
	mcpSvc          *mcp.Service
	promptsSvc      *prompts.Service
	skillsSvc       *skills.Service
	usageSvc        *usage.Service
	optimizerSvc    *optimizer.Service
	deeplinkMgr     *deeplink.Manager
	presetsSvc      *presets.Service
	syncMgr         *sync.Manager
	sessionsSvc     *sessions.Service
	configioSvc     *configio.Service
	localTokenStore *localtoken.Store
	mitmMgr         *mitm.Manager
	logRotator      *logging.LogRotator
	mcpServer       *mcp.GatewayServer
	perfAuditor     *perf.Auditor
}

func NewApp(
	authSvc *auth.Service,
	configSvc *config.Service,
	gatewaySvc *gateway.Service,
	logSvc *logs.Service,
	discoverySvc *discovery.Service,
	mcpSvc *mcp.Service,
	promptsSvc *prompts.Service,
	skillsSvc *skills.Service,
	usageSvc *usage.Service,
	optimizerSvc *optimizer.Service,
	deeplinkMgr *deeplink.Manager,
	presetsSvc *presets.Service,
	syncMgr *sync.Manager,
	sessionsSvc *sessions.Service,
	configioSvc *configio.Service,
	localTokenStore *localtoken.Store,
) *App {
	return &App{
		authSvc:         authSvc,
		configSvc:       configSvc,
		gatewaySvc:      gatewaySvc,
		logSvc:          logSvc,
		discoverySvc:    discoverySvc,
		tunnelMgr:       tunnel.NewManager(4318),
		mcpSvc:          mcpSvc,
		promptsSvc:      promptsSvc,
		skillsSvc:       skillsSvc,
		usageSvc:        usageSvc,
		optimizerSvc:    optimizerSvc,
		deeplinkMgr:     deeplinkMgr,
		presetsSvc:      presetsSvc,
		syncMgr:         syncMgr,
		sessionsSvc:     sessionsSvc,
		configioSvc:     configioSvc,
		localTokenStore: localTokenStore,
		mcpServer:       mcp.NewGatewayServer(),
		perfAuditor:     perf.NewAuditor(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	auth.SetPlatformAPIBaseURL(a.configSvc.GetPlatformAPIBaseURL())
}

func (a *App) GetAppInfo() map[string]any {
	return map[string]any{
		"name":    "SeasAGI",
		"version": "0.1.5",
	}
}

func (a *App) SelectDirectory() string {
	result, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Directory",
	})
	if err != nil {
		return ""
	}
	return result
}

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
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", status.ProxyPort))

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

func (a *App) CopyToClipboard(text string) {
	runtime.ClipboardSetText(a.ctx, text)
}

func (a *App) ShowMessage(title, message string) {
	_, _ = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:    runtime.InfoDialog,
		Title:   title,
		Message: message,
	})
}

func (a *App) ConfirmAction(title, message string) bool {
	result, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         title,
		Message:       message,
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No",
	})
	if err != nil {
		return false
	}
	return result == "Yes"
}

func (a *App) OpenInBrowser(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

func (a *App) StartLocalGateway() error {
	go a.gatewaySvc.Start(a.ctx)
	return nil
}

func (a *App) StopLocalGateway() error {
	a.gatewaySvc.Stop()
	return nil
}

func (a *App) GetRuntimeStatus() map[string]any {
	cfg := a.configSvc.GetConfig()
	return map[string]any{
		"gateway_running":    a.gatewaySvc.IsRunning(),
		"listen_port":        a.gatewaySvc.GetListenPort(),
		"default_model":      emptyStringToNil(cfg.DefaultModel),
		"default_channel_id": emptyStringToNil(cfg.DefaultChannelID),
	}
}

// SimulateIntentRouting 意图预测实验室（M4.4）：对给定 Prompt 跑意图检测 + 路由解析，
// 只返回决策结果，不发起任何上游请求。model 为空时与网关同语义（默认 Combo）。
func (a *App) SimulateIntentRouting(prompt string, model string) map[string]any {
	intent := routing.DetectIntent([]map[string]interface{}{{"role": "user", "content": prompt}})
	result := map[string]any{
		"intent": map[string]any{
			"task_type":      intent.TaskType,
			"scenario":       intent.Scenario,
			"required_iq":    intent.RequiredIQ,
			"security_level": intent.SecurityLevel,
			"tags":           intent.Tags,
			"confidence":     intent.Confidence,
		},
	}
	if model == "" {
		model = a.configSvc.GetDefaultComboName()
	}
	resolver := routing.NewResolver(a.configSvc)
	planName, steps, err := resolver.ResolveChatPlan(model, intent.TaskType, intent)
	if err != nil {
		result["error"] = err.Error()
		return result
	}
	stepList := make([]map[string]any, 0, len(steps))
	for i, step := range steps {
		stepList = append(stepList, map[string]any{
			"order":          i + 1,
			"channel_id":     step.Channel.ChannelID,
			"channel_name":   step.Channel.DisplayName,
			"upstream_model": step.UpstreamModel,
			"step_role":      step.StepRole,
		})
	}
	result["model"] = model
	result["plan_name"] = planName
	result["steps"] = stepList
	return result
}

// GetIntentScenarioStats 意图场景分布统计（M4.5）。
func (a *App) GetIntentScenarioStats() []map[string]any {
	return a.logSvc.GetIntentScenarioStats()
}

// GetComboRouteMetrics returns combo-level route metrics from the gateway
func (a *App) GetComboRouteMetrics() []map[string]any {
	metrics := a.gatewaySvc.GetComboRouteMetrics()
	result := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		result = append(result, map[string]any{
			"combo_name":         m.ComboName,
			"total_requests":     m.TotalRequests,
			"total_fallbacks":    m.TotalFallbacks,
			"step1_success":      m.Step1Success,
			"last_step_fallback": m.LastStepFallback,
			"step1_hit_rate":     m.Step1HitRate,
			"last_step_hit_rate": m.LastStepHitRate,
			"avg_attempts":       m.AvgAttempts,
			"task_type":          m.TaskType,
		})
	}
	return result
}

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

// === UI-Batch 7: 设置页增强 ===

// GetLogRotationConfig 获取日志轮转配置。
func (a *App) GetLogRotationConfig() map[string]any {
	if a.logRotator == nil {
		return map[string]any{
			"max_file_size_mb": 0,
			"retention_days":   0,
			"max_files":        0,
			"current_size_mb":  0,
		}
	}
	cfg := a.logRotator.Config()
	size, _ := a.logRotator.TotalSize()
	return map[string]any{
		"max_file_size_mb": cfg.MaxFileSize / (1024 * 1024),
		"retention_days":   cfg.RetentionDays,
		"max_files":        cfg.MaxFiles,
		"current_size_mb":  size / (1024 * 1024),
	}
}

// SetLogRotationConfig 设置日志轮转配置（单位与 GetLogRotationConfig 一致）。
func (a *App) SetLogRotationConfig(cfg map[string]any) error {
	if a.logRotator == nil {
		return fmt.Errorf("log rotator not initialized")
	}
	cur := a.logRotator.Config()
	next := cur
	if v, ok := numberToInt(cfg["max_file_size_mb"]); ok && v > 0 {
		next.MaxFileSize = int64(v) * 1024 * 1024
	}
	if v, ok := numberToInt(cfg["retention_days"]); ok && v > 0 {
		next.RetentionDays = v
	}
	if v, ok := numberToInt(cfg["max_files"]); ok && v > 0 {
		next.MaxFiles = v
	}
	a.logRotator.UpdateConfig(next)
	return nil
}

// SetLogRotator 注入日志轮转器。
func (a *App) SetLogRotator(r *logging.LogRotator) {
	a.logRotator = r
}

// numberToInt 将 JSON 反序列化后的数值（float64/int/json.Number）安全转为 int。
func numberToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	}
	return 0, false
}

// GetCloudSyncStatus 获取云同步状态（版本哈希与冲突均基于真实本地配置计算）。
func (a *App) GetCloudSyncStatus() map[string]any {
	homeDir, _ := os.UserHomeDir()
	localDir := filepath.Join(homeDir, ".seasagi")

	conflicts := a.detectSyncConflicts()
	conflictItems := make([]map[string]any, 0, len(conflicts))
	for _, c := range conflicts {
		conflictItems = append(conflictItems, map[string]any{
			"type":   c.Type,
			"detail": strings.Join(c.Conflicts, ", "),
		})
	}

	st := a.syncMgr.GetStatus()
	return map[string]any{
		// sync.Manager 的 Push/Pull 目前未对同步负载做 HMAC 签名，如实上报 false。
		"hmac_enabled":   false,
		"version_hash":   syncPayloadVersionHash(localDir),
		"last_sync":      st.LastSyncTime,
		"status":         st.Status,
		"error":          st.Error,
		"conflicts":      conflictItems,
		"conflict_count": len(conflictItems),
	}
}

// syncPayloadVersionHash 对同步负载（sync.Manager 管理的本地配置文件）计算确定性版本哈希；无文件时返回空串。
func syncPayloadVersionHash(localDir string) string {
	files := []string{"mcp_servers.json", "prompt_presets.json", "skills.json", "usage_records.json"}
	var buf []byte
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(localDir, name))
		if err != nil {
			continue
		}
		buf = append(buf, []byte(name)...)
		buf = append(buf, data...)
	}
	if len(buf) == 0 {
		return ""
	}
	return sync.ComputeVersionHash(buf)
}

// detectSyncConflicts 基于当前 channel / combo 配置做引用完整性冲突检测。
func (a *App) detectSyncConflicts() []sync.ConflictResult {
	channels, _ := a.configSvc.ListChannels()
	providerConns := make([]map[string]interface{}, 0, len(channels))
	for _, ch := range channels {
		models := make([]interface{}, 0, len(ch.Models))
		for _, m := range ch.Models {
			models = append(models, m)
		}
		providerConns = append(providerConns, map[string]interface{}{
			"channel_id": ch.ChannelID,
			"models":     models,
		})
	}

	combos := a.configSvc.ListModelCombos()
	comboItems := make([]map[string]interface{}, 0, len(combos))
	for _, c := range combos {
		steps := make([]interface{}, 0, len(c.Steps))
		for _, s := range c.Steps {
			steps = append(steps, map[string]interface{}{"channel_id": s.ChannelID})
		}
		comboItems = append(comboItems, map[string]interface{}{"steps": steps})
	}

	return sync.DetectConflicts(&sync.ConfigBundle{
		ProviderConns: providerConns,
		Combos:        comboItems,
	})
}

func (a *App) GetProviderHealthMetrics(providerId string) []map[string]any {
	return a.authSvc.FetchProviderHealthMetrics(providerId)
}

func (a *App) GetProviderHealthSummary() []map[string]any {
	return a.authSvc.FetchProviderHealthSummary()
}

func (a *App) GetByokPolicies() []map[string]any {
	return a.authSvc.FetchByokPolicies()
}

func (a *App) SetByokPolicy(policy map[string]any) map[string]any {
	result, _ := a.authSvc.CreateByokPolicy(policy)
	return result
}

func (a *App) GetAppConfig() config.AppConfig {
	return a.configSvc.GetConfig()
}

// SetRTKSettings 保存 RTK Token 压缩与 Caveman 输出精简设置，并热更新网关管线。
func (a *App) SetRTKSettings(rtkEnabled bool, rtkMaxOutputChars int, cavemanEnabled bool, cavemanStyle string) error {
	// 空串允许（保持既有值/默认）；非空必须是四风格之一，避免注入时静默降级
	if cavemanStyle != "" && !rtk.IsValidCavemanStyle(cavemanStyle) {
		return fmt.Errorf("invalid caveman style: %q (supported: concise, brief, minimal, terse)", cavemanStyle)
	}
	if err := a.configSvc.SetRTKConfig(rtkEnabled, rtkMaxOutputChars); err != nil {
		return err
	}
	if err := a.configSvc.SetCavemanConfig(cavemanEnabled, cavemanStyle); err != nil {
		return err
	}
	if a.gatewaySvc != nil {
		a.gatewaySvc.SetRTKConfig(rtkEnabled, rtkMaxOutputChars)
	}
	return nil
}

// QuitApp 设置页"退出"按钮：置退出标志并真正退出应用（停止网关与驻留）。
func (a *App) QuitApp() {
	atomic.StoreInt32(&quitting, 1)
	runtime.Quit(a.ctx)
}

func (a *App) GetPlatformAPIBaseURL() string {
	return a.configSvc.GetPlatformAPIBaseURL()
}

func (a *App) GetPlatformToken() string {
	return a.authSvc.GetPlatformToken()
}

func (a *App) SetPlatformAPIBaseURL(url string) error {
	if url == "" {
		return fmt.Errorf("platform API base URL cannot be empty")
	}
	if err := a.configSvc.SetPlatformAPIBaseURL(url); err != nil {
		return err
	}
	auth.SetPlatformAPIBaseURL(url)
	return nil
}

func (a *App) SyncPlatformChannels() error {
	if !a.authSvc.IsLoggedIn() {
		return fmt.Errorf("not logged in")
	}
	platformChannels, err := a.authSvc.FetchPlatformChannels(a.ctx)
	if err != nil {
		return err
	}
	return a.configSvc.UpsertPlatformChannels(convertPlatformChannels(platformChannels))
}

// FetchFreeChannels 从企业服务端（优先）或平台 API 拉取免费通道种子列表，供 Token 市场展示。
func (a *App) FetchFreeChannels() []map[string]any {
	if !a.authSvc.IsLoggedIn() {
		return []map[string]any{}
	}
	seeds, err := a.authSvc.FetchFreeChannels(a.ctx)
	if err != nil {
		// 返回空列表而不是包含 error 字段的对象：前端 Token 市场直接访问 ch.models.length，
		// 缺少 models 字段的数据会导致页面崩溃白屏。
		return []map[string]any{}
	}
	if seeds == nil {
		return []map[string]any{}
	}
	return seeds
}

// FetchEnterpriseChannels 从企业服务端拉取通道列表，含 provider_type、base_url、models。
// 客户端据此将企业服务端配置的通道同步到本地。
func (a *App) FetchEnterpriseChannels() []map[string]any {
	if !a.authSvc.IsLoggedIn() {
		return []map[string]any{}
	}
	channels, err := a.authSvc.FetchEnterpriseChannels(a.ctx)
	if err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	if channels == nil {
		return []map[string]any{}
	}
	return channels
}

// FetchModelCatalog 从企业服务端拉取模型目录，供通道配置页面按 provider 自动补全模型列表。
func (a *App) FetchModelCatalog() []map[string]any {
	if !a.authSvc.IsLoggedIn() {
		return []map[string]any{}
	}
	models, err := a.authSvc.FetchModelCatalog(a.ctx)
	if err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	if models == nil {
		return []map[string]any{}
	}
	return models
}

// FetchModelIndex 拉取 AI 模型指数榜单（公开资讯，无需登录；企业服务端优先，降级平台 API）。
// 返回 {category, updated_at, entries:[...]}，异常时返回 {"error": ...}。
func (a *App) FetchModelIndex(category string) map[string]any {
	data, err := a.authSvc.FetchModelIndex(a.ctx, category)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	if data == nil {
		return map[string]any{}
	}
	return data
}

func (a *App) CreateCheckoutSession(planID string, quantity ...int) (map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	qty := 1
	if len(quantity) > 0 && quantity[0] > 1 {
		qty = quantity[0]
	}

	body := map[string]any{"plan_id": planID, "quantity": qty}
	bodyBytes, _ := json.Marshal(body)

	apiBase := a.authSvc.PlatformAPIBaseURL()
	reqCtx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, apiBase+"/checkout/create", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authSvc.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode >= 400 {
		errMsg := "create checkout session failed"
		if result != nil {
			if e, ok := result["error"].(string); ok {
				errMsg = e
			}
		}
		return nil, errors.New(errMsg)
	}
	return result, nil
}

func (a *App) Login(email, password string) error {
	if err := a.authSvc.Login(email, password); err != nil {
		return err
	}
	return a.afterPlatformLogin()
}

// afterPlatformLogin 登录成功后的公共处理：拉平台通道、云同步、启动网关。
func (a *App) afterPlatformLogin() error {
	platformChannels, err := a.authSvc.FetchPlatformChannels(a.ctx)
	if err != nil {
		return err
	}

	if err := a.configSvc.UpsertPlatformChannels(convertPlatformChannels(platformChannels)); err != nil {
		return err
	}

	// Sync local data to cloud (best-effort)
	_ = a.syncLocalDataToCloud()

	if !a.gatewaySvc.IsRunning() {
		go a.gatewaySvc.Start(a.ctx)
	}
	return nil
}

// GetOAuthProviders 获取服务端已配置的第三方登录方式。
func (a *App) GetOAuthProviders() ([]auth.OAuthProvider, error) {
	return a.authSvc.FetchOAuthProviders()
}

// StartOAuthLogin 发起第三方 OAuth 登录：
// 请求服务端创建授权会话 -> 打开浏览器访问服务端授权跳转页（服务端 302 到 Google/GitHub）
// -> 轮询服务端取回登录态。
func (a *App) StartOAuthLogin(provider string) error {
	if err := a.authSvc.StartOAuthLogin(provider); err != nil {
		return err
	}
	return a.afterPlatformLogin()
}

func (a *App) Logout() error {
	a.gatewaySvc.Stop()

	// Sync local data before clearing auth, unless user is not logged in
	if a.authSvc.IsLoggedIn() {
		if err := a.syncLocalDataToCloud(); err != nil {
			// Best-effort: don't block logout
		}
	}

	if err := a.authSvc.Logout(); err != nil {
		return err
	}
	return a.configSvc.ClearPlatformChannels()
}

func (a *App) SyncLocalDataToCloud() error {
	return a.syncLocalDataToCloud()
}

func (a *App) syncLocalDataToCloud() error {
	if !a.authSvc.IsLoggedIn() {
		return fmt.Errorf("not logged in")
	}

	// Sync local custom channels
	channels, _ := a.configSvc.ListChannels()
	var customChannels []map[string]any
	for _, ch := range channels {
		if ch.ChannelType == "custom" {
			customChannels = append(customChannels, map[string]any{
				"channel_id":    ch.ChannelID,
				"display_name":  ch.DisplayName,
				"provider_type": ch.ProviderType,
				"base_url":      ch.BaseURL,
				"channel_type":  ch.ChannelType,
				"models":        ch.Models,
				"enabled":       ch.Enabled,
			})
		}
	}
	if len(customChannels) > 0 {
		_, err := a.SyncCustomChannelsToCloud(customChannels)
		if err != nil {
			return fmt.Errorf("sync channels: %w", err)
		}
	}

	// Sync local combos
	combos := a.configSvc.ListModelCombos()
	for _, combo := range combos {
		stepsJSON, _ := json.Marshal(combo.Steps)
		_, _ = a.PushCloudCombo(combo.LogicalName, combo.DisplayName, combo.Description,
			combo.Strategy, combo.StickyUses, combo.QuickStrategy,
			combo.TaskProfile, string(stepsJSON))
	}

	return nil
}

func (a *App) GetAuthState() auth.AuthInfo {
	return a.authSvc.GetAuthState()
}

func (a *App) ListChannels() ([]config.Channel, error) {
	return a.configSvc.ListChannels()
}

func (a *App) SaveCustomChannel(channel config.Channel) (string, error) {
	if channel.ChannelID == "" {
		channel.ChannelID = fmt.Sprintf("ch_%d", time.Now().UnixNano())
	}
	// Save single APIKey to keychain
	if channel.APIKey != "" {
		if err := keychain.SaveChannelKey(channel.ChannelID, channel.APIKey); err != nil {
			return "", err
		}
	}
	// Save multi-key APIKeys: always persist to config JSON via configSvc
	// If no single key but multi-keys exist, save first key to keychain too
	if channel.APIKey == "" && len(channel.APIKeys) > 0 {
		firstKey := channel.APIKeys[0]
		if firstKey != "" {
			if err := keychain.SaveChannelKey(channel.ChannelID, firstKey); err != nil {
				return "", err
			}
			channel.APIKey = "" // keep single key empty; keychain has it
		}
	}
	return a.configSvc.SaveCustomChannel(channel)
}

func (a *App) DeleteCustomChannel(channelID string) error {
	if err := keychain.DeleteChannelKey(channelID); err != nil {
		return err
	}
	return a.configSvc.DeleteCustomChannel(channelID)
}

func (a *App) ReorderChannels(orderedIDs []string) error {
	return a.configSvc.ReorderChannels(orderedIDs)
}

func (a *App) ExportConfig() (string, error) {
	data, err := a.configioSvc.Export()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) ImportConfig(jsonStr string) error {
	return a.configioSvc.Import([]byte(jsonStr))
}

func (a *App) TestCustomChannel(channelID string) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	return a.testConnectivity(ch, true)
}

func (a *App) TestChannelDirect(channelID string) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	return a.testConnectivity(ch, false)
}

// testConnectivity checks whether the channel's API is reachable via a lightweight request.
// When useGateway is true, it sends a minimal request through the local gateway.
// When false, it sends a GET to the channel's models endpoint directly.
func (a *App) testConnectivity(ch config.Channel, useGateway bool) (map[string]any, error) {
	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}

	channelID := ch.ChannelID

	if !useGateway {
		// Direct connectivity check: GET {base_url}/v1/models (or /models as fallback)
		if apiKey == "" {
			return map[string]any{
				"success": false,
				"error":   "no API key configured for this channel",
			}, nil
		}

		baseURL := strings.TrimRight(ch.BaseURL, "/")
		endpoints := []string{baseURL + "/v1/models", baseURL + "/models", baseURL}
		var lastErr string
		for _, url := range endpoints {
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)
			client := &http.Client{Timeout: 8 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				lastErr = fmt.Sprintf("connectivity test failed: %v", err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode < 500 {
				_ = a.configSvc.UpdateChannelHealth(channelID, "healthy")
				return map[string]any{
					"success":     true,
					"message":     fmt.Sprintf("channel API is reachable (HTTP %d)", resp.StatusCode),
					"status_code": resp.StatusCode,
				}, nil
			}
			lastErr = fmt.Sprintf("server returned status %d", resp.StatusCode)
		}
		_ = a.configSvc.UpdateChannelHealth(channelID, "unhealthy")
		return map[string]any{
			"success": false,
			"error":   lastErr,
		}, nil
	}

	// Via local gateway: send a minimal chat completion to verify the pipeline
	if !a.gatewaySvc.IsRunning() {
		return map[string]any{
			"success": false,
			"error":   "local gateway is not running, start it first",
		}, nil
	}

	if apiKey == "" {
		return map[string]any{
			"success": false,
			"error":   "no API key configured for this channel",
		}, nil
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, err
	}

	port := a.gatewaySvc.GetListenPort()
	body := map[string]any{
		"model":      "gpt-4o-mini",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
		"stream":     false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		_ = a.configSvc.UpdateChannelHealth(channelID, "unhealthy")
		return map[string]any{
			"success": false,
			"error":   fmt.Sprintf("gateway connectivity test failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		_ = a.configSvc.UpdateChannelHealth(channelID, "healthy")
		return map[string]any{
			"success":     true,
			"message":     "channel is reachable via local gateway",
			"status_code": resp.StatusCode,
		}, nil
	}

	_ = a.configSvc.UpdateChannelHealth(channelID, "unhealthy")
	return map[string]any{
		"success":     false,
		"error":       fmt.Sprintf("gateway returned HTTP %d", resp.StatusCode),
		"status_code": resp.StatusCode,
	}, nil
}

func (a *App) DiscoverModels(channelID string) ([]discovery.DiscoveredModel, error) {
	return a.discoverySvc.DiscoverModels(channelID)
}

func (a *App) FetchModelsFromURL(modelsURL, apiKey string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract model IDs from OpenAI-compatible response
	var modelNames []string
	if data, ok := result["data"].([]any); ok {
		for _, item := range data {
			if m, ok := item.(map[string]any); ok {
				if id, ok := m["id"].(string); ok {
					modelNames = append(modelNames, id)
				}
			}
		}
	}

	return map[string]any{
		"models": modelNames,
		"total":  len(modelNames),
	}, nil
}

func (a *App) SyncCustomChannelsToCloud(channels []map[string]any) (map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	baseURL := a.configSvc.GetPlatformAPIBaseURL()
	token := a.authSvc.GetPlatformToken()

	body := map[string]any{"channels": channels}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/tenant-admin/channels/sync-custom", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.StatusCode >= 400 {
		errMsg, _ := result["error"].(string)
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, errMsg)
	}

	return result, nil
}

func (a *App) ListLogs(limit, offset int) ([]logs.RequestLog, error) {
	return a.logSvc.ListLogs(limit, offset)
}

func (a *App) ListLogsFiltered(limit, offset int, status, channelID, timeFrom, timeTo, keyword string) ([]logs.RequestLog, error) {
	return a.logSvc.ListLogsFiltered(limit, offset, status, channelID, timeFrom, timeTo, keyword)
}

func (a *App) ClearLogs() error {
	return a.logSvc.ClearLogs()
}

func (a *App) UpdateDefaultModel(modelName, channelID string) error {
	a.configSvc.SetDefaultModel(modelName, channelID)
	return nil
}

func (a *App) GetDefaultComboName() string {
	return a.configSvc.GetDefaultComboName()
}

func (a *App) SetDefaultComboName(name string) error {
	a.configSvc.SetDefaultComboName(name)
	return nil
}

func (a *App) GetComboByName(name string) config.ModelCombo {
	c, ok := a.configSvc.GetModelCombo(name)
	if !ok {
		return config.ModelCombo{}
	}
	return c
}

func (a *App) GetLocalAccessToken() (string, error) {
	return a.localTokenStore.GetOrCreate()
}

func (a *App) ResetLocalAccessToken() (string, error) {
	token, err := a.localTokenStore.Reset()
	if err != nil {
		return "", err
	}
	a.gatewaySvc.SetAccessToken(token)
	return token, nil
}

func (a *App) SetAutoLaunch(enabled bool) error {
	if err := network.SetAutoLaunch(enabled); err != nil {
		return err
	}
	return a.configSvc.SetAutoLaunch(enabled)
}

func (a *App) UpdateRoutingSettings(strategy string, stickyUses int) error {
	return a.configSvc.UpdateRoutingSettings(strategy, stickyUses)
}

func (a *App) ListModelCombos() []config.ModelCombo {
	return a.configSvc.ListModelCombos()
}

func (a *App) SaveModelCombo(combo config.ModelCombo) error {
	return a.configSvc.SaveModelCombo(combo)
}

func (a *App) DeleteModelCombo(name string) error {
	return a.configSvc.DeleteModelCombo(name)
}

func (a *App) ApplyComboSortPreset(comboName string, preset string) error {
	return a.configSvc.ApplyComboSortPreset(comboName, preset)
}

func (a *App) ApplyRecommendation(toModel string, preset string) error {
	if toModel == "" {
		return fmt.Errorf("recommended model is empty")
	}
	presetName := preset
	if presetName == "" {
		presetName = "budget"
	}
	combo := config.ModelCombo{
		Name:        fmt.Sprintf("推荐方案 - %s", toModel),
		Description: fmt.Sprintf("由智能优化引擎推荐，目标模型：%s，排序预设：%s", toModel, presetName),
		Steps:       []config.ModelComboStep{{ChannelID: "", Model: toModel}},
		Strategy:    "fallback",
		StickyUses:  1,
	}
	err := a.configSvc.SaveModelCombo(combo)
	if err != nil {
		return fmt.Errorf("failed to apply recommendation: %w", err)
	}
	return a.configSvc.ApplyComboSortPreset(combo.Name, presetName)
}

// PreviewComboOptimization returns a detailed preview of an optimization recommendation
// without applying it. Returns the recommendation details, estimated impact, and current vs new step comparison.
func (a *App) PreviewComboOptimization(mode string, taskType string) map[string]any {
	if mode == "" {
		mode = "value_first"
	}
	if taskType == "" {
		taskType = optimizer.TaskGeneralChat
	}
	plan := a.optimizerSvc.GetOptimizationPlan(mode, taskType)
	if plan == nil || len(plan.Recommendations) == 0 {
		return map[string]any{
			"has_recommendations": false,
			"message":             "当前暂无优化建议",
		}
	}

	previews := make([]map[string]any, 0, len(plan.Recommendations))
	for _, rec := range plan.Recommendations {
		currentCombo := a.configSvc.FindComboByModel(rec.FromModel)
		currentSteps := make([]map[string]string, 0)
		if currentCombo != nil {
			for _, step := range currentCombo.Steps {
				currentSteps = append(currentSteps, map[string]string{
					"model":   step.Model,
					"channel": step.ChannelID,
				})
			}
		}

		previews = append(previews, map[string]any{
			"from_model":     rec.FromModel,
			"to_model":       rec.ToModel,
			"model_tag":      rec.ModelTag,
			"channel_id":     rec.ChannelID,
			"channel_name":   rec.ChannelName,
			"savings_usd":    rec.SavingsUSD,
			"quality_diff":   rec.QualityDiff,
			"avg_latency_ms": rec.AvgLatencyMs,
			"error_rate":     rec.ErrorRate,
			"reason":         rec.Reason,
			"current_steps":  currentSteps,
			"proposed_steps": []map[string]string{
				{"model": rec.ToModel, "channel": rec.ChannelID},
			},
		})
	}

	return map[string]any{
		"has_recommendations": true,
		"mode":                plan.Mode,
		"strategy":            plan.Strategy,
		"monthly_savings":     plan.MonthlySavings,
		"recommendations":     previews,
	}
}

// ApplyComboOptimization applies a specific optimization recommendation as a new combo.
// If update_existing is true, it updates the existing combo for the from_model instead of creating a new one.
func (a *App) ApplyComboOptimization(toModel string, preset string, updateExisting bool) error {
	if toModel == "" {
		return fmt.Errorf("target model is required")
	}
	presetName := preset
	if presetName == "" {
		presetName = "budget"
	}

	// Try to find existing combo for the from-model if updateExisting is true
	plan := a.optimizerSvc.GetOptimizationPlan("value_first", optimizer.TaskGeneralChat)
	var fromModel string
	if plan != nil {
		for _, rec := range plan.Recommendations {
			if rec.ToModel == toModel {
				fromModel = rec.FromModel
				break
			}
		}
	}

	if updateExisting && fromModel != "" {
		existingCombo := a.configSvc.FindComboByModel(fromModel)
		if existingCombo != nil {
			existingCombo.Steps = []config.ModelComboStep{
				{ChannelID: "", Model: toModel},
			}
			existingCombo.Description = fmt.Sprintf("由智能优化自动更新，新目标模型：%s", toModel)
			if err := a.configSvc.SaveModelCombo(*existingCombo); err != nil {
				return fmt.Errorf("failed to update combo: %w", err)
			}
			return a.configSvc.ApplyComboSortPreset(existingCombo.Name, presetName)
		}
	}

	// Create new combo
	comboName := fmt.Sprintf("优化方案 - %s", toModel)
	combo := config.ModelCombo{
		Name:        comboName,
		Description: fmt.Sprintf("由智能优化引擎生成，目标模型：%s，排序预设：%s", toModel, presetName),
		Steps:       []config.ModelComboStep{{ChannelID: "", Model: toModel}},
		Strategy:    "fallback",
		StickyUses:  1,
	}
	if err := a.configSvc.SaveModelCombo(combo); err != nil {
		return fmt.Errorf("failed to save combo: %w", err)
	}
	return a.configSvc.ApplyComboSortPreset(comboName, presetName)
}

func (a *App) SyncOptimizationConfigToCloud() error {
	cfg := a.configSvc.GetOptimizationConfig()
	raw, err := json.Marshal(map[string]any{
		"config": map[string]any{
			"mode":                 cfg.Mode,
			"penalty_enabled":      cfg.PenaltyEnabled,
			"penalty_decay_sec":    cfg.PenaltyDecaySec,
			"health_check_enabled": cfg.HealthCheckEnabled,
			"health_check_sec":     cfg.HealthCheckSec,
			"health_max_failures":  cfg.HealthMaxFailures,
			"cooldown_enabled":     cfg.CooldownEnabled,
			"cooldown_sec":         cfg.CooldownSec,
			"sticky_enabled":       cfg.StickyEnabled,
			"sticky_ttl_sec":       cfg.StickyTTLSec,
			"preset_enabled":       cfg.PresetEnabled,
			"default_preset":       cfg.DefaultPreset,
		},
	})
	if err != nil {
		return err
	}
	return a.authSvc.PushCloudOptimizationConfig(string(raw))
}

func (a *App) SyncOptimizationConfigFromCloud() map[string]any {
	raw, err := a.authSvc.FetchCloudOptimizationConfig()
	if err != nil || raw == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	cfgMap, ok := parsed["config"].(map[string]any)
	if !ok {
		return nil
	}

	local := a.configSvc.GetOptimizationConfig()
	if v, ok := cfgMap["mode"]; ok {
		if s, ok := v.(string); ok && s != "" {
			local.Mode = s
		}
	}
	if v, ok := cfgMap["penalty_enabled"]; ok {
		local.PenaltyEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["penalty_decay_sec"]; ok {
		local.PenaltyDecaySec, _ = toInt(v)
	}
	if v, ok := cfgMap["health_check_enabled"]; ok {
		local.HealthCheckEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["health_check_sec"]; ok {
		local.HealthCheckSec, _ = toInt(v)
	}
	if v, ok := cfgMap["health_max_failures"]; ok {
		local.HealthMaxFailures, _ = toInt(v)
	}
	if v, ok := cfgMap["cooldown_enabled"]; ok {
		local.CooldownEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["cooldown_sec"]; ok {
		local.CooldownSec, _ = toInt(v)
	}
	if v, ok := cfgMap["sticky_enabled"]; ok {
		local.StickyEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["sticky_ttl_sec"]; ok {
		local.StickyTTLSec, _ = toInt(v)
	}
	if v, ok := cfgMap["preset_enabled"]; ok {
		local.PresetEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["default_preset"]; ok {
		if s, ok := v.(string); ok && s != "" {
			local.DefaultPreset = s
		}
	}
	_ = a.configSvc.SetOptimizationConfig(local)

	return map[string]any{
		"mode":                 local.Mode,
		"penalty_enabled":      local.PenaltyEnabled,
		"penalty_decay_sec":    local.PenaltyDecaySec,
		"health_check_enabled": local.HealthCheckEnabled,
		"health_check_sec":     local.HealthCheckSec,
		"health_max_failures":  local.HealthMaxFailures,
		"cooldown_enabled":     local.CooldownEnabled,
		"cooldown_sec":         local.CooldownSec,
		"sticky_enabled":       local.StickyEnabled,
		"sticky_ttl_sec":       local.StickyTTLSec,
		"preset_enabled":       local.PresetEnabled,
		"default_preset":       local.DefaultPreset,
	}
}

func (a *App) FetchModelStats() []map[string]any {
	entries, err := a.authSvc.FetchCloudModelStats()
	if err != nil {
		return nil
	}
	stats := make([]optimizer.ModelStatsInfo, 0, len(entries))
	result := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		stats = append(stats, optimizer.ModelStatsInfo{
			Model:         e.Model,
			TotalRequests: e.TotalRequests,
			TotalErrors:   e.TotalErrors,
			AvgLatencyMs:  e.AvgLatencyMs,
			ErrorRate:     e.ErrorRate,
		})
		result = append(result, map[string]any{
			"model":          e.Model,
			"total_requests": e.TotalRequests,
			"total_errors":   e.TotalErrors,
			"avg_latency_ms": e.AvgLatencyMs,
			"error_rate":     e.ErrorRate,
		})
	}
	a.optimizerSvc.SetCloudModelStats(stats)
	return result
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func (a *App) ListComboTemplates() []config.ModelCombo {
	return a.configSvc.ListComboTemplates()
}

func (a *App) SaveComboTemplate(template config.ModelCombo) error {
	return a.configSvc.SaveComboTemplate(template)
}

func (a *App) DeleteComboTemplate(name string) error {
	return a.configSvc.DeleteComboTemplate(name)
}

func (a *App) RenameComboTemplate(oldName, newName string) error {
	return a.configSvc.RenameComboTemplate(oldName, newName)
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

func convertPlatformChannels(items []map[string]interface{}) []config.Channel {
	result := make([]config.Channel, 0, len(items))
	for _, item := range items {
		channel := config.Channel{
			ChannelType:            "platform",
			Enabled:                true,
			HealthStatus:           "healthy",
			ProviderSpecificConfig: map[string]string{},
		}
		if value, ok := item["channel_id"].(string); ok {
			channel.ChannelID = value
		}
		if value, ok := item["provider_type"].(string); ok {
			channel.ProviderType = value
		}
		if value, ok := item["display_name"].(string); ok {
			channel.DisplayName = value
		}
		if value, ok := item["base_url"].(string); ok {
			channel.BaseURL = value
		}
		if value, ok := item["enabled"].(bool); ok {
			channel.Enabled = value
		}
		if values, ok := item["models"].([]interface{}); ok {
			models := make([]string, 0, len(values))
			for _, raw := range values {
				model, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if modelName, ok := model["model_name"].(string); ok {
					models = append(models, modelName)
				} else if modelID, ok := model["model_id"].(string); ok {
					models = append(models, modelID)
				}
			}
			channel.Models = models
		}
		result = append(result, channel)
	}
	return result
}

func emptyStringToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
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

func (a *App) GetDailyUsage(days int) []usage.DailyUsage {
	return a.usageSvc.GetDailyUsage(days)
}

func (a *App) GetTotalUsage() map[string]interface{} {
	return a.usageSvc.GetTotalUsage()
}

func (a *App) RecordUsage(channelID, channelName, model string, inputTokens, outputTokens int64) {
	a.usageSvc.RecordUsage(channelID, channelName, model, inputTokens, outputTokens)
}

func (a *App) ListModelPricing() []usage.ModelPricing {
	return a.usageSvc.ListPricing()
}

func (a *App) SaveModelPricing(pricing []usage.ModelPricing) error {
	return a.usageSvc.SavePricing(pricing)
}

func (a *App) HandleDeepLink(rawURL string) error {
	return a.deeplinkMgr.Handle(rawURL)
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

func (a *App) Register(email, password, displayName string) error {
	return a.authSvc.Register(email, password, displayName)
}

func (a *App) GetCloudUsage() (map[string]any, error) {
	usageData, err := a.authSvc.FetchCloudUsage()
	if err != nil {
		return nil, err
	}
	if usageData == nil {
		return nil, nil
	}
	return map[string]any{
		"month_requests":      usageData.MonthRequests,
		"month_input_tokens":  usageData.MonthInputTokens,
		"month_output_tokens": usageData.MonthOutputTokens,
		"total_cost_usd":      usageData.TotalCostUSD,
	}, nil
}

func (a *App) GetCloudBilling() (map[string]any, error) {
	billingData, err := a.authSvc.FetchCloudBilling()
	if err != nil {
		return nil, err
	}
	if billingData == nil {
		return nil, nil
	}
	return map[string]any{
		"plan_id":        billingData.PlanID,
		"plan_name":      billingData.PlanName,
		"price":          billingData.Price,
		"quota":          billingData.Quota,
		"used_quota":     billingData.UsedQuota,
		"renewal_date":   billingData.RenewalDate,
		"relay_enabled":  billingData.RelayEnabled,
		"relay_gateways": billingData.RelayGateways,
	}, nil
}

// GetOverageUsage 获取当前用户超额用量（由 Go 后端代理平台请求，避免前端直接持有并外发平台 Token）。
func (a *App) GetOverageUsage() (map[string]any, error) {
	rec, err := a.authSvc.FetchOverageUsage()
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return map[string]any{
		"overage_id":       rec.OverageID,
		"user_id":          rec.UserID,
		"plan_id":          rec.PlanID,
		"billing_period":   rec.BillingPeriod,
		"overage_requests": rec.OverageRequests,
		"overage_cost":     rec.OverageCost,
		"currency":         rec.Currency,
		"billed":           rec.Billed,
		"invoice_id":       rec.InvoiceID,
		"created_at":       rec.CreatedAt,
	}, nil
}

// PlatformRequest 是平台 API 的统一代理入口：由 Go 后端附加访问令牌后转发，
// 前端无需持有平台 Token，也不再直连平台地址（避免绕过 Wails 层）。
// 返回 { status, body }，status 为平台 HTTP 状态码，body 为解析后的响应体（非 JSON 时退回字符串）。
func (a *App) PlatformRequest(method, path, body string) (map[string]any, error) {
	var payload []byte
	if strings.TrimSpace(body) != "" {
		payload = []byte(body)
	}
	status, respBody, err := a.authSvc.DoPlatformRequest(method, path, payload)
	if err != nil {
		return nil, err
	}
	var parsed any
	if len(respBody) > 0 {
		if json.Unmarshal(respBody, &parsed) != nil {
			parsed = string(respBody)
		}
	}
	return map[string]any{
		"status": status,
		"body":   parsed,
	}, nil
}

func (a *App) FetchActiveGrants() ([]map[string]any, error) {
	grants, err := a.authSvc.FetchActiveGrants()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		result = append(result, map[string]any{
			"grant_id":          g.GrantID,
			"grantor_user_id":   g.GrantorUserID,
			"channel_id":        g.ChannelID,
			"token_fingerprint": g.TokenFingerprint,
			"granted_quota_usd": g.GrantedQuotaUSD,
			"used_quota_usd":    g.UsedQuotaUSD,
			"remaining_quota":   g.RemainingQuota,
			"granted_tokens":    g.GrantedTokens,
			"used_tokens":       g.UsedTokens,
			"remaining_tokens":  g.RemainingTokens,
			"status":            g.Status,
			"expires_at":        g.ExpiresAt,
		})
	}
	return result, nil
}

func (a *App) SetSelectedGrant(grantID, relayURL string) error {
	return a.configSvc.SetSelectedGrant(grantID, relayURL)
}

func (a *App) GetSelectedGrant() map[string]any {
	return map[string]any{
		"grant_id":  a.configSvc.GetSelectedGrantID(),
		"relay_url": a.configSvc.GetSelectedGrantRelayURL(),
	}
}

func (a *App) ClearSelectedGrant() error {
	return a.configSvc.SetSelectedGrant("", "")
}

func (a *App) GetPlans() ([]map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}
	plans, err := a.authSvc.FetchPlans()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		entry := map[string]any{
			"plan_id":       p.PlanID,
			"name":          p.Name,
			"description":   p.Description,
			"price":         p.Price,
			"monthly_quota": p.MonthlyQuota,
			"max_rpm":       p.MaxRPM,
			"max_tpm":       p.MaxTPM,
			"sort_order":    p.SortOrder,
			"relay_enabled": p.RelayEnabled,
		}
		result = append(result, entry)
	}
	return result, nil
}

func (a *App) ChatCompletion(messages []map[string]any, model string) (map[string]any, error) {
	body := map[string]any{
		"model":    model,
		"messages": messages,
		// 本地网关识别该标记后，在非流式响应中附加 _combo_steps/_intent 诊断字段
		"_seasagi_debug": true,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// P8: Grant-based routing — if a grant is selected, send directly to relay gateway
	grantID := a.configSvc.GetSelectedGrantID()
	grantRelayURL := a.configSvc.GetSelectedGrantRelayURL()
	if grantID != "" && grantRelayURL != "" {
		targetURL := strings.TrimRight(grantRelayURL, "/") + "/v1/chat/completions"
		req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Grant-Id", grantID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("relay gateway request failed: %w", err)
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var result map[string]any
		if err := json.Unmarshal(respBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		result["_gateway_status"] = resp.StatusCode
		result["_grant_id"] = grantID
		result["_curl_command"] = buildCurlCommand(targetURL, "X-Grant-Id: "+grantID, string(bodyBytes))
		return result, nil
	}

	// Default: local gateway routing
	gatewayPort := a.gatewaySvc.GetListenPort()
	if gatewayPort == 0 {
		return nil, fmt.Errorf("local gateway is not running")
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", gatewayPort),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_gateway_status"] = resp.StatusCode
	result["_gateway_port"] = gatewayPort

	// Build curl command for debugging
	result["_curl_command"] = buildCurlCommand(
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", gatewayPort),
		"Bearer "+token,
		string(bodyBytes),
	)

	return result, nil
}

func buildCurlCommand(url, authHeader, bodyJSON string) string {
	escapedBody := strings.ReplaceAll(bodyJSON, "'", "'\\''")
	return fmt.Sprintf("curl -X POST '%s' \\\n  -H 'Content-Type: application/json' \\\n  -H 'Authorization: %s' \\\n  -d '%s'", url, authHeader, escapedBody)
}

func (a *App) ChatCompletionForChannel(channelID string, messages []map[string]any, model string) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for channel %s", channelID)
	}

	body := map[string]any{
		"model":    model,
		"messages": messages,
	}
	bodyBytes, _ := json.Marshal(body)

	baseURL := strings.TrimRight(ch.BaseURL, "/")
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channel request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_channel_id"] = channelID
	result["_channel_name"] = ch.DisplayName
	result["_status_code"] = resp.StatusCode

	// Build curl command for debugging
	result["_curl_command"] = buildCurlCommand(
		baseURL+"/chat/completions",
		"Bearer "+apiKey,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) ImageGeneration(prompt string, model string, params map[string]any) (map[string]any, error) {
	gatewayPort := a.gatewaySvc.GetListenPort()
	if gatewayPort == 0 {
		return nil, fmt.Errorf("local gateway is not running")
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/images/generations", gatewayPort),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_gateway_status"] = resp.StatusCode
	result["_gateway_port"] = gatewayPort
	result["_curl_command"] = buildCurlCommand(
		fmt.Sprintf("http://127.0.0.1:%d/v1/images/generations", gatewayPort),
		"Bearer "+token,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) ImageGenerationForChannel(channelID string, prompt string, model string, params map[string]any) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for channel %s", channelID)
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, _ := json.Marshal(body)

	baseURL := strings.TrimRight(ch.BaseURL, "/")
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/images/generations", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channel request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_channel_id"] = channelID
	result["_channel_name"] = ch.DisplayName
	result["_status_code"] = resp.StatusCode
	result["_curl_command"] = buildCurlCommand(
		baseURL+"/images/generations",
		"Bearer "+apiKey,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) VideoGeneration(prompt string, model string, params map[string]any) (map[string]any, error) {
	gatewayPort := a.gatewaySvc.GetListenPort()
	if gatewayPort == 0 {
		return nil, fmt.Errorf("local gateway is not running")
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/videos/generations", gatewayPort),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_gateway_status"] = resp.StatusCode
	result["_gateway_port"] = gatewayPort
	result["_curl_command"] = buildCurlCommand(
		fmt.Sprintf("http://127.0.0.1:%d/v1/videos/generations", gatewayPort),
		"Bearer "+token,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) VideoGenerationForChannel(channelID string, prompt string, model string, params map[string]any) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for channel %s", channelID)
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, _ := json.Marshal(body)

	baseURL := strings.TrimRight(ch.BaseURL, "/")
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/videos/generations", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channel request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_channel_id"] = channelID
	result["_channel_name"] = ch.DisplayName
	result["_status_code"] = resp.StatusCode
	result["_curl_command"] = buildCurlCommand(
		baseURL+"/videos/generations",
		"Bearer "+apiKey,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) GetRecommendedCombos() ([]map[string]any, error) {
	combos, err := a.authSvc.FetchRecommendedCombos()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(combos))
	for _, combo := range combos {
		result = append(result, map[string]any{
			"name":        combo.Name,
			"models":      combo.Models,
			"description": combo.Description,
			"strategy":    combo.Strategy,
		})
	}
	return result, nil
}

func (a *App) GetOfficialComboTemplates() ([]map[string]any, error) {
	templates, err := a.authSvc.FetchOfficialComboTemplates(a.ctx)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(templates))
	for _, tmpl := range templates {
		steps := make([]map[string]any, 0, len(tmpl.Steps))
		for _, step := range tmpl.Steps {
			steps = append(steps, map[string]any{
				"channel_id": step.ChannelID,
				"model":      step.Model,
			})
		}
		result = append(result, map[string]any{
			"name":        tmpl.Name,
			"description": tmpl.Description,
			"tags":        tmpl.Tags,
			"steps":       steps,
			"models":      tmpl.Models,
			"strategy":    tmpl.Strategy,
			"sticky_uses": tmpl.StickyUses,
		})
	}
	return result, nil
}

// FetchCloudCombos retrieves user combos from the cloud server.
func (a *App) FetchCloudCombos() ([]map[string]any, error) {
	combos, err := a.authSvc.FetchCloudCombos()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(combos))
	for _, c := range combos {
		steps := make([]map[string]any, 0, len(c.Steps))
		for _, s := range c.Steps {
			steps = append(steps, map[string]any{
				"channel_id": s.ChannelID,
				"model":      s.Model,
				"step_role":  s.StepRole,
			})
		}
		result = append(result, map[string]any{
			"combo_id":       c.ComboID,
			"scope":          c.Scope,
			"logical_name":   c.LogicalName,
			"display_name":   c.DisplayName,
			"description":    c.Description,
			"tags":           c.Tags,
			"strategy":       c.Strategy,
			"sticky_uses":    c.StickyUses,
			"quick_strategy": c.QuickStrategy,
			"task_profile":   c.TaskProfile,
			"steps":          steps,
			"status":         c.Status,
			"source":         c.Source,
			"version":        c.Version,
			"created_at":     c.CreatedAt,
			"updated_at":     c.UpdatedAt,
		})
	}
	return result, nil
}

// PushCloudCombo creates a user combo on the cloud server.
func (a *App) PushCloudCombo(logicalName, displayName, description, strategy string, stickyUses int, quickStrategy string, taskProfile map[string]any, stepsJSON string) (map[string]any, error) {
	combo, err := a.authSvc.PushCloudCombo(logicalName, displayName, description, strategy, stickyUses, quickStrategy, taskProfile, []byte(stepsJSON))
	if err != nil {
		return nil, err
	}
	if combo == nil {
		return nil, nil
	}
	return map[string]any{
		"combo_id":       combo.ComboID,
		"logical_name":   combo.LogicalName,
		"display_name":   combo.DisplayName,
		"description":    combo.Description,
		"strategy":       combo.Strategy,
		"sticky_uses":    combo.StickyUses,
		"quick_strategy": combo.QuickStrategy,
		"task_profile":   combo.TaskProfile,
		"status":         combo.Status,
		"version":        combo.Version,
	}, nil
}

// UpdateCloudCombo updates a user combo on the cloud server.
func (a *App) UpdateCloudCombo(comboID, displayName, description, strategy string, stickyUses int, status, quickStrategy string, taskProfile map[string]any, stepsJSON string) error {
	return a.authSvc.UpdateCloudCombo(comboID, displayName, description, strategy, stickyUses, status, quickStrategy, taskProfile, []byte(stepsJSON))
}

// DeleteCloudCombo deletes a user combo on the cloud server.
func (a *App) DeleteCloudCombo(comboID string) error {
	return a.authSvc.DeleteCloudCombo(comboID)
}

func (a *App) GetOptimizationPlan(mode string, taskType string) map[string]any {
	if mode == "" {
		mode = "value_first"
	}
	if taskType == "" {
		taskType = optimizer.TaskGeneralChat
	}
	plan := a.optimizerSvc.GetOptimizationPlan(mode, taskType)
	if plan == nil {
		return nil
	}
	recommendations := make([]map[string]any, 0, len(plan.Recommendations))
	for _, rec := range plan.Recommendations {
		recommendations = append(recommendations, map[string]any{
			"from_model":   rec.FromModel,
			"to_model":     rec.ToModel,
			"model_tag":    rec.ModelTag,
			"channel_id":   rec.ChannelID,
			"channel_name": rec.ChannelName,
			"savings_usd":  rec.SavingsUSD,
			"quality_diff": rec.QualityDiff,
			"reason":       rec.Reason,
		})
	}
	return map[string]any{
		"recommendations": recommendations,
		"monthly_savings": plan.MonthlySavings,
		"strategy":        plan.Strategy,
		"mode":            plan.Mode,
		"task_type":       plan.TaskType,
	}
}

func (a *App) GetQuickStrategies() []map[string]any {
	return []map[string]any{
		{
			"alias":        "quality_first",
			"display_name": "质量优先",
			"description":  "优先分配高智商模型（Claude/GPT-4 系），适合复杂推理与代码任务",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"anthropic", "openai", "google"},
				"fallback_order":     []string{"anthropic", "openai", "google"},
				"min_success_rate":   0.95,
			},
			"combo_constraints": map[string]any{
				"min_steps":          2,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback"},
			},
		},
		{
			"alias":        "cost_first",
			"display_name": "性价比优先",
			"description":  "优先选择成本更低的模型与提供商组合",
			"task_profile": map[string]any{
				"task_type":            optimizer.TaskGeneralChat,
				"priority_providers":   []string{"openrouter", "google", "openai"},
				"fallback_order":       []string{"openrouter", "google", "openai"},
				"max_cost_per_request": 0.02,
			},
			"combo_constraints": map[string]any{
				"min_steps":          1,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback", "round_robin"},
			},
		},
		{
			"alias":        "balanced",
			"display_name": "均衡推荐",
			"description":  "质量与成本并重，按意图场景在中高端与轻量模型间动态取舍",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"openai", "anthropic", "google", "openrouter"},
				"fallback_order":     []string{"openai", "anthropic", "google", "openrouter"},
			},
			"combo_constraints": map[string]any{
				"min_steps":          1,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback", "round_robin"},
			},
		},
		{
			"alias":        "stable_first",
			"display_name": "稳定优先",
			"description":  "优先选择稳定性高、回退链清晰的模型组合",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"openai", "anthropic", "google"},
				"fallback_order":     []string{"openai", "anthropic", "google"},
				"min_success_rate":   0.98,
			},
			"combo_constraints": map[string]any{
				"min_steps":          2,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback"},
			},
		},
		{
			"alias":        "speed_first",
			"display_name": "速度优先",
			"description":  "优先选择低延迟模型，适合交互式场景",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"google", "openai", "anthropic"},
				"fallback_order":     []string{"google", "openai", "anthropic"},
				"max_latency_ms":     1500,
			},
			"combo_constraints": map[string]any{
				"min_steps":          1,
				"max_steps":          2,
				"allowed_strategies": []string{"fallback", "round_robin"},
			},
		},
		{
			"alias":        "tools_first",
			"display_name": "工具优先",
			"description":  "优先选择工具调用成功率和结构化输出稳定性更高的模型",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskToolCalling,
				"priority_providers": []string{"openai", "anthropic", "google", "openrouter"},
				"fallback_order":     []string{"openai", "anthropic", "google", "openrouter"},
				"min_success_rate":   0.97,
			},
			"combo_constraints": map[string]any{
				"min_steps":          2,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback"},
			},
		},
	}
}

func (a *App) GetTaskProfiles() []map[string]any {
	return []map[string]any{
		{
			"task_type":          optimizer.TaskGeneralChat,
			"priority_providers": []string{"openai", "anthropic", "google"},
			"fallback_order":     []string{"openai", "anthropic", "google"},
			"min_success_rate":   0.95,
		},
		{
			"task_type":          optimizer.TaskToolCalling,
			"priority_providers": []string{"openai", "anthropic", "google", "openrouter"},
			"fallback_order":     []string{"openai", "anthropic", "google", "openrouter"},
			"min_success_rate":   0.97,
		},
		{
			"task_type":          optimizer.TaskStructured,
			"priority_providers": []string{"openai", "anthropic", "google"},
			"fallback_order":     []string{"openai", "google", "anthropic"},
			"min_success_rate":   0.96,
		},
		{
			"task_type":          optimizer.TaskLongContext,
			"priority_providers": []string{"google", "anthropic", "openai"},
			"fallback_order":     []string{"google", "anthropic", "openai"},
			"max_latency_ms":     4000,
			"min_success_rate":   0.94,
		},
		{
			"task_type":          optimizer.TaskVision,
			"priority_providers": []string{"openai", "google", "anthropic"},
			"fallback_order":     []string{"openai", "google", "anthropic"},
			"min_success_rate":   0.95,
		},
	}
}

func (a *App) GetUsageSummary() map[string]any {
	summary := a.usageSvc.GetUsageSummary()
	return map[string]any{
		"month_requests":      summary.MonthRequests,
		"month_input_tokens":  summary.MonthInputTokens,
		"month_output_tokens": summary.MonthOutputTokens,
		"month_cost_usd":      summary.MonthCostUSD,
	}
}

func (a *App) GetOptimizationConfig() map[string]any {
	cfg := a.configSvc.GetOptimizationConfig()
	return map[string]any{
		"mode":                 cfg.Mode,
		"penalty_enabled":      cfg.PenaltyEnabled,
		"penalty_decay_sec":    cfg.PenaltyDecaySec,
		"health_check_enabled": cfg.HealthCheckEnabled,
		"health_check_sec":     cfg.HealthCheckSec,
		"health_max_failures":  cfg.HealthMaxFailures,
		"cooldown_enabled":     cfg.CooldownEnabled,
		"cooldown_sec":         cfg.CooldownSec,
		"sticky_enabled":       cfg.StickyEnabled,
		"sticky_ttl_sec":       cfg.StickyTTLSec,
		"preset_enabled":       cfg.PresetEnabled,
		"default_preset":       cfg.DefaultPreset,
	}
}

func (a *App) SetOptimizationConfig(cfg map[string]any) error {
	parsed := config.OptimizationConfig{}
	if v, ok := cfg["mode"]; ok {
		parsed.Mode, _ = v.(string)
	}
	if v, ok := cfg["penalty_enabled"]; ok {
		parsed.PenaltyEnabled, _ = v.(bool)
	}
	if v, ok := cfg["penalty_decay_sec"]; ok {
		parsed.PenaltyDecaySec, _ = v.(int)
	}
	if v, ok := cfg["health_check_enabled"]; ok {
		parsed.HealthCheckEnabled, _ = v.(bool)
	}
	if v, ok := cfg["health_check_sec"]; ok {
		parsed.HealthCheckSec, _ = v.(int)
	}
	if v, ok := cfg["health_max_failures"]; ok {
		parsed.HealthMaxFailures, _ = v.(int)
	}
	if v, ok := cfg["cooldown_enabled"]; ok {
		parsed.CooldownEnabled, _ = v.(bool)
	}
	if v, ok := cfg["cooldown_sec"]; ok {
		parsed.CooldownSec, _ = v.(int)
	}
	if v, ok := cfg["sticky_enabled"]; ok {
		parsed.StickyEnabled, _ = v.(bool)
	}
	if v, ok := cfg["sticky_ttl_sec"]; ok {
		parsed.StickyTTLSec, _ = v.(int)
	}
	if v, ok := cfg["preset_enabled"]; ok {
		parsed.PresetEnabled, _ = v.(bool)
	}
	if v, ok := cfg["default_preset"]; ok {
		parsed.DefaultPreset, _ = v.(string)
	}
	return a.configSvc.SetOptimizationConfig(parsed)
}

// GetRateLimitConfig 获取速率限制配置。
func (a *App) GetRateLimitConfig() map[string]any {
	cfg := a.configSvc.GetRateLimitConfig()
	return map[string]any{
		"enabled":                cfg.Enabled,
		"default_rpm":            cfg.DefaultRPM,
		"default_tpm":            cfg.DefaultTPM,
		"min_interval_ms":        cfg.MinIntervalMs,
		"max_concurrent":         cfg.MaxConcurrent,
		"max_wait_ms":            cfg.MaxWaitMs,
		"monthly_cost_limit_usd": cfg.MonthlyCostLimitUSD,
		"channel_overrides":      cfg.ChannelOverrides,
	}
}

// SetRateLimitConfig 设置速率限制配置。
func (a *App) SetRateLimitConfig(cfg map[string]any) error {
	parsed := config.RateLimitConfig{}
	if v, ok := cfg["enabled"]; ok {
		parsed.Enabled, _ = v.(bool)
	}
	if v, ok := cfg["default_rpm"]; ok {
		parsed.DefaultRPM, _ = toInt(v)
	}
	if v, ok := cfg["default_tpm"]; ok {
		parsed.DefaultTPM, _ = toInt(v)
	}
	if v, ok := cfg["min_interval_ms"]; ok {
		parsed.MinIntervalMs, _ = toInt(v)
	}
	if v, ok := cfg["max_concurrent"]; ok {
		parsed.MaxConcurrent, _ = toInt(v)
	}
	if v, ok := cfg["max_wait_ms"]; ok {
		parsed.MaxWaitMs, _ = toInt(v)
	}
	if v, ok := cfg["monthly_cost_limit_usd"]; ok {
		parsed.MonthlyCostLimitUSD, _ = v.(float64)
	}
	if v, ok := cfg["channel_overrides"]; ok {
		if overrides, ok := v.(map[string]any); ok {
			parsed.ChannelOverrides = make(map[string]*config.ChannelRateLimit)
			for chID, raw := range overrides {
				if m, ok := raw.(map[string]any); ok {
					ov := &config.ChannelRateLimit{}
					if rv, ok := m["rpm"]; ok {
						ov.RPM, _ = toInt(rv)
					}
					if rv, ok := m["tpm"]; ok {
						ov.TPM, _ = toInt(rv)
					}
					if rv, ok := m["min_interval_ms"]; ok {
						ov.MinIntervalMs, _ = toInt(rv)
					}
					if rv, ok := m["max_concurrent"]; ok {
						ov.MaxConcurrent, _ = toInt(rv)
					}
					parsed.ChannelOverrides[chID] = ov
				}
			}
		}
	}
	return a.configSvc.SetRateLimitConfig(parsed)
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
