package gateway

import (
	"net/http"
	"sort"
	"strings"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/mitm"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
)

// mitmDecision 描述一次 MITM 路由决策的结果。
type mitmDecision struct {
	// Intercepted 表示请求带 MITM 标记（用于日志 [MITM] 标注，即使未改写）。
	Intercepted bool
	// Matched 表示命中了「改写」动作（combo/model），用于 Grant 优先级判定。
	Matched bool
	// Target 来源工具 ID。
	Target string
	// RuleID 命中的规则 ID（默认动作命中时为空）。
	RuleID string
	// RequestedModel 工具发出的原始模型名。
	RequestedModel string
	// ResolvedTarget 改写后的目标（Combo 名或逻辑模型名）；未改写时为空。
	ResolvedTarget string
	// GrantID 命中 market_asset 时指定的订阅授权 ID。
	GrantID string
	// Constraints 随映射注入的请求级约束。
	Constraints map[string]any
	// HeaderMismatch 表示 X-SeasAGI-MITM-Target 与 Original-Host 反查结果不一致（疑似伪造）。
	HeaderMismatch bool
}

// effectiveMITMRouting 将本地规则与企业下发规则合并（企业规则置底，见 sortedMITMRules）。
func effectiveMITMRouting(cfg config.MITMRoutingConfig) config.MITMRoutingConfig {
	ent := mitm.EnterpriseRoutingRules()
	if len(ent) == 0 {
		return cfg
	}
	merged := cfg
	merged.Rules = append(append([]config.MITMRoutingRule{}, cfg.Rules...), ent...)
	return merged
}

// resolveMITMRoute 根据来源标记头与配置决定 MITM 请求的路由改写。
// 仅当请求带 X-SeasAGI-MITM 标记且配置启用时生效；否则返回零值（不干预）。
func resolveMITMRoute(header http.Header, cfg config.MITMRoutingConfig, requestedModel string) mitmDecision {
	if header.Get(config.HeaderMITMSource) != "1" || !cfg.Enabled {
		return mitmDecision{}
	}

	decision := mitmDecision{
		Intercepted:    true,
		Target:         header.Get(config.HeaderMITMTarget),
		RequestedModel: requestedModel,
	}

	// 头部一致性校验：用 Original-Host 反查工具 ID，与声明的 Target 比对（Q5/P2-13）。
	if host := header.Get(config.HeaderMITMHost); host != "" && decision.Target != "" {
		if td := mitm.ResolveTarget(host); td != nil && !strings.EqualFold(td.ID, decision.Target) {
			decision.HeaderMismatch = true
		}
	}

	target := decision.Target
	path := header.Get(config.HeaderMITMEndpoint)

	for _, rule := range sortedMITMRules(cfg.Rules) {
		if !mitmRuleMatches(rule, target, path, requestedModel) {
			continue
		}
		applyMITMAction(&decision, rule.Action, rule.ComboName, rule.Model, rule.GrantID, rule.Constraints, rule.ID)
		return decision
	}

	applyMITMAction(&decision, cfg.DefaultAction, cfg.DefaultCombo, cfg.DefaultModel, "", nil, "")
	return decision
}

// applyMITMAction 将动作结果写入决策。combo/model/market_asset 视为「命中改写」。
func applyMITMAction(decision *mitmDecision, action, comboName, model, grantID string, constraints map[string]any, ruleID string) {
	decision.RuleID = ruleID
	switch action {
	case config.MITMActionCombo:
		decision.Matched = true
		decision.ResolvedTarget = comboName
		decision.Constraints = constraints
	case config.MITMActionModel:
		decision.Matched = true
		decision.ResolvedTarget = model
		decision.Constraints = constraints
	case config.MITMActionMarketAsset:
		// 直选订阅资产：不改写模型名，转发时以 grant 为准。
		decision.Matched = true
		decision.GrantID = grantID
		decision.Constraints = constraints
	default: // passthrough
		decision.Matched = false
	}
}

// sortedMITMRules 返回按「本地规则优先、企业规则置底」再按 priority 升序（小者优先）排列的
// 启用规则；同级保持配置顺序。
func sortedMITMRules(rules []config.MITMRoutingRule) []config.MITMRoutingRule {
	enabled := make([]config.MITMRoutingRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			enabled = append(enabled, rule)
		}
	}
	isEnterprise := func(r config.MITMRoutingRule) bool {
		return r.Source == config.MITMRuleSourceEnterprise
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		ei, ej := isEnterprise(enabled[i]), isEnterprise(enabled[j])
		if ei != ej {
			return !ei // 本地（非企业）优先，企业置底
		}
		return enabled[i].Priority < enabled[j].Priority
	})
	return enabled
}

// marketFirstForCombo 返回该 Combo 是否启用「市场优先」（P3-21）：
// Combo 缺失或未显式关闭（MarketPriority == nil）时默认启用，兼容既有行为。
func marketFirstForCombo(combo config.ModelCombo, ok bool) bool {
	if !ok || combo.MarketPriority == nil {
		return true
	}
	return *combo.MarketPriority
}

// mitmRuleMatches 判断规则是否命中（三个维度全部满足才算命中）。
func mitmRuleMatches(rule config.MITMRoutingRule, target, path, model string) bool {
	if rule.TargetID != "" && rule.TargetID != "*" && !strings.EqualFold(rule.TargetID, target) {
		return false
	}
	if rule.EndpointPath != "" && !strings.HasPrefix(path, rule.EndpointPath) {
		return false
	}
	if rule.RequestModel != "" && !matchMITMModel(rule.RequestModel, model) {
		return false
	}
	return true
}

// matchMITMModel 匹配请求模型：以 `*` 结尾时按前缀匹配，否则等值匹配。
func matchMITMModel(pattern, model string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == model
}

// splitMarketGrantSteps 从路由计划中分离出 Token 市场 Grant 虚拟渠道步骤与其余步骤。
func splitMarketGrantSteps(plan []routing.PlanStep) (grantSteps, rest []routing.PlanStep) {
	for _, step := range plan {
		if step.Channel.Source == "market_grant" {
			grantSteps = append(grantSteps, step)
			continue
		}
		rest = append(rest, step)
	}
	return grantSteps, rest
}

// grantIDFromChannelID 从虚拟渠道 ID（grant://{id}）中提取 Grant ID。
func grantIDFromChannelID(channelID string) string {
	return strings.TrimPrefix(channelID, config.MarketGrantChannelPrefix)
}
