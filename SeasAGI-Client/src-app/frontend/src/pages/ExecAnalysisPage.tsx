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

  useEffect(() => {
    loadMetrics();
    loadProviderHealth();
  }, [loadMetrics, loadProviderHealth]);

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(() => { loadMetrics(); loadProviderHealth(); }, 5000);
    return () => clearInterval(interval);
  }, [autoRefresh, loadMetrics, loadProviderHealth]);

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

      {/* Provider Health Summary */}
      {providerHealth.length > 0 && (
        <div className="section section-card">
          <h2 className="section-title">{t("execAnalysis.providerHealthOverview")}</h2>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))", gap: 8 }}>
            {providerHealth.map((p) => (
              <div key={p.provider_id} style={{ padding: "10px 14px", borderRadius: "var(--radius-sm)", background: "var(--bg-tertiary)" }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 4 }}>
                  <strong style={{ fontSize: 13 }}>{p.provider_id}</strong>
                  <span style={{ fontSize: 11, color: p.circuit_open_count > 0 ? "var(--red)" : "var(--green)" }}>
                    {p.circuit_open_count > 0 ? t("execAnalysis.circuitBreaker", { count: p.circuit_open_count }) : t("execAnalysis.normal")}
                  </span>
                </div>
                <div style={{ display: "flex", gap: 12, fontSize: 11, color: "var(--text-muted)" }}>
                  <span>{t("execAnalysis.successRate")} {formatPct(p.avg_success_rate)}</span>
                  <span>{t("execAnalysis.latency")} {p.avg_latency_ms.toFixed(0)}ms</span>
                  <span>{t("execAnalysis.penalty")} {p.avg_penalty_score.toFixed(2)}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
