import { useState, useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import { getCloudUsage, getCloudBilling, getUsageSummary, getPlatformAPIBaseURL, getPlatformToken } from "../utils/commands";
import type { CloudUsage, CloudBilling, UsageSummary, OverageRecord } from "../utils/types";
import { useTranslation } from "../i18n";

export function UsagePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const setCloudBilling = useAppStore((s) => s.setCloudBilling);
  const setCloudUsageStore = useAppStore((s) => s.setCloudUsage);
  const [cloudUsage, setCloudUsage] = useState<CloudUsage | null>(null);
  const [billing, setBilling] = useState<CloudBilling | null>(null);
  const [localSummary, setLocalSummary] = useState<UsageSummary | null>(null);
  const [overage, setOverage] = useState<OverageRecord | null>(null);
  const [loading, setLoading] = useState(true);
  const [showOverageInfo, setShowOverageInfo] = useState(false);
  const [dismissedSuggestions, setDismissedSuggestions] = useState<string[]>([]);

  useEffect(() => {
    (async () => {
      try {
        const [cu, cb, ls] = await Promise.all([
          getCloudUsage().catch(() => null),
          getCloudBilling().catch(() => null),
          getUsageSummary().catch(() => null),
        ]);
        setCloudUsage(cu);
        setBilling(cb);
        setCloudUsageStore(cu);
        if (cb) setCloudBilling(cb);
        setLocalSummary(ls);

        try {
          const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
          const resp = await fetch(`${baseURL}/usage/overage/user`, {
            headers: token ? { Authorization: `Bearer ${token}` } : undefined,
          });
          if (resp.ok) {
            const data: OverageRecord = await resp.json();
            setOverage(data);
          }
        } catch {
          // overage data is optional
        }
      } catch {
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const quotaUsed = billing ? billing.used_quota : 0;
  const quotaTotal = billing ? billing.quota : 0;
  const quotaPercent = quotaTotal > 0 ? Math.min(100, (quotaUsed / quotaTotal) * 100) : 0;
  const localOverage = billing && billing.price > 0 ? Math.max(0, quotaUsed - quotaTotal) : 0;
  const hasOverage = localOverage > 0 || (overage && overage.overage_requests > 0);
  const overageCost = overage ? overage.overage_cost : 0;

  const suggestions = useMemo(() => {
    const list: { id: string; type: "quota" | "collab" | "govern"; title: string; desc: string; action: string; severity: "info" | "warning" | "danger" }[] = [];
    const planId = billing?.plan_id || "free";
    const pct = quotaPercent;
    const hasHighOverage = hasOverage && (localOverage > 100 || (overage?.overage_requests || 0) > 100);

    if (planId === "free") {
      if (pct > 60 || hasHighOverage) {
        list.push({
          id: "free-pro-quota",
          type: "quota",
          title: t("usage.suggestion.freeProQuota.title"),
          desc: t("usage.suggestion.freeProQuota.desc", { pct: pct.toFixed(0) }),
          action: t("usage.suggestion.freeProQuota.action"),
          severity: pct > 85 ? "danger" : "warning",
        });
      }
      if (pct > 30) {
        list.push({
          id: "free-collab",
          type: "collab",
          title: t("usage.suggestion.freeCollab.title"),
          desc: t("usage.suggestion.freeCollab.desc"),
          action: t("usage.suggestion.freeCollab.action"),
          severity: "info",
        });
      }
    } else if (planId === "pro") {
      if (pct > 70 || hasHighOverage) {
        list.push({
          id: "pro-teams-quota",
          type: "quota",
          title: t("usage.suggestion.proTeamsQuota.title"),
          desc: t("usage.suggestion.proTeamsQuota.desc", { pct: pct.toFixed(0) }),
          action: t("usage.suggestion.proTeamsQuota.action"),
          severity: pct > 85 ? "danger" : "warning",
        });
      }
      if (pct > 40) {
        list.push({
          id: "pro-teams-collab",
          type: "collab",
          title: t("usage.suggestion.proTeamsCollab.title"),
          desc: t("usage.suggestion.proTeamsCollab.desc"),
          action: t("usage.suggestion.proTeamsCollab.action"),
          severity: "info",
        });
      }
    } else if (planId === "teams") {
      if (pct > 80) {
        list.push({
          id: "teams-enterprise-govern",
          type: "govern",
          title: t("usage.suggestion.teamsEnterprise.title"),
          desc: t("usage.suggestion.teamsEnterprise.desc", { pct: pct.toFixed(0) }),
          action: t("usage.suggestion.teamsEnterprise.action"),
          severity: pct > 90 ? "danger" : "warning",
        });
      }
    }

    return list.filter((s) => !dismissedSuggestions.includes(s.id));
  }, [billing, quotaPercent, hasOverage, localOverage, overage, dismissedSuggestions]);

  const dismissSuggestion = (id: string) => {
    setDismissedSuggestions((prev) => [...prev, id]);
  };

  if (loading) return <div className="loading">{t("usage.loading")}</div>;

  return (
    <div className="page usage-page">
      <div className="page-hero">
        <div className="page-hero-head">
          <div className="page-hero-copy">
            <h1>{t("usage.title")}</h1>
            <div className="page-hero-content-row">
              <p className="page-subtitle">{t("usage.subtitle")}</p>
              <div className="hero-metrics">
                <div className="hero-metric-card">
                  <span className="hero-metric-label">{t("usage.localRequests")}</span>
                  <strong className="hero-metric-value">{(localSummary?.month_requests ?? 0).toLocaleString()}</strong>
                </div>
                <div className="hero-metric-card">
                  <span className="hero-metric-label">{t("usage.cloudCost")}</span>
                  <strong className="hero-metric-value">{cloudUsage ? `$${cloudUsage.total_cost_usd.toFixed(2)}` : "--"}</strong>
                </div>
                {billing && (
                  <div className="hero-metric-card">
                    <span className="hero-metric-label">{billing.plan_name}</span>
                    <strong className="hero-metric-value">{billing.price > 0 ? `$${billing.price}/mo` : t("subscription.free")}</strong>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>

      {billing && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("usage.subscriptionQuota")}</h2>
            <p className="hint">{t("usage.subscriptionQuotaHint", { plan: billing.plan_name })}</p>
          </div>
          <div className="usage-quota-card">
            <div className="usage-quota-header">
              <span className="usage-quota-label">
                {t("usage.quotaUsed", { used: quotaUsed.toLocaleString(), total: quotaTotal.toLocaleString() })}
                {billing.relay_enabled && <span className="badge badge-blue" style={{ marginLeft: 8 }}>{t("usage.relayAvailable")}</span>}
              </span>
              <span className="usage-quota-pct">{quotaPercent.toFixed(0)}%</span>
            </div>
            <div className="progress-bar">
              <div
                className={`progress-fill ${quotaPercent > 85 ? "fill-danger" : quotaPercent > 60 ? "fill-warning" : ""}`}
                style={{ width: `${quotaPercent}%` }}
              />
              {hasOverage && (
                <div
                  className="progress-fill fill-overage"
                  style={{
                    width: `${Math.min(100, ((localOverage || overage?.overage_requests || 0) / quotaTotal) * 100)}%`,
                    left: `${Math.min(100, quotaPercent)}%`,
                  }}
                />
              )}
            </div>
            {hasOverage && (
              <div className="usage-overage-hint">
                {t("usage.overageRequests")}: <strong>{(localOverage || overage?.overage_requests || 0).toLocaleString()}</strong>
                {overageCost > 0 && (
                  <span style={{ marginLeft: 12 }}>
                    {t("usage.estimatedOverageCost")}: <strong>${overageCost.toFixed(4)}</strong>
                  </span>
                )}
                <button className="link-btn" onClick={() => setShowOverageInfo(!showOverageInfo)} style={{ marginLeft: 8 }}>
                  {showOverageInfo ? t("usage.collapse") : t("usage.learnOverage")}
                </button>
                {showOverageInfo && (
                  <div className="usage-overage-detail" style={{ marginTop: 8, fontSize: "0.85rem", color: "var(--text-secondary)" }}>
                    {t("usage.overageDetail")}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {billing && billing.relay_enabled && billing.relay_gateways && billing.relay_gateways.length > 0 && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("usage.relayChannels")}</h2>
            <p className="hint">{t("usage.relayChannelsHint")}</p>
          </div>
          <div className="relay-gw-list">
            {billing.relay_gateways.map((gw) => (
              <div key={gw.gateway_id} className="relay-gw-item">
                <span className="relay-gw-name">{gw.name}</span>
                <span className="relay-gw-region">{gw.region}</span>
                <span className="relay-gw-host">{gw.host}:{gw.port}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {localSummary && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("usage.localUsage")}</h2>
            <p className="hint">{t("usage.localUsageHint")}</p>
          </div>
          <div className="stats-grid">
            <div className="stat-card">
              <div className="stat-label">{t("usage.monthlyRequests")}</div>
              <div className="stat-value blue">{localSummary.month_requests.toLocaleString()}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("usage.inputTokens")}</div>
              <div className="stat-value">{localSummary.month_input_tokens.toLocaleString()}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("usage.outputTokens")}</div>
              <div className="stat-value">{localSummary.month_output_tokens.toLocaleString()}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("usage.monthlyCost")}</div>
              <div className="stat-value green">${localSummary.month_cost_usd.toFixed(2)}</div>
            </div>
          </div>
          <div className="usage-cost-hint">
            {t("usage.localCostHint")}
          </div>
          <div className="attribution-hint">
            <strong>{t("usage.requestAttribution")}:</strong>
            <span style={{ color: "var(--green)" }}>■ {t("usage.localByok")}</span> — {t("usage.localByokDesc")} &nbsp;
            <span style={{ color: "var(--accent)" }}>■ {t("usage.platform")}</span> — {t("usage.platformDesc")} &nbsp;
            <span style={{ color: "var(--orange)" }}>■ {t("usage.relay")}</span> — {t("usage.relayDesc")}
          </div>
        </div>
      )}

      {cloudUsage && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("usage.cloudUsage")}</h2>
            <p className="hint">{t("usage.cloudUsageHint")}</p>
          </div>
          <div className="stats-grid">
            <div className="stat-card">
              <div className="stat-label">{t("usage.monthlyRequests")}</div>
              <div className="stat-value blue">{cloudUsage.month_requests.toLocaleString()}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("usage.inputTokens")}</div>
              <div className="stat-value">{cloudUsage.month_input_tokens?.toLocaleString() || t("usage.na")}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("usage.outputTokens")}</div>
              <div className="stat-value">{cloudUsage.month_output_tokens?.toLocaleString() || t("usage.na")}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("usage.totalCost")}</div>
              <div className="stat-value green">${cloudUsage.total_cost_usd.toFixed(2)}</div>
            </div>
          </div>
          {overage && overage.overage_requests > 0 && (
            <div className="usage-cost-hint" style={{ marginTop: 12, borderTop: "1px solid var(--border)", paddingTop: 12 }}>
              {t("usage.overageSummary", { requests: overage.overage_requests.toLocaleString(), cost: overage.overage_cost.toFixed(4) })}
            </div>
          )}
        </div>
      )}

      {suggestions.length > 0 && (
        <div className="section">
          <div className="section-heading">
            <h2>{t("usage.upgradeSuggestions")}</h2>
            <p className="hint">{t("usage.upgradeSuggestionsHint")}</p>
          </div>
          <div className="upgrade-suggestions">
            {suggestions.map((s) => (
              <div key={s.id} className={`upgrade-card upgrade-${s.severity}`}>
                <div className="upgrade-card-body">
                  <div className="upgrade-card-header">
                    <span className={`upgrade-icon ${s.type}`}>
                      {s.type === "quota" ? "📊" : s.type === "collab" ? "👥" : "🏢"}
                    </span>
                    <div>
                      <div className="upgrade-card-title">{s.title}</div>
                      <div className="upgrade-card-desc">{s.desc}</div>
                    </div>
                  </div>
                  <div className="upgrade-card-actions">
                    <button className="btn-primary btn-sm" onClick={() => navigate("/subscription")}>
                      {s.action}
                    </button>
                    <button className="btn-ghost btn-sm" onClick={() => dismissSuggestion(s.id)}>
                      {t("usage.dismiss")}
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {billing && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("usage.routingStrategy")}</h2>
            <p className="hint">{t("usage.routingStrategyHint")}</p>
          </div>
          <div className="billing-card">
            <div className="billing-row">
              <span className="billing-label">{t("usage.recommendedStrategy")}</span>
              <span className="billing-value">{billing.price > 0 ? t("usage.balancedMode") : t("usage.byokPriority")}</span>
            </div>
            <div className="billing-row">
              <span className="billing-label">{t("usage.highCostModel")}</span>
              <span className="billing-value">
                {billing.plan_id === "free" || billing.plan_id === "pro"
                  ? t("usage.overageSuggestion")
                  : t("usage.includedInPlan")}
              </span>
            </div>
            <div className="billing-row">
              <span className="billing-label">{t("usage.upgradeSuggestion")}</span>
              <span className="billing-value">
                {billing.plan_id === "free"
                  ? t("usage.freeUpgradeSuggestion")
                  : billing.plan_id === "pro"
                  ? t("usage.proUpgradeSuggestion")
                  : t("usage.enterpriseUpgradeSuggestion")}
              </span>
            </div>
          </div>
        </div>
      )}

      {!cloudUsage && !localSummary && (
        <div className="empty-state">{t("usage.noData")}</div>
      )}
    </div>
  );
}
