package main

import (
	"fmt"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

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

func (a *App) GetUsageSummary() map[string]any {
	summary := a.usageSvc.GetUsageSummary()
	return map[string]any{
		"month_requests":      summary.MonthRequests,
		"month_input_tokens":  summary.MonthInputTokens,
		"month_output_tokens": summary.MonthOutputTokens,
		"month_cost_usd":      summary.MonthCostUSD,
	}
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

// GetKeyQuotaStatus 返回本地访问令牌的月度配额状态。
// 配额状态持久化在 ~/.seasagi/key_quotas.json，重启后已用量不会归零。
func (a *App) GetKeyQuotaStatus() map[string]any {
	limit, used, configured := a.gatewaySvc.KeyQuotaStatus()
	return map[string]any{
		"configured":        configured,
		"monthly_limit_usd": limit,
		"used_usd":          used,
	}
}

// SetKeyQuota 设置本地访问令牌的月度美元配额（0 表示不限）。
func (a *App) SetKeyQuota(monthlyLimitUSD float64) error {
	if monthlyLimitUSD < 0 {
		return fmt.Errorf("monthly_limit_usd must be >= 0")
	}
	a.gatewaySvc.SetKeyQuota(monthlyLimitUSD)
	return nil
}

// RemoveKeyQuota 删除本地访问令牌的配额限制。
func (a *App) RemoveKeyQuota() error {
	a.gatewaySvc.RemoveKeyQuota()
	return nil
}
