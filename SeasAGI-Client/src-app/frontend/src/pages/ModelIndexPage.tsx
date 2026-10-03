import { useEffect, useMemo, useState } from "react";
import { fetchModelIndex } from "../utils/commands";
import type { ModelIndexData, ModelIndexEntry } from "../utils/types";
import { useTranslation } from "../i18n";

const CATEGORIES = [
  { value: "overall", key: "modelIndex.tab.overall" },
  { value: "coding", key: "modelIndex.tab.coding" },
  { value: "reasoning", key: "modelIndex.tab.reasoning" },
  { value: "knowledge", key: "modelIndex.tab.knowledge" },
  { value: "professional", key: "modelIndex.tab.professional" },
] as const;

type SortMode = "score" | "input_price" | "output_price";

// 排名徽标配色：金 / 银 / 铜（TOP 3）
const RANK_COLORS: Record<number, string> = {
  1: "#f5c542",
  2: "#c0c8d4",
  3: "#cd8b5c",
};

const fmtPrice = (v: number) => (v > 0 ? `¥${v}` : "-");
const fmtDate = (v: string) => (v ? v.slice(0, 10) : "-");

// 防脏数据：entries 非数组时返回空数组，避免渲染崩溃白屏。
const normalizeEntries = (data: unknown): ModelIndexEntry[] => {
  if (!data || typeof data !== "object") return [];
  const entries = (data as { entries?: unknown }).entries;
  return Array.isArray(entries) ? (entries as ModelIndexEntry[]) : [];
};

