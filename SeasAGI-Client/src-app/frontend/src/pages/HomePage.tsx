import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import { useTeamStore } from "../stores/teamStore";
import * as cmd from "../utils/commands";
import { useTranslation } from "../i18n";
import { formatPlanTier, normalizePlanTier } from "../utils/plan";

/* --- Team Cost Snapshot (local store) --- */
function TeamCostSnapshot() {
  const { t } = useTranslation();
  const store = useTeamStore();
  const costAttribution = store.getCostAttribution();
  const members = store.members;
  if (!costAttribution || costAttribution.members.length === 0) return null;
  return (
    <div className="section section-card">
      <div className="section-heading">
        <h2>{t("home.teamCostTitle")}</h2>
        <p className="hint">{costAttribution.month} · {t("home.teamCostHint")}</p>
      </div>
      <div className="hero-metrics" style={{ marginTop: 8 }}>
        <div className="hero-metric-card">
          <span className="hero-metric-label">{t("home.teamSize")}</span>
          <strong className="hero-metric-value">{members.filter((m: { status: string }) => m.status === "active").length}{t("home.teamMembers")}</strong>
        </div>
        <div className="hero-metric-card">
          <span className="hero-metric-label">{t("home.monthlyEstimatedCost")}</span>
          <strong className="hero-metric-value">${(costAttribution.total_cost || 0).toFixed(2)}</strong>
        </div>
        <div className="hero-metric-card">
          <span className="hero-metric-label">{t("home.costVsLastMonth")}</span>
          <strong className="hero-metric-value" style={{ color: costAttribution.total_cost > costAttribution.previous_cost ? "var(--red)" : "var(--green)" }}>
            {costAttribution.total_cost > costAttribution.previous_cost ? "↑" : "↓"} ${Math.abs(costAttribution.total_cost - costAttribution.previous_cost).toFixed(2)}
          </strong>
        </div>
      </div>
    </div>
  );
}

