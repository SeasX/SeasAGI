import { useState, useEffect, useCallback } from "react";
import * as cmd from "../utils/commands";
import { useAppStore } from "../stores/appStore";
import { useTranslation } from "../i18n";

interface ComboRouteMetricsItem {
  combo_name: string;
  total_requests: number;
  total_fallbacks: number;
  step1_success: number;
  last_step_fallback: number;
  step1_hit_rate: number;
  last_step_hit_rate: number;
  avg_attempts: number;
  task_type?: string;
}

export function ExecAnalysisPage() {
  const { t } = useTranslation();
  const [metrics, setMetrics] = useState<ComboRouteMetricsItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [selectedCombo, setSelectedCombo] = useState<string | null>(null);
  const [taskFilter, setTaskFilter] = useState<"all" | "chat" | "tools">("all");
  const { providerHealth, setProviderHealth } = useAppStore();

  const loadMetrics = useCallback(async () => {
    try {
      const data = await cmd.getComboRouteMetrics();
      setMetrics(data || []);
      setError("");
    } catch (e: any) {
      setError(e.message || t("execAnalysis.loadFailed"));
    }
    setLoading(false);
  }, []);

  const loadProviderHealth = useCallback(async () => {
    try {
      const data = await cmd.getProviderHealthSummary();
      setProviderHealth(data || []);
    } catch {}
  }, [setProviderHealth]);

  // M4.4 意图预测实验室：只做路由决策预演，不发真实请求
  const [labPrompt, setLabPrompt] = useState("");
  const [labModel, setLabModel] = useState("");
  const [labResult, setLabResult] = useState<any>(null);
  const [labLoading, setLabLoading] = useState(false);
  const [comboNames, setComboNames] = useState<string[]>([]);
  // M4.5 意图场景分布
  const [intentStats, setIntentStats] = useState<any[]>([]);

  const loadIntentStats = useCallback(async () => {
    try {
      const data = await cmd.getIntentScenarioStats();
      setIntentStats(data || []);
    } catch {}
  }, []);

  const runSimulation = useCallback(async () => {
    if (!labPrompt.trim() || labLoading) return;
    setLabLoading(true);
    try {
      const r = await cmd.simulateIntentRouting(labPrompt, labModel);
      setLabResult(r);
    } catch (e: any) {
      setLabResult({ error: e.message || t("execAnalysis.labFailed") });
    }
    setLabLoading(false);
  }, [labPrompt, labModel, labLoading, t]);

  useEffect(() => {
    loadMetrics();
    loadProviderHealth();
    loadIntentStats();
    cmd.listModelCombos().then((cs) => setComboNames((cs || []).map((c) => c.name))).catch(() => {});
    cmd.getDefaultComboName().then((n) => setLabModel(n || "")).catch(() => {});
  }, [loadMetrics, loadProviderHealth, loadIntentStats]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(() => { loadMetrics(); loadProviderHealth(); loadIntentStats(); }, 5000);
    return () => clearInterval(interval);
  }, [autoRefresh, loadMetrics, loadProviderHealth, loadIntentStats]);

  const formatPct = (v: number) => (v * 100).toFixed(1) + "%";
  const formatNum = (v: number) => v.toLocaleString();

  const filteredMetrics = taskFilter === "all" ? metrics : metrics.filter((m) => m.task_type === taskFilter);
  const selected = filteredMetrics.find((m) => m.combo_name === selectedCombo);
  const sorted = [...filteredMetrics].sort((a, b) => b.total_requests - a.total_requests);

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1>{t("execAnalysis.title")}</h1>
          <p className="page-subtitle">{t("execAnalysis.subtitle")}</p>
        </div>
        <div className="action-bar">
          <select value={taskFilter} onChange={(e) => setTaskFilter(e.target.value as any)} style={{ fontSize: 13, padding: "4px 8px", borderRadius: 4 }}>
            <option value="all">{t("execAnalysis.allTypes")}</option>
            <option value="chat">Chat</option>
            <option value="tools">Tools</option>
          </select>
          <label className="toggle-row" style={{ fontSize: 13 }}>
            <input type="checkbox" checked={autoRefresh} onChange={(e) => setAutoRefresh(e.target.checked)} />
            <span>{t("execAnalysis.autoRefresh")}</span>
          </label>
          <button className="btn-sm" onClick={loadMetrics} disabled={loading}>
            {loading ? t("execAnalysis.refreshing") : t("execAnalysis.refresh")}
          </button>
        </div>
      </div>

      {error && <div className="error-msg">{error}</div>}

      {!loading && metrics.length === 0 && !error && (
        <div className="empty-state">
          {t("execAnalysis.noData")}
        </div>
      )}

      {/* Summary Cards */}
      {metrics.length > 0 && (
        <div className="metrics-grid" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 12, marginBottom: 20 }}>
          <div className="stat-card">
            <div className="stat-label">{t("execAnalysis.monitoredCombos")}</div>
            <div className="stat-value">{metrics.length}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">{t("execAnalysis.totalRequests")}</div>
            <div className="stat-value">{formatNum(metrics.reduce((s, m) => s + m.total_requests, 0))}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">{t("execAnalysis.totalFallbacks")}</div>
            <div className="stat-value">{formatNum(metrics.reduce((s, m) => s + m.total_fallbacks, 0))}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">{t("execAnalysis.step1HitRateOverall")}</div>
            <div className="stat-value">
              {(() => {
                const total = metrics.reduce((s, m) => s + m.total_requests, 0);
                const hits = metrics.reduce((s, m) => s + m.step1_success, 0);
                return total > 0 ? formatPct(hits / total) : "-";
              })()}
            </div>
          </div>
        </div>
      )}

      {/* Combo List with bar chart */}
      {sorted.length > 0 && (
        <div className="section section-card">
          <h2 className="section-title">{t("execAnalysis.comboHitOverview")}</h2>
          <div className="combo-metrics-list" style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {sorted.map((m) => (
              <div
                key={m.combo_name}
                className={`combo-metrics-row ${selectedCombo === m.combo_name ? "selected" : ""}`}
                onClick={() => setSelectedCombo(selectedCombo === m.combo_name ? null : m.combo_name)}
                style={{
                  padding: "10px 14px",
                  borderRadius: "var(--radius-sm)",
                  background: selectedCombo === m.combo_name ? "var(--accent-light)" : "var(--bg-tertiary)",
                  cursor: "pointer",
                  transition: "var(--transition)",
                }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 6 }}>
                  <strong style={{ fontSize: 14 }}>{m.combo_name}</strong>
                  <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
                    {t("execAnalysis.requestsAndFallbacks", { req: formatNum(m.total_requests), fb: formatNum(m.total_fallbacks) })}
                  </span>
                </div>
                <div style={{ display: "flex", gap: 16, alignItems: "center", fontSize: 12 }}>
                  <div style={{ flex: 1 }}>
                    <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 2 }}>
                      <span style={{ color: "var(--green)" }}>{t("execAnalysis.step1HitRate")}</span>
                      <span>{formatPct(m.step1_hit_rate)}</span>
                    </div>
                    <div style={{ height: 6, background: "var(--bg-primary)", borderRadius: 3, overflow: "hidden" }}>
                      <div style={{ width: `${m.step1_hit_rate * 100}%`, height: "100%", background: "var(--green)", borderRadius: 3, transition: "width 0.3s" }} />
                    </div>
                  </div>
                  <div style={{ flex: 1 }}>
                    <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 2 }}>
                      <span style={{ color: "var(--yellow)" }}>{t("execAnalysis.lastStepHitRate")}</span>
                      <span>{formatPct(m.last_step_hit_rate)}</span>
                    </div>
                    <div style={{ height: 6, background: "var(--bg-primary)", borderRadius: 3, overflow: "hidden" }}>
                      <div style={{ width: `${m.last_step_hit_rate * 100}%`, height: "100%", background: "var(--yellow)", borderRadius: 3, transition: "width 0.3s" }} />
                    </div>
                  </div>
                  <div style={{ minWidth: 80, textAlign: "right" }}>
                    <span className="text-muted">{t("execAnalysis.avgAttempts")} </span>
                    <strong>{m.avg_attempts.toFixed(2)}</strong>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Detail Panel */}
      {selected && (
        <div className="section section-card">
          <h2 className="section-title">{t("execAnalysis.detailTitle", { name: selected.combo_name })}</h2>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))", gap: 12 }}>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.totalRequests")}</div>
              <div className="stat-value">{formatNum(selected.total_requests)}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.step1Success")}</div>
              <div className="stat-value">{formatNum(selected.step1_success)}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.fallbackCount")}</div>
              <div className="stat-value">{formatNum(selected.total_fallbacks)}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.lastStepFallback")}</div>
              <div className="stat-value">{formatNum(selected.last_step_fallback)}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.step1HitRate")}</div>
              <div className="stat-value" style={{ color: selected.step1_hit_rate > 0.8 ? "var(--green)" : "var(--yellow)" }}>
                {formatPct(selected.step1_hit_rate)}
              </div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.lastStepHitRate")}</div>
              <div className="stat-value" style={{ color: selected.last_step_hit_rate > 0.5 ? "var(--green)" : "var(--red)" }}>
                {formatPct(selected.last_step_hit_rate)}
              </div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.avgAttemptsCount")}</div>
              <div className="stat-value">{selected.avg_attempts.toFixed(2)}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">{t("execAnalysis.evaluation")}</div>
              <div className="stat-value" style={{ fontSize: 13 }}>
                {selected.step1_hit_rate > 0.9
                  ? t("execAnalysis.excellent")
                  : selected.step1_hit_rate > 0.7
                  ? t("execAnalysis.good")
                  : selected.step1_hit_rate > 0.5
                  ? t("execAnalysis.needsOptimization")
                  : t("execAnalysis.poor")}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* M4.4 Intent Prediction Lab */}
      <div className="section section-card">
        <h2 className="section-title">{t("execAnalysis.intentLab")}</h2>
        <div style={{ display: "flex", gap: 8, marginBottom: 10, flexWrap: "wrap" }}>
          <input
            value={labPrompt}
            onChange={(e) => setLabPrompt(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && runSimulation()}
            placeholder={t("execAnalysis.labPlaceholder")}
            style={{ flex: 1, minWidth: 240, fontSize: 13, padding: "6px 10px", borderRadius: "var(--radius-sm)", background: "var(--bg-tertiary)", border: "1px solid var(--border-color)", color: "var(--text-primary)" }}
          />
          <select value={labModel} onChange={(e) => setLabModel(e.target.value)} style={{ fontSize: 13, padding: "6px 8px", borderRadius: "var(--radius-sm)" }}>
            {comboNames.map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
          <button className="btn-sm" onClick={runSimulation} disabled={labLoading || !labPrompt.trim()}>
            {labLoading ? t("execAnalysis.labRunning") : t("execAnalysis.labRun")}
          </button>
        </div>
        {labResult && !labResult.error && (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
            <div style={{ minWidth: 240 }}>
              <div style={{ fontSize: 12, color: "var(--text-muted)", marginBottom: 6 }}>{t("execAnalysis.detectedIntent")}</div>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                <span className="badge badge-blue">{labResult.intent.scenario}</span>
                <span className="badge">{labResult.intent.task_type}</span>
                <span className="badge">IQ {labResult.intent.required_iq}</span>
                <span className="badge">{labResult.intent.security_level}</span>
                <span className="badge">{t("execAnalysis.confidence")} {formatPct(labResult.intent.confidence)}</span>
              </div>
            </div>
            <div style={{ flex: 1, minWidth: 260 }}>
              <div style={{ fontSize: 12, color: "var(--text-muted)", marginBottom: 6 }}>
                {t("execAnalysis.routingPlan")}: {labResult.plan_name} @ {labResult.model}
              </div>
              {(labResult.steps || []).map((s: any) => (
                <div key={s.order} style={{ display: "flex", gap: 8, alignItems: "center", padding: "6px 10px", marginBottom: 4, borderRadius: "var(--radius-sm)", background: "var(--bg-tertiary)", fontSize: 13 }}>
                  <span style={{ color: "var(--text-muted)", minWidth: 20 }}>#{s.order}</span>
                  <strong>{s.channel_name || s.channel_id}</strong>
                  <span className="text-muted">{s.upstream_model}</span>
                  <span className="badge">{s.step_role}</span>
                </div>
              ))}
            </div>
          </div>
        )}
        {labResult?.error && <div className="error-msg">{labResult.error}</div>}
      </div>

      {/* M4.5 Intent Scenario Distribution */}
      {intentStats.length > 0 && (
        <div className="section section-card">
          <h2 className="section-title">{t("execAnalysis.intentDistribution")}</h2>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {intentStats.map((s) => (
              <div key={s.scenario} style={{ padding: "8px 14px", borderRadius: "var(--radius-sm)", background: "var(--bg-tertiary)" }}>
                <div style={{ display: "flex", justifyContent: "space-between", fontSize: 13, marginBottom: 4 }}>
                  <strong>{s.scenario}</strong>
                  <span style={{ color: "var(--text-muted)" }}>{formatNum(s.count)} · {formatPct(s.share)}</span>
                </div>
                <div style={{ height: 6, background: "var(--bg-primary)", borderRadius: 3, overflow: "hidden" }}>
                  <div style={{ width: `${s.share * 100}%`, height: "100%", background: "var(--primary)", borderRadius: 3, transition: "width 0.3s" }} />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Provider Health Summary — M4.3 热力图矩阵 */}
      {providerHealth.length > 0 && (
        <div className="section section-card">
          <h2 className="section-title">{t("execAnalysis.providerHealthOverview")}</h2>
          <div className="heat-grid" style={{ gridTemplateColumns: "minmax(120px, 1.4fr) repeat(4, 1fr)" }}>
            <div className="heat-head">{t("execAnalysis.provider")}</div>
            <div className="heat-head">{t("execAnalysis.successRate")}</div>
            <div className="heat-head">{t("execAnalysis.errorRate")}</div>
            <div className="heat-head">{t("execAnalysis.latency")}</div>
            <div className="heat-head">{t("execAnalysis.circuit")}</div>
            {providerHealth.map((p) => {
              const tier = (ok: boolean, warn: boolean) => (ok ? "var(--heat-ok)" : warn ? "var(--heat-warn)" : "var(--heat-bad)");
              const okColor = tier(p.avg_success_rate >= 0.98, p.avg_success_rate >= 0.9);
              const errRate = p.avg_error_rate ?? 1 - p.avg_success_rate;
              const errColor = tier(errRate <= 0.02, errRate <= 0.1);
              const latColor = tier(p.avg_latency_ms <= 800, p.avg_latency_ms <= 2000);
              const cbColor = tier(p.circuit_open_count === 0, p.circuit_open_count <= 2);
              return (
                <div key={p.provider_id} style={{ display: "contents" }}>
                  <div className="heat-row-label">{p.provider_id}</div>
                  <div className="heat-cell" style={{ background: okColor }}>{formatPct(p.avg_success_rate)}</div>
                  <div className="heat-cell" style={{ background: errColor }}>{formatPct(errRate)}</div>
                  <div className="heat-cell" style={{ background: latColor }}>{p.avg_latency_ms.toFixed(0)}ms</div>
                  <div className="heat-cell" style={{ background: cbColor }}>
                    {p.circuit_open_count > 0 ? t("execAnalysis.circuitBreaker", { count: p.circuit_open_count }) : t("execAnalysis.normal")}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
