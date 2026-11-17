package main

import (
	"context"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/buildinfo"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/configio"
	"github.com/SeasAGI/SeasAGI-Client/internal/deeplink"
	"github.com/SeasAGI/SeasAGI-Client/internal/discovery"
	"github.com/SeasAGI/SeasAGI-Client/internal/gateway"
	"github.com/SeasAGI/SeasAGI-Client/internal/localtoken"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/mcp"
	"github.com/SeasAGI/SeasAGI-Client/internal/mitm"
	"github.com/SeasAGI/SeasAGI-Client/internal/optimizer"
	"github.com/SeasAGI/SeasAGI-Client/internal/perf"
	"github.com/SeasAGI/SeasAGI-Client/internal/presets"
	"github.com/SeasAGI/SeasAGI-Client/internal/prompts"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
	"github.com/SeasAGI/SeasAGI-Client/internal/rtk"
	"github.com/SeasAGI/SeasAGI-Client/internal/sessions"
	"github.com/SeasAGI/SeasAGI-Client/internal/skills"
	"github.com/SeasAGI/SeasAGI-Client/internal/sync"
	"github.com/SeasAGI/SeasAGI-Client/internal/tunnel"
	"github.com/SeasAGI/SeasAGI-Client/internal/urlguard"
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
	a.restoreMarketNode()
}

func (a *App) GetAppInfo() map[string]any {
	return map[string]any{
		"name":    "SeasAGI",
		"version": buildinfo.Version,
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

func (a *App) SetPlatformAPIBaseURL(url string) error {
	if url == "" {
		return fmt.Errorf("platform API base URL cannot be empty")
	}
	// 平台令牌会随每个请求发出，必须限制为 HTTPS（仅回环允许 http）且不得指向链路本地（CL-S19）。
	if err := urlguard.ValidateSecureURL(url, true); err != nil {
		return err
	}
	if u, err := neturl.Parse(url); err != nil {
		return fmt.Errorf("invalid platform API base URL: %w", err)
	} else if err := urlguard.RejectLinkLocalHost(u.Host); err != nil {
		return err
	}
	if err := a.configSvc.SetPlatformAPIBaseURL(url); err != nil {
		return err
	}
	auth.SetPlatformAPIBaseURL(url)
	return nil
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

func (a *App) UpdateRoutingSettings(strategy string, stickyUses int) error {
	return a.configSvc.UpdateRoutingSettings(strategy, stickyUses)
}
