package mitm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
)

// P2-12：企业端下发的 MITM 路由规则（只读、置底）。
// 与 FetchTargetsFromEnterprise 同风格：GET {ENTERPRISE_API_BASE_URL}/mitm/routing-rules，
// 离线时回退本地缓存。企业规则存于内存，不写入本地配置。

const mitmRoutingCacheFile = ".mitm-routing-rules-cache.json"

var (
	enterpriseRulesMu sync.RWMutex
	enterpriseRules   []config.MITMRoutingRule
)

// FetchRoutingRulesFromEnterprise 从企业服务端拉取默认 MITM 路由规则并缓存到内存。
func FetchRoutingRulesFromEnterprise(ctx context.Context, bearerToken string) error {
	enterpriseURL := os.Getenv("ENTERPRISE_API_BASE_URL")
	if enterpriseURL == "" {
		return nil // 未配置企业服务端
	}

	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, enterpriseURL+"/mitm/routing-rules", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if cached := loadCachedRoutingRules(); cached != nil {
			setEnterpriseRules(cached)
			return nil
		}
		return fmt.Errorf("fetch routing rules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		if cached := loadCachedRoutingRules(); cached != nil {
			setEnterpriseRules(cached)
			return nil
		}
		return fmt.Errorf("enterprise API returned %d", resp.StatusCode)
	}

	var result struct {
		Data []config.MITMRoutingRule `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// 统一标记来源为企业，便于 UI 只读展示与 resolver 置底排序。
	for i := range result.Data {
		result.Data[i].Source = config.MITMRuleSourceEnterprise
	}
	saveCachedRoutingRules(result.Data)
	setEnterpriseRules(result.Data)
	return nil
}

// enterpriseRulesRefreshInterval 企业规则定期刷新间隔（P4-24）。
const enterpriseRulesRefreshInterval = 30 * time.Minute

// StartEnterpriseRulesRefresher 启动后台企业规则同步（P4-24）：
// 启动时立即拉取一次，之后每 enterpriseRulesRefreshInterval 刷新；
// tokenFn 提供当前平台令牌（未登录时返回空串）。未配置 ENTERPRISE_API_BASE_URL 时静默跳过。
func StartEnterpriseRulesRefresher(ctx context.Context, tokenFn func() string) {
	if os.Getenv("ENTERPRISE_API_BASE_URL") == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	refresh := func() {
		token := ""
		if tokenFn != nil {
			token = tokenFn()
		}
		if err := FetchRoutingRulesFromEnterprise(ctx, token); err != nil {
			logging.Debugf("enterprise mitm routing rules refresh failed: %v", err)
		}
	}
	go refresh()
	go func() {
		ticker := time.NewTicker(enterpriseRulesRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
}

// EnterpriseRoutingRules 返回当前内存中的企业下发规则（副本）。
func EnterpriseRoutingRules() []config.MITMRoutingRule {
	enterpriseRulesMu.RLock()
	defer enterpriseRulesMu.RUnlock()
	if len(enterpriseRules) == 0 {
		return nil
	}
	out := make([]config.MITMRoutingRule, len(enterpriseRules))
	copy(out, enterpriseRules)
	return out
}

func setEnterpriseRules(rules []config.MITMRoutingRule) {
	enterpriseRulesMu.Lock()
	enterpriseRules = rules
	enterpriseRulesMu.Unlock()
}

func getRoutingCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return mitmRoutingCacheFile
	}
	return filepath.Join(home, ".seasagi", mitmRoutingCacheFile)
}

func loadCachedRoutingRules() []config.MITMRoutingRule {
	data, err := os.ReadFile(getRoutingCachePath())
	if err != nil {
		return nil
	}
	var result struct {
		Data []config.MITMRoutingRule `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	for i := range result.Data {
		result.Data[i].Source = config.MITMRuleSourceEnterprise
	}
	return result.Data
}

func saveCachedRoutingRules(rules []config.MITMRoutingRule) {
	data, err := json.Marshal(struct {
		Data []config.MITMRoutingRule `json:"data"`
	}{Data: rules})
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(getRoutingCachePath()), 0700)
	_ = os.WriteFile(getRoutingCachePath(), data, 0600)
}
