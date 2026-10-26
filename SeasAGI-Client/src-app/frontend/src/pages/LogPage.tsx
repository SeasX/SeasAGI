import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import type { RequestLog } from "../utils/types";
import { useTranslation } from "../i18n";

interface LogFilterPreset {
  name: string;
  search: string;
  status: string;
  channel: string;
  sort: string;
}

const PRESET_KEY = "seasagi-log-filter-presets";

function loadPresets(): LogFilterPreset[] {
  try {
    return JSON.parse(localStorage.getItem(PRESET_KEY) || "[]");
  } catch { return []; }
}

function savePresets(presets: LogFilterPreset[]) {
  localStorage.setItem(PRESET_KEY, JSON.stringify(presets));
}

export function LogPage() {
  const logs = useAppStore((s) => s.logs);
  const setLogs = useAppStore((s) => s.setLogs);
  const { t } = useTranslation();

  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const mitmOnly = searchParams.get("mitm") === "1";
  const [search, setSearch] = useState(searchParams.get("q") || "");
  const [statusFilter, setStatusFilter] = useState("");
  const [channelFilter, setChannelFilter] = useState("");
  const [sortOrder, setSortOrder] = useState("newest");
  const [presets, setPresets] = useState<LogFilterPreset[]>(loadPresets);
  const [presetName, setPresetName] = useState("");
  const [showPresets, setShowPresets] = useState(false);
  const [expandedRoute, setExpandedRoute] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [clearing, setClearing] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const [channelOptions, setChannelOptions] = useState<string[]>([]);

  // 卸载标记 + 请求序号：卸载或筛选变更后的过期响应不再写入状态。
  const mountedRef = useRef(true);
  const requestSeq = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const refreshLogs = useCallback(async () => {
    const reqId = ++requestSeq.current;
    setLoading(true);
    setLoadError(null);
    try {
      const items = await cmd.listLogsFiltered(500, 0, statusFilter, channelFilter, "", "", "");
      if (!mountedRef.current || reqId !== requestSeq.current) return;
      const next = items || [];
      setLogs(next);
      setChannelOptions((prev) => {
        const merged = new Set(prev);
        for (const item of next) {
          if (item.channel_id) merged.add(item.channel_id);
        }
        return Array.from(merged);
      });
    } catch (e) {
      if (!mountedRef.current || reqId !== requestSeq.current) return;
      setLoadError(e instanceof Error ? e.message : String(e));
    } finally {
      if (mountedRef.current && reqId === requestSeq.current) setLoading(false);
    }
  }, [channelFilter, setLogs, statusFilter]);

  // 进入页面、切换筛选（搜索/状态/通道/排序）或重试时重新拉取。
  useEffect(() => {
    const timer = window.setTimeout(() => {
      void refreshLogs();
    }, 200);
    return () => window.clearTimeout(timer);
  }, [refreshLogs, search, sortOrder, mitmOnly, reloadKey]);

  const handleRetry = () => setReloadKey((key) => key + 1);

  const channelTypeColors: Record<string, string> = {
    local: "var(--green)",
    platform: "var(--accent)",
    relay: "var(--orange)",
    unknown: "var(--text-dim)",
  };

  const getChannelTypeLabel = (channelType: string) => {
    switch (channelType) {
      case "local":
      case "platform":
      case "relay":
      case "unknown":
        return t(`log.channelType.${channelType}`);
      default:
        return channelType;
    }
  };

  const getChannelType = (log: RequestLog): string => {
    const id = log.channel_id || "";
    if (id.startsWith("relay_") || id.startsWith("relay-")) return "relay";
    if (id.startsWith("platform_") || id.startsWith("platform-")) return "platform";
    if (!id || id === "local" || id === "custom") return "local";
    return "unknown";
  };
  const filtered = logs
    .filter((l) => {
      if (mitmOnly && !l.mitm_target) return false;
      if (
        search &&
        !l.logical_model_name?.toLowerCase().includes(search.toLowerCase()) &&
        !l.channel_id?.toLowerCase().includes(search.toLowerCase()) &&
        !l.error_message?.toLowerCase().includes(search.toLowerCase()) &&
        !l.mitm_target?.toLowerCase().includes(search.toLowerCase()) &&
        !l.resolved_target?.toLowerCase().includes(search.toLowerCase()) &&
        !l.requested_model?.toLowerCase().includes(search.toLowerCase())
      ) {
        return false;
      }
      if (statusFilter && l.status !== statusFilter) return false;
      if (channelFilter && l.channel_id !== channelFilter) return false;
      return true;
    })
    .sort((a, b) => (sortOrder === "newest" ? (b.created_at > a.created_at ? 1 : -1) : (a.created_at > b.created_at ? 1 : -1)));

  const successCount = filtered.filter((log) => log.status === "success").length;
  const failureCount = filtered.length - successCount;
  const avgDuration = filtered.length > 0 ? Math.round(filtered.reduce((sum, log) => sum + log.duration_ms, 0) / filtered.length) : 0;
  const ttftSamples = filtered.filter((log) => (log.ttft_ms || 0) > 0);
  const avgTtft = ttftSamples.length > 0 ? Math.round(ttftSamples.reduce((sum, log) => sum + log.ttft_ms, 0) / ttftSamples.length) : 0;
  const activeFilters = [
    search ? `${t("log.searchLabel")}：${search}` : "",
    statusFilter ? `${t("log.statusLabel")}：${statusFilter === "success" ? t("log.success") : t("log.failure")}` : "",
    channelFilter ? `${t("log.channelLabel")}：${channelFilter}` : "",
    sortOrder ? `${t("log.sortLabel")}：${sortOrder === "newest" ? t("log.newestFirst") : t("log.oldestFirst")}` : "",
  ].filter(Boolean);

  const formatDateTime = (value: string) => {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  };

  const formatDuration = (value: number) => (value >= 1000 ? `${(value / 1000).toFixed(2)}s` : `${value}ms`);

  const handleExportCSV = () => {
    const headers = [t("log.csvHeaders.time"), t("log.csvHeaders.logicalModel"), t("log.csvHeaders.channel"), t("log.csvHeaders.upstreamModel"), t("log.csvHeaders.duration"), t("log.csvHeaders.httpStatus"), t("log.csvHeaders.ttft"), t("log.csvHeaders.tokens"), t("log.csvHeaders.status"), t("log.csvHeaders.errorCode"), t("log.csvHeaders.errorMessage"), t("log.csvHeaders.routeTrace"), t("log.csvHeaders.stepCount")];
    const rows = filtered.map((l) => [l.created_at, l.logical_model_name, l.channel_id, l.upstream_model, String(l.duration_ms), String(l.http_status || ""), String(l.ttft_ms || ""), String((l.input_tokens || 0) + (l.output_tokens || 0)), l.status, l.error_code || "", l.error_message || "", l.route_trace, String(l.route_steps?.length || 0)]);
    const csv = "\uFEFF" + [headers.join(","), ...rows.map((r) => r.map((c) => `"${c}"`).join(","))].join("\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url; a.download = `seasagi-logs-${new Date().toISOString().slice(0, 10)}.csv`; a.click();
    URL.revokeObjectURL(url);
  };

  const handleClear = async () => {
    if (!window.confirm(t("log.confirmClear"))) return;
    setClearing(true);
    setLoadError(null);
    try {
      await cmd.clearLogs();
      setLogs([]);
      setChannelOptions([]);
      handleRetry();
    } catch (e) {
      const detail = e instanceof Error ? e.message : String(e);
      setLoadError(`${t("log.clearFailed")}: ${detail}`);
    } finally {
      setClearing(false);
    }
  };

  const handleSavePreset = () => {
    if (!presetName) return;
    const newPresets = [...presets, { name: presetName, search, status: statusFilter, channel: channelFilter, sort: sortOrder }];
    setPresets(newPresets);
    savePresets(newPresets);
    setPresetName("");
  };

  const handleApplyPreset = (p: LogFilterPreset) => {
    setSearch(p.search); setStatusFilter(p.status); setChannelFilter(p.channel); setSortOrder(p.sort);
  };

  const handleDeletePreset = (name: string) => {
    const newPresets = presets.filter((p) => p.name !== name);
    setPresets(newPresets);
    savePresets(newPresets);
  };

  // P3-16：Log-to-Rule——按日志来源元数据回填并跳转到 MITM 路由编辑器。
  const handleCreateRule = (log: RequestLog) => {
    const params = new URLSearchParams({ tab: "mitm" });
    if (log.mitm_target) params.set("rule_target", log.mitm_target);
    if (log.requested_model) params.set("rule_model", log.requested_model);
    navigate(`/settings?${params.toString()}`);
  };

  const handleCopyError = (log: RequestLog, format: "text" | "json" | "markdown") => {
    if (!log.error_message) return;
    if (format === "text") { navigator.clipboard.writeText(log.error_message); }
    else if (format === "json") { navigator.clipboard.writeText(JSON.stringify(log, null, 2)); }
    else { navigator.clipboard.writeText(`### ${t("log.requestDiagnosis")}\n\n- **${t("log.requestId")}**: ${log.request_id}\n- **${t("log.logicalModel")}**: ${log.logical_model_name}\n- **${t("log.hitChannel")}**: ${log.channel_id}\n- **${t("log.upstreamModel")}**: ${log.upstream_model}\n- **${t("log.status")}**: ${log.status}\n- **${t("log.error")}**: ${log.error_message}\n\n\`\`\`json\n${JSON.stringify(log.route_steps || [], null, 2)}\n\`\`\``); }
  };

  return (
    <div className="page log-page">
      <div className="page-header log-page-header">
        <div>
          <h1>{t("log.title")}</h1>
          <p className="page-subtitle">{t("log.subtitle")}</p>
        </div>
        <div className="log-summary-inline">
          <div className="log-summary-chip">
            <span className="log-summary-label">{t("log.totalRecords")}</span>
            <strong>{filtered.length}</strong>
          </div>
          <div className="log-summary-chip">
            <span className="log-summary-label">{t("log.avgDuration")}</span>
            <strong>{formatDuration(avgDuration)}</strong>
          </div>
          <div className="log-summary-chip success">
            <span className="log-summary-label">{t("log.successCount")}</span>
            <strong>{successCount}</strong>
          </div>
          <div className="log-summary-chip danger">
            <span className="log-summary-label">{t("log.failureCount")}</span>
            <strong>{failureCount}</strong>
          </div>
        </div>
      </div>

      <div className="tab-content log-panel section-card">
          <div className="section-heading">
            <h2>{t("log.filterSection")}</h2>
            <p className="hint">{t("log.listItemHint")}</p>
          </div>
        <div className="form-row log-filter-grid">
          <div className="form-group log-search-group">
            <label>{t("log.searchLabel")}</label>
            <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t("log.searchPlaceholder")} />
          </div>
          <div className="form-group">
            <label>{t("log.statusLabel")}</label>
            <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
              <option value="">{t("log.allStatus")}</option>
              <option value="success">{t("log.success")}</option>
              <option value="failure">{t("log.failure")}</option>
            </select>
          </div>
        </div>
        <div className="form-row log-filter-grid">
          <div className="form-group">
            <label>{t("log.channelLabel")}</label>
            <select value={channelFilter} onChange={(e) => setChannelFilter(e.target.value)}>
              <option value="">{t("log.allChannels")}</option>
              {channelOptions.map((ch) => (
                <option key={ch} value={ch}>
                  {ch}
                </option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label>{t("log.sortLabel")}</label>
            <select value={sortOrder} onChange={(e) => setSortOrder(e.target.value)}>
              <option value="newest">{t("log.newestFirst")}</option>
              <option value="oldest">{t("log.oldestFirst")}</option>
            </select>
          </div>
        </div>

        <div className="log-toolbar">
          <button
            onClick={() => {
              setSearch("");
              setStatusFilter("");
              setChannelFilter("");
              setSortOrder("newest");
            }}
            className="btn-secondary"
          >
            {t("log.reset")}
          </button>
          <button onClick={() => setShowPresets(!showPresets)} className="btn-secondary">
            {showPresets ? t("log.collapsePresets") : t("log.filterPresets")}
          </button>
          <button onClick={handleExportCSV} className="btn-secondary">
            {t("log.exportCSV")}
          </button>
          <button onClick={handleClear} className="btn-danger" disabled={clearing}>
            {clearing ? t("common.processing") : t("log.clear")}
          </button>
        </div>

        <div className="log-active-filters">
          <span className="log-active-title">{t("log.filterSummary")}</span>
          {activeFilters.length === 0 ? (
            <span className="hint">{t("log.noActiveFilters")}</span>
          ) : (
            <div className="log-filter-tags">
              {activeFilters.map((item) => (
                <span key={item} className="badge badge-blue">
                  {item}
                </span>
              ))}
            </div>
          )}
        </div>
      </div>

      {showPresets && (
        <div className="tab-content log-panel section-card">
          <div className="section-heading">
            <h2>{t("log.presetSection")}</h2>
            <p className="hint">{t("log.presetHint")}</p>
          </div>
          <div className="form-row log-preset-save">
            <div className="form-group">
              <label>{t("log.presetName")}</label>
              <input value={presetName} onChange={(e) => setPresetName(e.target.value)} placeholder={t("log.presetName")} />
            </div>
            <div className="form-group log-preset-save-action">
              <label>&nbsp;</label>
              <button onClick={handleSavePreset} className="btn-primary">
                {t("log.saveCurrentFilter")}
              </button>
            </div>
          </div>

          {presets.length === 0 && <p className="empty-hint">{t("log.noPresets")}</p>}
          {presets.length > 0 && (
            <div className="log-preset-list">
              {presets.map((p) => (
                <div key={p.name} className="log-preset-card">
                  <div>
                    <div className="log-preset-name">{p.name}</div>
                    <div className="log-preset-meta">
                      {[p.search, p.status, p.channel, p.sort].filter(Boolean).join(" · ") || t("log.noActiveFilters")}
                    </div>
                  </div>
                  <div className="log-preset-actions">
                    <button onClick={() => handleApplyPreset(p)} className="btn-secondary btn-sm">
                      {t("log.apply")}
                    </button>
                    <button onClick={() => handleDeletePreset(p.name)} className="btn-danger btn-sm">
                      {t("channel.delete")}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("log.summary")}</h2>
          <p className="hint">{t("log.overviewHint")}</p>
        </div>
        <div className="stats-grid">
          <div className="stat-card">
            <div className="stat-label">{t("log.totalRecords")}</div>
            <div className="stat-value blue">{filtered.length}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">{t("log.successCount")}</div>
            <div className="stat-value green">{successCount}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">{t("log.failureCount")}</div>
            <div className="stat-value red">{failureCount}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">{t("log.avgDuration")}</div>
            <div className="stat-value">{formatDuration(avgDuration)}</div>
          </div>
          <div className="stat-card">
            <div className="stat-label">{t("log.avgTtft")}</div>
            <div className="stat-value">{avgTtft > 0 ? formatDuration(avgTtft) : "-"}</div>
          </div>
        </div>
      </div>

      <div className="tab-content log-panel section-card">
        <div className="log-section-header">
          <div>
            <h2>{t("log.listSection")}</h2>
            <p className="hint">{t("log.total", { count: filtered.length })}</p>
          </div>
        </div>

        {loading ? (
          <div className="loading-state">{t("common.loading")}</div>
        ) : loadError ? (
          <div className="form-error" role="alert">
            <div style={{ marginBottom: 8 }}>{loadError}</div>
            <button onClick={handleRetry} className="btn-secondary btn-sm">
              {t("log.retry")}
            </button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="empty-state">{t("log.noLogs")}</div>
        ) : (
          <div className="log-list">
            {filtered.map((log) => {
              const isExpanded = expandedRoute === log.request_id;
              const channelType = getChannelType(log);
              return (
                <div key={log.request_id} className={`log-list-item${isExpanded ? " is-expanded" : ""}`}>
                  <div className="log-list-row">
                    <div className="log-list-primary">
                      <div className="log-list-title-row">
                        <div className="log-card-title">{log.logical_model_name || "-"}</div>
                        <span className={`badge ${log.status === "success" ? "badge-green" : "badge-red"}`}>
                          {log.status === "success" ? t("log.success") : t("log.failure")}
                        </span>
                        <span className="log-channel-badge" style={{ color: channelTypeColors[channelType], borderColor: channelTypeColors[channelType] }}>
                          {getChannelTypeLabel(channelType)}
                        </span>
                        {log.mitm_target && (
                          <span
                            className="badge badge-blue"
                            title={`来源工具: ${log.mitm_target}｜请求模型: ${log.requested_model || "-"}｜改写目标: ${log.resolved_target || "直连"}｜规则: ${log.mitm_rule_id || "默认动作"}`}
                          >
                            [MITM] {log.mitm_target}
                          </span>
                        )}
                      </div>
                      <div className="log-list-meta">
                        <span>{formatDateTime(log.created_at)}</span>
                        <span>HTTP {log.http_status || "-"}</span>
                        <span>{t("log.ttft")} {log.ttft_ms ? formatDuration(log.ttft_ms) : "-"}</span>
                        <span>{t("log.tokens")} {(log.input_tokens || 0) + (log.output_tokens || 0)}</span>
                        <span>{log.channel_id || "-"}</span>
                        <span>{log.upstream_model || "-"}</span>
                        <code>{log.route_trace || "-"}</code>
                      </div>
                      {log.error_message && <div className="log-inline-error">{log.error_message}</div>}
                    </div>
                    <div className="log-list-side">
                      <span className="log-duration-pill">{formatDuration(log.duration_ms)}</span>
                      <div className="log-card-actions">
                        {/* P3-16：未命中规则的接管请求可一键创建映射规则 */}
                        {log.mitm_target && !log.resolved_target && (
                          <button
                            onClick={() => handleCreateRule(log)}
                            className="btn-secondary btn-sm"
                            title="按该请求的来源工具与模型创建一条 MITM 路由映射规则"
                          >
                            {t("log.createRule")}
                          </button>
                        )}
                        <button onClick={() => setExpandedRoute(isExpanded ? null : log.request_id)} className="btn-secondary btn-sm">
                          {isExpanded ? t("log.hideRoute") : t("log.route")}
                        </button>
                        {log.error_message && (
                          <>
                            <button onClick={() => handleCopyError(log, "text")} className="btn-secondary btn-sm">
                              {t("channel.copy")}
                            </button>
                            <button onClick={() => handleCopyError(log, "json")} className="btn-secondary btn-sm">
                              JSON
                            </button>
                            <button onClick={() => handleCopyError(log, "markdown")} className="btn-secondary btn-sm">
                              MD
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  </div>

                  {isExpanded && (
                    <div className="log-route-panel">
                      <div className="log-route-title">{t("log.stepByStepRoute")}</div>
                      {log.route_steps?.length ? (
                        <div className="log-route-list">
                          {log.route_steps.map((step, i) => (
                            <div key={`${log.request_id}-${i}`} className="log-route-step">
                              <div className="log-route-index">#{i + 1}</div>
                              <div className="log-route-content">
                                <div className="log-route-line">
                                  <span>{t("log.stepChannel")}：{step.channel_id}</span>
                                  <span>{t("log.stepModel")}：{step.upstream_model}</span>
                                  <span className={step.status === "success" ? "text-green" : "text-red"}>
                                    {step.status === "success" ? t("log.success") : t("log.failure")}
                                  </span>
                                </div>
                                {step.error && <div className="log-route-error">{t("log.stepError")}：{step.error}</div>}
                              </div>
                            </div>
                          ))}
                        </div>
                      ) : (
                        <div className="empty-hint">{t("log.noRouteDetails")}</div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