export function HomePage() {
  const navigate = useNavigate();
  const auth = useAppStore((s) => s.auth);
  const runtime = useAppStore((s) => s.runtime);
  const channels = useAppStore((s) => s.channels);
  const combos = useAppStore((s) => s.combos);
  const appConfig = useAppStore((s) => s.appConfig);
  const cloudBilling = useAppStore((s) => s.cloudBilling);
  const cloudUsage = useAppStore((s) => s.cloudUsage);
  const defaultComboName = useAppStore((s) => s.defaultComboName);
  const setRuntime = useAppStore((s) => s.setRuntime);
  const setAppConfig = useAppStore((s) => s.setAppConfig);
  const setCloudBilling = useAppStore((s) => s.setCloudBilling);
  const setCloudUsage = useAppStore((s) => s.setCloudUsage);
  const setDefaultComboName = useAppStore((s) => s.setDefaultComboName);
  const activeGrants = useAppStore((s) => s.activeGrants);
  const selectedGrant = useAppStore((s) => s.selectedGrant);
  const setActiveGrants = useAppStore((s) => s.setActiveGrants);
  const setSelectedGrant = useAppStore((s) => s.setSelectedGrant);
  const { t } = useTranslation();

  const [loadingCloud, setLoadingCloud] = useState(false);
  const [cloudError, setCloudError] = useState("");
  const baseUrl = `http://127.0.0.1:${runtime?.listen_port || 4318}/v1`;

  const isRunning = runtime?.gateway_running ?? false;

  useEffect(() => {
    if (auth.is_logged_in) {
      setLoadingCloud(true);
      setCloudError("");
      Promise.all([
        cmd.getCloudUsage().catch(() => null),
        cmd.getCloudBilling().catch(() => null),
      ])
        .then(([usage, billing]) => {
          setCloudUsage(usage);
          setCloudBilling(billing);
        })
        .catch(() => setCloudError(t("home.cloudLoadFailed")))
        .finally(() => setLoadingCloud(false));
    } else {
      setCloudUsage(null);
      setCloudBilling(null);
      setCloudError("");
    }
  }, [auth.is_logged_in, setCloudBilling]);

  // Load persisted default combo name from config
  useEffect(() => {
    cmd.getDefaultComboName().then((name) => {
      if (name) setDefaultComboName(name);
    }).catch(() => {});
  }, [setDefaultComboName]);

  // P8: Fetch active grants and load selected grant
  useEffect(() => {
    if (auth.is_logged_in) {
      cmd.fetchActiveGrants().then((grants) => {
        setActiveGrants(grants || []);
      }).catch(() => {});
      cmd.getSelectedGrant().then((sg) => {
        if (sg && sg.grant_id) {
          setSelectedGrant(sg);
        }
      }).catch(() => {});
    } else {
      setActiveGrants([]);
      setSelectedGrant(null);
    }
  }, [auth.is_logged_in, setActiveGrants, setSelectedGrant]);

  const handleSelectGrant = async (grantID: string) => {
    const relayGW = cloudBilling?.relay_gateways?.[0];
    if (!relayGW) return;
    const relayURL = `http://${relayGW.host}:${relayGW.port}`;
    await cmd.setSelectedGrant(grantID, relayURL);
    setSelectedGrant({ grant_id: grantID, relay_url: relayURL });
  };

  const handleClearGrant = async () => {
    await cmd.clearSelectedGrant();
    setSelectedGrant(null);
  };

  const handleToggleGateway = async () => {
    try {
      if (isRunning) {
        await cmd.stopLocalGateway();
        setRuntime({ ...runtime, gateway_running: false });
      } else {
        await cmd.startLocalGateway();
        setRuntime({ ...runtime, gateway_running: true });
      }
    } catch (e) {
      console.error(e);
    }
  };

  const allModels = [...new Set(channels.flatMap((ch) => ch.models || []))].sort();

  const handleSetDefaultModel = async (modelName: string) => {
    let channelId = appConfig?.default_channel_id || "";
    if (modelName && modelName !== "__auto__") {
      const candidate = channels.find(
        (ch) => ch.enabled && (ch.models || []).includes(modelName),
      );
      if (candidate) {
        channelId = candidate.channel_id;
      }
    }
    try {
      await cmd.updateDefaultModel(modelName, channelId);
      if (appConfig) setAppConfig({ ...appConfig, default_model: modelName, default_channel_id: channelId });
    } catch (e) {
      console.error(e);
    }
  };

  const handleSetDefaultChannel = async (channelId: string) => {
    let modelName = appConfig?.default_model || "";
    if (modelName && modelName !== "__auto__") {
      const channel = channels.find((ch) => ch.channel_id === channelId);
      if (channel && !(channel.models || []).includes(modelName)) {
        modelName = "";
      }
    }
    try {
      await cmd.updateDefaultModel(modelName, channelId);
      if (appConfig) setAppConfig({ ...appConfig, default_model: modelName, default_channel_id: channelId });
    } catch (e) {
      console.error(e);
    }
  };

  const handleCopyBaseUrl = async () => {
    const token = await cmd.getLocalAccessToken();
    await navigator.clipboard.writeText(`Base URL: ${baseUrl}\nAPI Key: ${token}`);
  };

  const platformChannels = channels.filter((ch) => ch.channel_type === "platform");
  const customChannels = channels.filter((ch) => ch.channel_type !== "platform");
  const planTier = normalizePlanTier(cloudBilling);
  const planLabel = formatPlanTier(cloudBilling);

  return (
    <div className="page home-page">
      {auth.is_logged_in && (
        <div className="auth-section">
          <div className="user-card">
            <div className="user-card-avatar">{auth.email?.[0]?.toUpperCase() || "U"}</div>
            <div className="user-card-info">
              <div className="user-card-name">{auth.email || t("home.loggedInAs", { email: "" })}</div>
              <div className="user-card-plan">
                {cloudBilling
                  ? t("home.planName", { plan: cloudBilling.plan_name })
                  : t("home.noBillingData")}
              </div>
            </div>
            {planTier && <div className={`plan-badge plan-badge-${planTier} user-card-badge`}>{planLabel}</div>}
          </div>

          <div className="usage-mini-grid">
            {loadingCloud ? (
              <div className="usage-mini-card">
                <div className="usage-mini-label">{t("home.usageThisMonth")}</div>
                <div className="usage-mini-loading">{t("home.noUsageData")}</div>
              </div>
            ) : cloudUsage ? (
              <>
                <div className="usage-mini-card">
                  <div className="usage-mini-label">{t("home.requests")}</div>
                  <div className="usage-mini-value blue">{cloudUsage.month_requests.toLocaleString()}</div>
                </div>
                <div className="usage-mini-card">
                  <div className="usage-mini-label">{t("home.inputTokens")}</div>
                  <div className="usage-mini-value">{cloudUsage.month_input_tokens.toLocaleString()}</div>
                </div>
                <div className="usage-mini-card">
                  <div className="usage-mini-label">{t("home.outputTokens")}</div>
                  <div className="usage-mini-value">{cloudUsage.month_output_tokens.toLocaleString()}</div>
                </div>
                <div className="usage-mini-card">
                  <div className="usage-mini-label">{t("home.costUSD")}</div>
                  <div className="usage-mini-value green">${cloudUsage.total_cost_usd.toFixed(2)}</div>
                </div>
              </>
            ) : cloudError ? (
              <div className="usage-mini-card">
                <div className="usage-mini-label">{t("home.usageThisMonth")}</div>
                <div className="usage-mini-error">{cloudError}</div>
              </div>
            ) : null}

            {cloudBilling && (
              <div className="usage-mini-card">
                <div className="usage-mini-label">{t("usage.quotaUsed")}</div>
                <div className="usage-mini-quota">
                  <div className="quota-bar">
                    <div
                      className="quota-fill"
                      style={{ width: `${Math.min(100, (cloudBilling.used_quota / cloudBilling.quota) * 100)}%` }}
                    />
                  </div>
                  <span>{cloudBilling.used_quota} / {cloudBilling.quota}</span>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {auth.is_logged_in && cloudBilling && (
        <div className="section section-card snapshot-card">
          <div className="section-heading">
            <h2>{t("home.snapshotTitle")}</h2>
            <p className="hint">{t("home.snapshotHint")}</p>
          </div>
          <div className="snapshot-grid">
            <div className="snapshot-item">
              <span className="snapshot-label">{t("home.snapshotCurrentPlan")}</span>
              <span className="snapshot-value">
                <span className={`plan-badge plan-badge-${planTier || "free"}`}>{planLabel || cloudBilling.plan_name}</span>
                <span className="snapshot-price">{cloudBilling.price > 0 ? `$${cloudBilling.price}/mo` : t("home.free")}</span>
              </span>
            </div>
            <div className="snapshot-item">
              <span className="snapshot-label">{t("home.snapshotQuotaUsage")}</span>
              <span className="snapshot-value">
                <span className="snapshot-quota-bar">
                  <span className="snapshot-quota-fill" style={{ width: `${Math.min(100, (cloudBilling.used_quota / cloudBilling.quota) * 100)}%` }} />
                </span>
                <span className="snapshot-quota-text">{cloudBilling.used_quota.toLocaleString()} / {cloudBilling.quota.toLocaleString()}</span>
              </span>
            </div>
            <div className="snapshot-item">
              <span className="snapshot-label">{t("home.snapshotRelayChannels")}</span>
              <span className="snapshot-value">
                <span className={`status-badge ${cloudBilling.relay_enabled ? "status-green" : "status-dim"}`}>
                  {cloudBilling.relay_enabled ? t("home.enabled") : t("home.disabled")}
                </span>
              </span>
            </div>
            <div className="snapshot-item">
              <span className="snapshot-label">{t("home.snapshotLocalGateway")}</span>
              <span className="snapshot-value">
                <span className={`status-badge ${isRunning ? "status-green" : "status-dim"}`}>
                  {isRunning ? t("home.running") : t("home.stopped")}
                </span>
              </span>
            </div>
            <div className="snapshot-item">
              <span className="snapshot-label">{t("home.snapshotAvailableChannels")}</span>
              <span className="snapshot-value">
                <strong>{channels.length}</strong>
                <span className="snapshot-sub">
                  {t("home.snapshotChannelBreakdown", { platform: platformChannels.length, custom: customChannels.length })}
                </span>
              </span>
            </div>
            <div className="snapshot-item">
              <span className="snapshot-label">{t("home.snapshotCloudChannels")}</span>
              <span className="snapshot-value">
                <strong>{cloudBilling.relay_gateways?.length || 0}</strong>
                <span className="snapshot-sub">{t("home.snapshotRelayNodes")}</span>
              </span>
            </div>
          </div>
          <div className="snapshot-footer">
            <button className="btn-primary btn-sm" onClick={() => navigate("/usage")}>{t("home.viewUsageDetails")}</button>
            <button className="btn-secondary btn-sm" onClick={() => navigate("/subscription")}>{t("home.manageSubscription")}</button>
          </div>
        </div>
      )}

      {auth.is_logged_in && (cloudBilling?.plan_id === "teams" || cloudBilling?.plan_id === "enterprise") && (
        <TeamCostSnapshot />
      )}

      {auth.is_logged_in && activeGrants.length > 0 && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>Token Grants</h2>
            <p className="hint">Active grants from token market purchases</p>
          </div>
          {selectedGrant && (
            <div className="snapshot-item" style={{ marginBottom: 12, padding: 8, background: "var(--color-bg-secondary, #f5f5f5)", borderRadius: 8 }}>
              <span className="snapshot-label">Selected Grant</span>
              <span className="snapshot-value">
                <strong>{String(selectedGrant.grant_id || "").slice(0, 16)}...</strong>
                <button className="btn-secondary btn-sm" style={{ marginLeft: 8 }} onClick={handleClearGrant}>Clear</button>
              </span>
            </div>
          )}
          <div className="snapshot-grid">
            {activeGrants.map((g) => (
              <div key={g.grant_id} className="snapshot-item" style={{ flexDirection: "column", alignItems: "flex-start", gap: 4 }}>
                <span className="snapshot-label">Grant {String(g.grant_id).slice(0, 12)}...</span>
                <span className="snapshot-value">
                  <strong>${(g.remaining_quota || 0).toFixed(2)}</strong> remaining
                </span>
                <span className="snapshot-sub">{(g.remaining_tokens || 0).toLocaleString()} tokens left</span>
                <button
                  className={selectedGrant?.grant_id === g.grant_id ? "btn-primary btn-sm" : "btn-secondary btn-sm"}
                  style={{ marginTop: 4 }}
                  onClick={() => handleSelectGrant(g.grant_id)}
                  disabled={!cloudBilling?.relay_gateways?.length}
                >
                  {selectedGrant?.grant_id === g.grant_id ? "Selected" : "Select"}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="page-hero">
        <div className="page-hero-head">
          <div className="page-hero-copy">
            <h1>{t("home.gatewayStatus")}</h1>
            <div className="page-hero-content-row">
              <p className="page-subtitle">{t("home.pageSubtitle")}</p>
              <div className="hero-metrics">
                <div className="hero-metric-card">
                  <span className="hero-metric-label">{t("home.metricGatewayStatus")}</span>
                  <strong className="hero-metric-value">{isRunning ? t("home.running") : t("home.stopped")}</strong>
                </div>
                <div className="hero-metric-card">
                  <span className="hero-metric-label">{t("home.metricAvailableChannels")}</span>
                  <strong className="hero-metric-value">{channels.length}</strong>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="home-status-grid">
        <div className="status-card status-card-emphasis">
          <div className="status-card-copy">
            <label>{t("home.gatewayStatus")}</label>
            <div className="status-card-title-row">
              <span className={`status-dot ${isRunning ? "ok" : "off"}`}></span>
              <strong className="status-card-title">{isRunning ? t("home.running") : t("home.stopped")}</strong>
            </div>
            <div className="text-dim">{t("home.gatewayDesc")}</div>
          </div>
          <button onClick={handleToggleGateway} className="btn-primary">
            {isRunning ? t("home.stop") : t("home.start")}
          </button>
        </div>

        {isRunning && (
          <div className="status-card">
            <div className="status-card-copy">
              <label>{t("home.localAddress")}</label>
              <code>{baseUrl}</code>
            </div>
            <button onClick={handleCopyBaseUrl} className="btn-secondary">
              {t("home.copyBaseUrl")}
            </button>
          </div>
        )}

        <div className="status-card">
          <div className="status-card-copy">
            <label>{t("home.defaultModel")}</label>
            <div className="text-dim">{t("home.defaultModelDesc")}</div>
          </div>
          <select
            value={appConfig?.default_model || ""}
            onChange={(e) => handleSetDefaultModel(e.target.value)}
          >
            <option value="">{t("home.notSet")}</option>
            <option value="__auto__">{t("home.autoSelect")}</option>
            {allModels.map((model) => (
              <option key={model} value={model}>
                {model}
              </option>
            ))}
          </select>
        </div>

        <div className="status-card">
          <div className="status-card-copy">
            <label>{t("home.defaultChannel")}</label>
            <div className="text-dim">{t("home.defaultChannelDesc")}</div>
          </div>
          <select
            value={appConfig?.default_channel_id || ""}
            onChange={(e) => handleSetDefaultChannel(e.target.value)}
          >
            <option value="">{t("home.notSet")}</option>
            <option value="__auto__">{t("home.autoSelect")}</option>
            {channels.map((ch) => (
              <option key={ch.channel_id} value={ch.channel_id}>
                {ch.display_name}
              </option>
            ))}
          </select>
        </div>

        <div className="section-card-inline">
          <div>
            <strong>{t("home.defaultCombo")}</strong>
            <div className="text-dim">{t("home.defaultComboDesc")}</div>
          </div>
          <select
            value={defaultComboName || ""}
            onChange={(e) => {
              const val = e.target.value;
              setDefaultComboName(val);
              cmd.setDefaultComboName(val).catch(() => {});
              setAppConfig({ ...appConfig!, default_combo_name: val || undefined });
            }}
          >
            <option value="">{t("home.notSet")}</option>
            {combos.map((c) => (
              <option key={c.name} value={c.name}>
                {t("home.comboWithStepCount", { name: c.name, count: c.steps?.length || 0 })}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("home.advancedTitle")}</h2>
          <p className="hint">{t("home.advancedHint")}</p>
        </div>
        <div className="advanced-capability-grid">
          <div className="advanced-capability-card">
            <div className="advanced-capability-head">
              <strong>{t("home.advancedTranslatorTitle")}</strong>
              <span className="badge badge-blue">{t("home.advancedTranslatorBadge")}</span>
            </div>
            <p className="hint">{t("home.advancedTranslatorDesc")}</p>
            <div className="advanced-capability-tags">
              <span className="badge badge-green">Responses API</span>
              <span className="badge badge-blue">Chat Completions</span>
              <span className="badge badge-yellow">Gemini</span>
            </div>
            <div className="advanced-capability-actions">
              <button className="btn-secondary btn-sm" onClick={() => navigate("/translator")}>{t("home.openTranslator")}</button>
            </div>
          </div>

          <div className="advanced-capability-card">
            <div className="advanced-capability-head">
              <strong>{t("home.advancedCompressionTitle")}</strong>
              <span className="badge badge-green">{t("home.advancedCompressionBadge")}</span>
            </div>
            <p className="hint">{t("home.advancedCompressionDesc")}</p>
            <div className="advanced-capability-tags">
              <span className="badge badge-blue">RTK</span>
              <span className="badge badge-blue">Caveman</span>
              <span className="badge badge-green">Semantic Cache</span>
            </div>
            <div className="advanced-capability-actions">
              <button className="btn-secondary btn-sm" onClick={() => navigate("/settings")}>{t("home.openCompressionSettings")}</button>
            </div>
          </div>

          <div className="advanced-capability-card">
            <div className="advanced-capability-head">
              <strong>{t("home.advancedRoutingTitle")}</strong>
              <span className="badge badge-yellow">{t("home.advancedRoutingBadge")}</span>
            </div>
            <p className="hint">{t("home.advancedRoutingDesc")}</p>
            <div className="advanced-capability-tags">
              <span className="badge badge-blue">Fusion</span>
              <span className="badge badge-blue">Family Fallback</span>
              <span className="badge badge-green">Reasoning Replay</span>
            </div>
            <div className="advanced-capability-actions">
              <button className="btn-secondary btn-sm" onClick={() => navigate("/logs")}>{t("home.openLogsRouting")}</button>
            </div>
          </div>

          <div className="advanced-capability-card">
            <div className="advanced-capability-head">
              <strong>{t("home.advancedGovernanceTitle")}</strong>
              <span className="badge badge-blue">{t("home.advancedGovernanceBadge")}</span>
            </div>
            <p className="hint">{t("home.advancedGovernanceDesc")}</p>
            <div className="advanced-capability-tags">
              <span className="badge badge-yellow">A2A</span>
              <span className="badge badge-green">Per-key Quota</span>
              <span className="badge badge-blue">WebSocket</span>
            </div>
            <div className="advanced-capability-actions">
              <button className="btn-secondary btn-sm" onClick={() => navigate("/settings")}>{t("home.openGovernanceSettings")}</button>
            </div>
          </div>
        </div>
      </div>

      <div className="section-heading">
        <h2>{t("home.availableChannels")}</h2>
        <p className="hint">{t("home.availableChannelsHint")}</p>
      </div>

      {platformChannels.length > 0 && (
        <div className="channel-group">
          <h3>{t("home.platform")}</h3>
          <p className="hint">{t("channel.platformHint")}</p>
          {platformChannels.map((ch) => (
            <div key={ch.channel_id} className="channel-item">
              <span className="channel-name">{ch.display_name}</span>
              <span className="channel-models">{t("home.modelCount", { count: ch.models?.length || 0 })}</span>
              <span className={`status-dot ${ch.health_status === "healthy" ? "ok" : "off"}`}></span>
            </div>
          ))}
        </div>
      )}

      {customChannels.length > 0 && (
        <div className="channel-group">
          <h3>{t("home.custom")}</h3>
          <p className="hint">{t("channel.customHint")}</p>
          {customChannels.map((ch) => (
            <div key={ch.channel_id} className="channel-item">
              <span className="channel-name">{ch.display_name}</span>
              <span className="channel-models">{t("home.modelCount", { count: ch.models?.length || 0 })}</span>
              <span className={`status-dot ${ch.health_status === "healthy" ? "ok" : "off"}`}></span>
            </div>
          ))}
        </div>
      )}

      {platformChannels.length === 0 && customChannels.length === 0 && (
        <div className="empty-hint">{t("home.loginToUnlock")}</div>
      )}
    </div>
  );
}
