import { useEffect, useState } from "react";
import { apiFetch } from "../utils/api";
import type { UsageRecord } from "../utils/types";
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  LineChart, Line, Legend,
} from "recharts";
import { useTranslation } from "../i18n";

interface ErrorDistItem {
  category: string;
  count: number;
}

interface TimelineItem {
  bucket: string;
  total_count: number;
  error_count: number;
  success_rate: number;
}

interface ByModelItem {
  model: string;
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_errors: number;
}

interface ByChannelItem {
  channel_id: string;
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_errors: number;
}

interface RecentErrorItem {
  user_id: string;
  channel_id: string;
  model: string;
  status_code: number;
  error_category: string;
  latency_ms: number;
  recorded_at: string;
}

type Tab = "records" | "errors" | "timeline" | "by-model" | "by-channel" | "recent-errors";

export function UsagePage() {
  const { t } = useTranslation();
  const [records, setRecords] = useState<UsageRecord[]>([]);
  const [err, setErr] = useState("");
  const [activeTab, setActiveTab] = useState<Tab>("records");
  const [errorDist, setErrorDist] = useState<ErrorDistItem[]>([]);
  const [timeline, setTimeline] = useState<TimelineItem[]>([]);
  const [timelinePeriod, setTimelinePeriod] = useState("24h");
  const [byModel, setByModel] = useState<ByModelItem[]>([]);
  const [byChannel, setByChannel] = useState<ByChannelItem[]>([]);
  const [recentErrors, setRecentErrors] = useState<RecentErrorItem[]>([]);
  const [loadingByModel, setLoadingByModel] = useState(false);
  const [loadingByChannel, setLoadingByChannel] = useState(false);
  const [loadingRecentErrors, setLoadingRecentErrors] = useState(false);

  const loading = <div className="loading">{t("admin.usageLoading")}</div>;
  const empty = (msg: string) => <div className="empty">{msg}</div>;

  useEffect(() => {
    apiFetch<{ data: UsageRecord[] }>("/usage/records")
      .then((res) => setRecords(res.data))
      .catch((e) => setErr(e.message));
  }, []);

  useEffect(() => {
    if (activeTab !== "errors") return;
    apiFetch<{ data: ErrorDistItem[] }>("/usage/error-distribution?period=month")
      .then((res) => setErrorDist(res.data || []))
      .catch(() => setErrorDist([]));
  }, [activeTab]);

  useEffect(() => {
    if (activeTab !== "timeline") return;
    const interval = timelinePeriod === "24h" ? "hour" : "day";
    apiFetch<{ data: TimelineItem[] }>(`/usage/timeline?period=${timelinePeriod}&interval=${interval}`)
      .then((res) => setTimeline(res.data || []))
      .catch(() => setTimeline([]));
  }, [activeTab, timelinePeriod]);

  useEffect(() => {
    if (activeTab !== "by-model") return;
    setLoadingByModel(true);
    apiFetch<{ data: ByModelItem[] }>("/usage/by-model")
      .then((res) => setByModel(res.data || []))
      .catch(() => setByModel([]))
      .finally(() => setLoadingByModel(false));
  }, [activeTab]);

  useEffect(() => {
    if (activeTab !== "by-channel") return;
    setLoadingByChannel(true);
    apiFetch<{ data: ByChannelItem[] }>("/usage/by-channel")
      .then((res) => setByChannel(res.data || []))
      .catch(() => setByChannel([]))
      .finally(() => setLoadingByChannel(false));
  }, [activeTab]);

  useEffect(() => {
    if (activeTab !== "recent-errors") return;
    setLoadingRecentErrors(true);
    apiFetch<{ data: RecentErrorItem[] }>("/usage/recent-errors?limit=50")
      .then((res) => setRecentErrors(res.data || []))
      .catch(() => setRecentErrors([]))
      .finally(() => setLoadingRecentErrors(false));
  }, [activeTab]);

  const MODEL_TAGS: Record<string, string> = {
    "glm-5": "open", "glm-5.1": "open",
    "deepseek-v4": "open", "deepseek-v4-pro": "open", "deepseek-v4-flash": "open",
    "deepseek-chat": "open", "deepseek-coder": "open",
    "kimi-2.5": "open", "kimi-2.6": "open",
    "minimax-m2.7": "open",
    "qwen-max": "open", "qwen-plus": "open",
    "llama": "open", "mistral": "open",
  };

  function getModelTag(model: string): string {
    for (const [prefix, tag] of Object.entries(MODEL_TAGS)) {
      if (model.startsWith(prefix)) return tag;
    }
    return "closed";
  }

  const errorCategoryName = (cat: string) => {
    const names: Record<string, string> = {
      rate_limit: "Rate Limit (429)",
      auth: "Auth (401/403)",
      model: "Model Not Found",
      context_length: "Context Length",
      server: "Server (5xx)",
      permanent: "Bad Request",
      unknown: "Other",
    };
    return names[cat] || cat || "Unknown";
  };

  const errorCategoryColor = (cat: string) => {
    const colors: Record<string, string> = {
      rate_limit: "#f59e0b",
      auth: "#ef4444",
      model: "#8b5cf6",
      context_length: "#3b82f6",
      server: "#f97316",
      permanent: "#6b7280",
      unknown: "#9ca3af",
    };
    return colors[cat] || "#9ca3af";
  };

  if (err) return <div className="error-msg">{err}</div>;

  const tab = (key: Tab, label: string) => (
    <button
      className={activeTab === key ? "btn btn-primary" : "btn btn-outline"}
      onClick={() => setActiveTab(key)}
    >{label}</button>
  );

  return (
    <>
      <h1 className="page-title">{t("admin.usagePage")}</h1>

      <div style={{ display: "flex", gap: 8, marginBottom: 20, flexWrap: "wrap" }}>
        {tab("records", t("admin.usageRecords"))}
        {tab("errors", t("admin.usageErrors"))}
        {tab("timeline", t("admin.usageTimeline"))}
        {tab("by-model", t("admin.usageByModel"))}
        {tab("by-channel", t("admin.usageByChannel"))}
        {tab("recent-errors", t("admin.usageRecentErrors"))}
      </div>

      {activeTab === "records" && (
        records.length === 0 ? empty(t("admin.usageEmpty")) : (
          <table>
            <thead>
              <tr>
                <th>{t("admin.usageUser")}</th>
                <th>{t("admin.usageChannel")}</th>
                <th>{t("admin.usageModel")}</th>
                <th>{t("admin.usageRequests")}</th>
                <th>{t("admin.usageInputTokens")}</th>
                <th>{t("admin.usageOutputTokens")}</th>
                <th>{t("admin.usageStatus")}</th>
                <th>{t("admin.usageTime")}</th>
              </tr>
            </thead>
            <tbody>
              {records.map((r) => (
                <tr key={r.record_id}>
                  <td style={{ fontFamily: "monospace", fontSize: 12 }}>{r.user_id.slice(0, 8)}</td>
                  <td style={{ fontSize: 12 }}>{r.channel_id.slice(0, 8)}</td>
                  <td>{r.model}</td>
                  <td>{r.request_count}</td>
                  <td>{r.input_tokens.toLocaleString()}</td>
                  <td>{r.output_tokens.toLocaleString()}</td>
                  <td>
                    {r.is_error ? <span className="badge badge-red">err</span> : <span className="badge badge-green">ok</span>}
                  </td>
                  <td>{new Date(r.recorded_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )
      )}

      {activeTab === "errors" && (
        errorDist.length === 0 ? empty(t("admin.usageEmptyErrors")) : (
          <div>
            <h2 style={{ fontSize: 16, marginBottom: 12 }}>{t("admin.usageTitleErrorDist")}</h2>
            <div style={{ width: "100%", height: 300 }}>
              <ResponsiveContainer>
                <BarChart data={errorDist}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="category" tickFormatter={errorCategoryName} />
                  <YAxis />
                  <Tooltip formatter={(value: any) => [Number(value), t("admin.usageCount")]} labelFormatter={(label: any) => errorCategoryName(String(label))} />
                  <Bar dataKey="count" fill="#3b82f6" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
            <table style={{ marginTop: 16 }}>
              <thead>
                <tr>
                  <th>{t("admin.usageErrorType")}</th>
                  <th>{t("admin.usageCount")}</th>
                </tr>
              </thead>
              <tbody>
                {errorDist.map((item) => (
                  <tr key={item.category}>
                    <td><span style={{ color: errorCategoryColor(item.category), fontWeight: 600 }}>●</span> {errorCategoryName(item.category)}</td>
                    <td>{item.count.toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      )}

      {activeTab === "timeline" && (
        <div>
          <div style={{ display: "flex", gap: 8, marginBottom: 16, alignItems: "center" }}>
            <h2 style={{ fontSize: 16, margin: 0 }}>{t("admin.usageTitleTimeline")}</h2>
            <select
              value={timelinePeriod}
              onChange={(e) => setTimelinePeriod(e.target.value)}
              style={{ marginLeft: 12, padding: "4px 8px", borderRadius: 4, border: "1px solid #ccc" }}
            >
              <option value="24h">{t("admin.usageTimeline24h")}</option>
              <option value="7d">{t("admin.usageTimeline7d")}</option>
              <option value="30d">{t("admin.usageTimeline30d")}</option>
            </select>
          </div>
          {timeline.length === 0 ? empty(t("admin.usageEmptyTimeline")) : (
            <div style={{ width: "100%", height: 300 }}>
              <ResponsiveContainer>
                <LineChart data={timeline}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="bucket" tick={{ fontSize: 11 }} />
                  <YAxis />
                  <Tooltip />
                  <Legend />
                  <Line type="monotone" dataKey="total_count" stroke="#3b82f6" name="总请求" strokeWidth={2} />
                  <Line type="monotone" dataKey="error_count" stroke="#ef4444" name="错误数" strokeWidth={2} />
                </LineChart>
              </ResponsiveContainer>
            </div>
          )}
        </div>
      )}

      {activeTab === "by-model" && (
        loadingByModel ? loading : (
          byModel.length === 0 ? empty(t("admin.usageEmptyModel")) : (
            <div>
              <h2 style={{ fontSize: 16, marginBottom: 12 }}>{t("admin.usageTitleModelAgg")}</h2>
              <div style={{ width: "100%", height: 300, marginBottom: 16 }}>
                <ResponsiveContainer>
                  <BarChart data={byModel}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="model" tick={{ fontSize: 10 }} />
                    <YAxis />
                    <Tooltip />
                    <Legend />
                    <Bar dataKey="total_requests" fill="#3b82f6" radius={[4, 4, 0, 0]} name={t("admin.usageRequests")} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
              <table>
                <thead>
                  <tr>
                    <th>{t("admin.usageModel")}</th>
                    <th>{t("admin.usageRequests")}</th>
                    <th>{t("admin.usageInputTokens")}</th>
                    <th>{t("admin.usageOutputTokens")}</th>
                    <th>{t("admin.usageErrorsCount")}</th>
                  </tr>
                </thead>
                <tbody>
                  {byModel.map((item) => (
                    <tr key={item.model}>
                      <td>
                        {item.model}
                        <span style={{
                          display: "inline-block",
                          padding: "1px 6px",
                          borderRadius: 4,
                          fontSize: 10,
                          fontWeight: 600,
                          marginLeft: 6,
                          background: getModelTag(item.model) === "open" ? "rgba(34,197,94,0.15)" : "rgba(59,130,246,0.15)",
                          color: getModelTag(item.model) === "open" ? "#22c55e" : "#3b82f6",
                        }}>
                          {getModelTag(item.model) === "open" ? t("admin.usageOpenSource") : t("admin.usageClosedSource")}
                        </span>
                      </td>
                      <td>{item.total_requests.toLocaleString()}</td>
                      <td>{item.total_input_tokens.toLocaleString()}</td>
                      <td>{item.total_output_tokens.toLocaleString()}</td>
                      <td>{item.total_errors > 0 ? <span className="badge badge-red">{item.total_errors}</span> : item.total_errors}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        )
      )}

      {activeTab === "by-channel" && (
        loadingByChannel ? loading : (
          byChannel.length === 0 ? empty(t("admin.usageEmptyChannel")) : (
            <div>
              <h2 style={{ fontSize: 16, marginBottom: 12 }}>{t("admin.usageTitleChannelAgg")}</h2>
              <div style={{ width: "100%", height: 300, marginBottom: 16 }}>
                <ResponsiveContainer>
                  <BarChart data={byChannel}>
                    <CartesianGrid strokeDasharray="3 3" />
                    <XAxis dataKey="channel_id" tick={{ fontSize: 10 }} />
                    <YAxis />
                    <Tooltip />
                    <Legend />
                    <Bar dataKey="total_requests" fill="#10b981" radius={[4, 4, 0, 0]} name={t("admin.usageRequests")} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
              <table>
                <thead>
                  <tr>
                    <th>{t("admin.usageChannelId")}</th>
                    <th>{t("admin.usageRequests")}</th>
                    <th>{t("admin.usageInputTokens")}</th>
                    <th>{t("admin.usageOutputTokens")}</th>
                    <th>{t("admin.usageErrorsCount")}</th>
                  </tr>
                </thead>
                <tbody>
                  {byChannel.map((item) => (
                    <tr key={item.channel_id}>
                      <td style={{ fontFamily: "monospace", fontSize: 12 }}>{item.channel_id.slice(0, 12)}</td>
                      <td>{item.total_requests.toLocaleString()}</td>
                      <td>{item.total_input_tokens.toLocaleString()}</td>
                      <td>{item.total_output_tokens.toLocaleString()}</td>
                      <td>{item.total_errors > 0 ? <span className="badge badge-red">{item.total_errors}</span> : item.total_errors}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        )
      )}

      {activeTab === "recent-errors" && (
        loadingRecentErrors ? loading : (
          recentErrors.length === 0 ? empty(t("admin.usageEmptyRecentErrors")) : (
            <div>
              <h2 style={{ fontSize: 16, marginBottom: 12 }}>{t("admin.usageTitleRecentErrors")}</h2>
              <table>
                <thead>
                  <tr>
                    <th>{t("admin.usageUser")}</th>
                    <th>{t("admin.usageChannel")}</th>
                    <th>{t("admin.usageModel")}</th>
                    <th>{t("admin.usageStatusCode")}</th>
                    <th>{t("admin.usageErrorCategory")}</th>
                    <th>{t("admin.usageLatency")}</th>
                    <th>{t("admin.usageTime")}</th>
                  </tr>
                </thead>
                <tbody>
                  {recentErrors.map((item, i) => (
                    <tr key={i}>
                      <td style={{ fontFamily: "monospace", fontSize: 12 }}>{item.user_id.slice(0, 8)}</td>
                      <td style={{ fontSize: 12 }}>{item.channel_id.slice(0, 8)}</td>
                      <td>{item.model}</td>
                      <td><span className="badge badge-red">{item.status_code}</span></td>
                      <td><span style={{ color: errorCategoryColor(item.error_category), fontWeight: 600 }}>●</span> {errorCategoryName(item.error_category)}</td>
                      <td>{item.latency_ms}ms</td>
                      <td>{new Date(item.recorded_at).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        )
      )}
    </>
  );
}