export function ModelIndexPage() {
  const { t } = useTranslation();
  const [category, setCategory] = useState<string>("overall");
  const [sortMode, setSortMode] = useState<SortMode>("score");
  const [data, setData] = useState<ModelIndexData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setError(null);
      try {
        const raw = await fetchModelIndex(category);
        if (cancelled) return;
        if (raw && typeof raw.error === "string" && raw.error) {
          throw new Error(raw.error);
        }
        setData({
          category,
          updated_at: typeof raw?.updated_at === "string" ? raw.updated_at : "",
          entries: normalizeEntries(raw),
        });
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [category, refreshKey]);

  const entries = useMemo(() => {
    const list = [...(data?.entries ?? [])];
    if (sortMode === "input_price") {
      list.sort((a, b) => (a.input_price || Infinity) - (b.input_price || Infinity));
    } else if (sortMode === "output_price") {
      list.sort((a, b) => (a.output_price || Infinity) - (b.output_price || Infinity));
    } else {
      list.sort((a, b) => (a.rank || 0) - (b.rank || 0));
    }
    return list;
  }, [data, sortMode]);

  const stats = useMemo(() => {
    const list = data?.entries ?? [];
    const providers = new Set(list.map((e) => e.provider).filter(Boolean));
    return {
      models: list.length,
      providers: providers.size,
      updated: fmtDate(data?.updated_at ?? ""),
    };
  }, [data]);

  const evidenceLabel = (v: string) =>
    v === "sufficient"
      ? t("modelIndex.evidence.sufficient")
      : v === "accumulating"
        ? t("modelIndex.evidence.accumulating")
        : v === "sensitive"
          ? t("modelIndex.evidence.sensitive")
          : v || "-";

  return (
    <div className="page-container model-index-page">
      <div className="page-header">
        <div>
          <h1>{t("nav.modelIndex")}</h1>
          <p className="page-subtitle">{t("modelIndex.subtitle")}</p>
        </div>
        <div className="page-header-actions">
          <button className="btn btn-secondary" onClick={() => setRefreshKey((k) => k + 1)} disabled={loading}>
            {t("modelIndex.refresh")}
          </button>
        </div>
      </div>

      <div className="token-market-hero">
        <div className="token-market-hero-main">
          <span className="token-market-kicker">{t("modelIndex.kicker")}</span>
          <h2>{t("modelIndex.heroTitle")}</h2>
          <p>{t("modelIndex.heroDescription")}</p>
        </div>
        <div className="token-market-stats">
          <div className="token-market-stat-card">
            <span className="token-market-stat-label">{t("modelIndex.metricModels")}</span>
            <strong>{stats.models}</strong>
          </div>
          <div className="token-market-stat-card">
            <span className="token-market-stat-label">{t("modelIndex.metricProviders")}</span>
            <strong>{stats.providers}</strong>
          </div>
          <div className="token-market-stat-card">
            <span className="token-market-stat-label">{t("modelIndex.metricUpdated")}</span>
            <strong>{stats.updated || "-"}</strong>
          </div>
        </div>
      </div>

      <div className="token-market-free-tabs" role="tablist" aria-label={t("nav.modelIndex")}>
        {CATEGORIES.map((c) => (
          <button
            key={c.value}
            className={`free-tab ${category === c.value ? "active" : ""}`}
            onClick={() => setCategory(c.value)}
          >
            {t(c.key)}
          </button>
        ))}
      </div>

      <div className="token-market-filters" style={{ display: "flex", gap: "12px", alignItems: "center", marginBottom: "12px" }}>
        <span className="token-market-mini-label">{t("modelIndex.sortBy")}</span>
        <div className="token-market-view-switch" role="group">
          {([
            { value: "score", label: t("modelIndex.sortScore") },
            { value: "input_price", label: t("modelIndex.sortInput") },
            { value: "output_price", label: t("modelIndex.sortOutput") },
          ] as Array<{ value: SortMode; label: string }>).map((s) => (
            <button
              key={s.value}
              type="button"
              className={`token-market-view-btn${sortMode === s.value ? " is-active" : ""}`}
              onClick={() => setSortMode(s.value)}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>

      {loading && <div className="loading-state">{t("common.loading")}</div>}
      {error && <div className="form-error">{t("modelIndex.loadFailed")}: {error}</div>}

      {!loading && !error && entries.length === 0 && (
        <div className="empty-state">
          <div className="empty-icon">🏆</div>
          <p>{t("modelIndex.empty")}</p>
        </div>
      )}

      {!loading && !error && entries.length > 0 && (
        <div className="token-market-table-wrap">
          <table className="token-market-table">
            <thead>
              <tr>
                <th>{t("modelIndex.col.rank")}</th>
                <th>{t("modelIndex.col.model")}</th>
                <th>{t("modelIndex.col.release")}</th>
                <th>{t("modelIndex.col.evidence")}</th>
                <th>{t("modelIndex.col.input")}</th>
                <th>{t("modelIndex.col.output")}</th>
                <th>{t("modelIndex.col.score")}</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <tr key={`${e.category}-${e.id}`} style={e.rank <= 3 && sortMode === "score" ? { background: "rgba(245,197,66,0.05)" } : undefined}>
                  <td>
                    {e.rank <= 3 ? (
                      <span
                        className="mi-rank-badge"
                        style={{
                          display: "inline-flex",
                          alignItems: "center",
                          justifyContent: "center",
                          width: "26px",
                          height: "26px",
                          borderRadius: "50%",
                          color: "#0f172a",
                          fontWeight: 700,
                          fontSize: "13px",
                          background: RANK_COLORS[e.rank],
                        }}
                      >
                        {e.rank}
                      </span>
                    ) : (
                      <span style={{ opacity: 0.7 }}>{e.rank}</span>
                    )}
                  </td>
                  <td>
                    <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                      {e.provider_logo ? (
                        <img
                          src={e.provider_logo}
                          alt=""
                          style={{ width: "22px", height: "22px", borderRadius: "6px", objectFit: "cover" }}
                          onError={(ev) => {
                            (ev.target as HTMLImageElement).style.display = "none";
                          }}
                        />
                      ) : null}
                      <div>
                        <div style={{ fontWeight: 600 }}>{e.model_name}</div>
                        <div style={{ fontSize: "12px", opacity: 0.65 }}>{e.provider || "-"}</div>
                      </div>
                    </div>
                  </td>
                  <td>{fmtDate(e.release_date)}</td>
                  <td>
                    <span className="status-badge status-active">{evidenceLabel(e.evidence_status)}</span>
                  </td>
                  <td>{fmtPrice(e.input_price)}</td>
                  <td>{fmtPrice(e.output_price)}</td>
                  <td>
                    <strong>{e.consensus_score > 0 ? e.consensus_score.toFixed(1) : "-"}</strong>
                    {e.eval_count > 0 && (
                      <span style={{ fontSize: "12px", opacity: 0.6, marginLeft: "6px" }}>
                        {t("modelIndex.evalCount", { count: e.eval_count })}
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {!loading && !error && entries.length > 0 && (
        <div style={{ marginTop: "16px", fontSize: "12px", opacity: 0.65, lineHeight: 1.8 }}>
          <p>{t("modelIndex.footer.scoreNote")}</p>
          <p>{t("modelIndex.footer.priceNote")}</p>
        </div>
      )}
    </div>
  );
}
