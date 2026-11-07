import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  platformRequest,
  fetchFreeChannels,
  listChannels,
  saveCustomChannel,
  describePlatformError,
  getPlatformArray,
  getPlatformData,
} from "../utils/commands";
import type { Channel } from "../utils/types";
import { useTranslation } from "../i18n";
import { useMarketStore } from "../stores/marketStore";
import { useAppStore } from "../stores/appStore";

interface FreeChannel {
  provider_id: string;
  display_name: string;
  auth_type: string;
  base_url: string;
  free_type: string;
  monthly_tokens: string;
  models: Array<{ model_id: string; model_name: string; capability: string }>;
  description: string;
}

type MarketViewMode = "card" | "list";
type FreeFilterTab = "all" | "keyless" | "recurring-monthly" | "one-time-initial";

// 过滤缺少 models 字段的脏数据（如错误对象），避免渲染时 ch.models.length 崩溃白屏。
const normalizeFreeChannels = (data: unknown): FreeChannel[] => {
  if (!Array.isArray(data)) return [];
  return data.filter(
    (ch): ch is FreeChannel => !!ch && Array.isArray((ch as FreeChannel).models),
  );
};

export function TokenMarketPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const marketListings = useMarketStore((s) => s.marketListings);
  const setMarketListings = useMarketStore((s) => s.setMarketListings);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [total, setTotal] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [freeChannels, setFreeChannels] = useState<FreeChannel[]>([]);
  const [freeFilterTab, setFreeFilterTab] = useState<FreeFilterTab>("all");
  const [viewMode, setViewMode] = useState<MarketViewMode>("card");
  const setChannels = useAppStore((s) => s.setChannels);
  const isLoggedIn = useAppStore((s) => s.auth.is_logged_in);
  // 免费通道导入状态
  const [importingId, setImportingId] = useState<string | null>(null);
  const [importMsg, setImportMsg] = useState<{ ok: boolean; text: string } | null>(null);
  // F7: Free channel cache from store
  const freeChannelsCache = useMarketStore((s) => s.freeChannelsCache);
  const freeChannelsCacheTime = useMarketStore((s) => s.freeChannelsCacheTime);
  const setFreeChannelsCache = useMarketStore((s) => s.setFreeChannelsCache);
  // P11: 搜索/筛选/排序
  const [search, setSearch] = useState("");
  const [minPrice, setMinPrice] = useState("");
  const [maxPrice, setMaxPrice] = useState("");
  const [saleType, setSaleType] = useState("");
  const [protocolFilter, setProtocolFilter] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [onlyCapacity, setOnlyCapacity] = useState(false);
  const [sortBy, setSortBy] = useState("newest");
  const [filterKey, setFilterKey] = useState(0);
  // 我的市场余额（登录后展示；null 表示不可用/未登录）
  const [myBalance, setMyBalance] = useState<number | null>(null);
  // 合规与费率公告（P3-4）：由平台下发，未配置时展示默认合规提示
  const [marketNotice, setMarketNotice] = useState<{ text: string; feeRate: number } | null>(null);

  const fetchNotice = async () => {
    try {
      const resp = await platformRequest("GET", "/token-market/notice");
      if (resp.status >= 400) return;
      const data = getPlatformData<Record<string, any>>(resp.body);
      if (!data) return;
      setMarketNotice({
        text: String(data.compliance_notice || ""),
        feeRate: typeof data.relay_fee_rate === "number" ? data.relay_fee_rate : 0.05,
      });
    } catch {
      // 静默失败：公告不可用不影响市场浏览
    }
  };

  const fetchAccount = async () => {
    if (!isLoggedIn) {
      setMyBalance(null);
      return;
    }
    try {
      const resp = await platformRequest("GET", "/token-market/account");
      if (resp.status >= 400) return;
      const data = getPlatformData<{ balance?: number }>(resp.body);
      if (typeof data?.balance === "number") setMyBalance(data.balance);
    } catch {
      // 静默失败：余额展示失败不影响市场浏览
    }
  };

  // 请求序号：仅最后一次发起的请求可写入状态，丢弃过期响应（竞态防护）。
  const requestSeq = useRef(0);

  const fetchListings = async (pageNum: number, append: boolean) => {
    const reqId = ++requestSeq.current;
    if (append) {
      setLoadingMore(true);
    } else {
      setLoading(true);
      setLoadingMore(false);
    }
    setError(null);
    try {
      const params = new URLSearchParams();
      params.set("page", String(pageNum));
      params.set("page_size", "20");
      if (search) params.set("search", search);
      if (minPrice) params.set("min_price", minPrice);
      if (maxPrice) params.set("max_price", maxPrice);
      if (saleType) params.set("sale_type", saleType);
      if (protocolFilter) params.set("wire_protocol", protocolFilter);
      if (modelFilter.trim()) params.set("model", modelFilter.trim());
      if (onlyCapacity) params.set("has_capacity", "1");
      params.set("sort", sortBy);
      const resp = await platformRequest("GET", `/token-market/listings?${params}`);
      // 过期响应（筛选变更/更晚的请求已发出）直接丢弃，避免旧数据覆盖新结果。
      if (reqId !== requestSeq.current) return;
      if (resp.status >= 400) throw new Error(describePlatformError(resp));
      const body = resp.body as Record<string, any>;
      const newItems = getPlatformArray(body);
      setTotal(typeof body.total === "number" ? body.total : 0);
      setTotalPages(typeof body.total_pages === "number" ? body.total_pages : 1);
      // 函数式更新，避免基于闭包中的旧列表拼接导致丢页/重复。
      setMarketListings((prev) => (append ? [...prev, ...newItems] : newItems));
      // 页码在请求成功后再推进，失败时保持原页码以便重试。
      setPage(pageNum);
    } catch (err) {
      if (reqId !== requestSeq.current) return;
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (reqId === requestSeq.current) {
        setLoading(false);
        setLoadingMore(false);
      }
    }
  };

  useEffect(() => {
    fetchListings(1, false);
    fetchAccount();
    fetchNotice();
    // F7: Use cached free channels if within 5 minutes.
    // 空列表不命中缓存：避免未登录时缓存空结果、登录后 5 分钟内仍显示为空。
    const now = Date.now();
    const cacheAge = now - (freeChannelsCacheTime ?? 0);
    if (freeChannelsCache && freeChannelsCache.length > 0 && cacheAge < 5 * 60 * 1000) {
      setFreeChannels(normalizeFreeChannels(freeChannelsCache));
    } else {
      (async () => {
        try {
          const data = await fetchFreeChannels();
          const normalized = normalizeFreeChannels(data);
          setFreeChannels(normalized);
          if (normalized.length > 0) {
            setFreeChannelsCache(data as unknown[]);
          }
        } catch {
          setFreeChannels([]);
        }
      })();
    }
  }, [filterKey]);

  const handleSearch = () => {
    setPage(1);
    setFilterKey((k) => k + 1);
  };

  const handleLoadMore = () => {
    if (loadingMore || loading || page >= totalPages) return;
    // 页码在 fetchListings 成功后推进，避免失败后页码错位。
    fetchListings(page + 1, true);
  };

  const marketStats = useMemo(() => {
    const activeCount = marketListings.filter((item) => item.status === "active").length;
    const fixedCount = marketListings.filter((item) => item.sale_type === "fixed_price").length;
    const avgPrice =
      marketListings.length > 0
        ? marketListings.reduce((sum, item) => sum + (item.sale_type === "fixed_price" ? item.price : 0), 0) /
          Math.max(fixedCount, 1)
        : 0;
    return { activeCount, fixedCount, avgPrice };
  }, [marketListings]);

  const filteredFreeChannels = useMemo(() => {
    if (freeFilterTab === "all") return freeChannels;
    return freeChannels.filter((ch) => ch.free_type === freeFilterTab);
  }, [freeChannels, freeFilterTab]);

  const freeStats = useMemo(() => {
    const keyless = freeChannels.filter((ch) => ch.free_type === "keyless").length;
    const recurring = freeChannels.filter((ch) => ch.free_type === "recurring-monthly").length;
    const oneTime = freeChannels.filter((ch) => ch.free_type === "one-time-initial").length;
    const totalModels = freeChannels.reduce((sum, ch) => sum + ch.models.length, 0);
    return { keyless, recurring, oneTime, totalModels };
  }, [freeChannels]);

  // 免费通道导入：直接创建当前用户的自定义通道，保存后即可在"通道管理"中使用。
  // noauth（免认证）通道导入即启用；apikey/web-cookie/oauth 通道导入后保持停用，
  // 待用户在"通道管理"中补充凭据并启用。
  const handleImportFreeChannel = async (ch: FreeChannel) => {
    if (importingId) return;
    setImportMsg(null);
    setImportingId(ch.provider_id);
    try {
      const current = await listChannels();
      const duplicated = current.find(
        (c) => c.channel_type !== "platform" && c.provider_type === ch.provider_id && c.base_url === ch.base_url,
      );
      if (duplicated) {
        setChannels(current);
        setImportMsg({ ok: false, text: t("tokenMarket.alreadyImported") });
        return;
      }
      const channel: Channel = {
        channel_id: "",
        channel_type: "custom",
        provider_type: ch.provider_id,
        display_name: ch.display_name,
        base_url: ch.base_url,
        enabled: ch.auth_type === "noauth",
        health_status: "unknown",
        provider_specific_config: { auth_type: ch.auth_type, free_type: ch.free_type },
        models: ch.models.map((m) => m.model_id),
        api_key: "",
      };
      await saveCustomChannel(channel);
      const refreshed = await listChannels();
      setChannels(refreshed);
      setImportMsg({
        ok: true,
        text:
          ch.auth_type === "noauth"
            ? t("tokenMarket.importSuccess", { name: ch.display_name })
            : t("tokenMarket.importNeedKey", { name: ch.display_name }),
      });
    } catch (e) {
      setImportMsg({
        ok: false,
        text: `${t("tokenMarket.importFailed")}: ${e instanceof Error ? e.message : String(e)}`,
      });
    } finally {
      setImportingId(null);
    }
  };

  return (
    <div className="page-container token-market-page">
      <div className="page-header token-market-header">
        <div>
          <h1>{t("nav.tokenMarket")}</h1>
          <p className="page-subtitle">{t("tokenMarket.subtitle")}</p>
        </div>
        <div className="page-header-actions token-market-header-actions">
          <button className="btn btn-primary" onClick={() => navigate("/token-market/create")}>
            {t("tokenMarket.createListing")}
          </button>
          <button className="btn btn-secondary" onClick={() => navigate("/token-market/my-listings")}>
            {t("tokenMarket.myListings")}
          </button>
          <button className="btn btn-secondary" onClick={() => navigate("/token-market/my-subscriptions")}>
            {t("tokenMarket.mySubscriptions")}
          </button>
          <button className="btn btn-secondary" onClick={() => navigate("/token-market/provider-earnings")}>
            {t("tokenMarket.providerEarnings")}
          </button>
        </div>
      </div>

      {marketNotice && (
        <div className="token-market-section" style={{ padding: "12px 16px", marginBottom: "16px" }}>
          <strong>{t("tokenMarket.noticeTitle")}</strong>
          <p style={{ margin: "6px 0" }}>
            {marketNotice.text || t("tokenMarket.noticeComplianceFallback")}
          </p>
          <p className="hint" style={{ margin: 0 }}>
            {t("tokenMarket.noticeFee", { rate: (marketNotice.feeRate * 100).toFixed(0) })}
          </p>
        </div>
      )}

      <div className="token-market-hero">
        <div className="token-market-hero-main">
          <span className="token-market-kicker">{t("tokenMarket.marketplace")}</span>
          <h2>{t("tokenMarket.marketTitle")}</h2>
          <p>{t("tokenMarket.marketDescription")}</p>
        </div>
        <div className="token-market-stats">
          {myBalance !== null && (
            <div className="token-market-stat-card token-market-balance-card">
              <span className="token-market-stat-label">{t("tokenMarket.myBalance")}</span>
              <strong>${myBalance.toFixed(2)}</strong>
            </div>
          )}
          <div className="token-market-stat-card">
            <span className="token-market-stat-label">{t("tokenMarket.marketActive")}</span>
            <strong>{marketStats.activeCount}</strong>
          </div>
          <div className="token-market-stat-card">
            <span className="token-market-stat-label">{t("tokenMarket.marketFixedPrice")}</span>
            <strong>{marketStats.fixedCount}</strong>
          </div>
          <div className="token-market-stat-card">
            <span className="token-market-stat-label">{t("tokenMarket.marketAvgPrice")}</span>
            <strong>{marketStats.avgPrice > 0 ? marketStats.avgPrice.toFixed(2) : "-"}</strong>
          </div>
        </div>
      </div>

      {freeChannels.length > 0 && (
        <div className="token-market-section token-market-free-section">
          <div className="token-market-section-header">
            <div>
              <h2>{t("tokenMarket.freeChannels")}</h2>
              <p>{t("tokenMarket.freeChannelsHint")}</p>
            </div>
            <div className="token-market-free-stats">
              <span className="free-stat-badge free-stat-keyless">{t("tokenMarket.keyless")}: {freeStats.keyless}</span>
              <span className="free-stat-badge free-stat-recurring">{t("tokenMarket.recurringMonthly")}: {freeStats.recurring}</span>
              <span className="free-stat-badge free-stat-onetime">{t("tokenMarket.oneTimeInitial")}: {freeStats.oneTime}</span>
              <span className="free-stat-badge free-stat-models">{t("tokenMarket.freeModels")}: {freeStats.totalModels}</span>
            </div>
          </div>
          <div className="token-market-free-tabs">
            {(["all", "keyless", "recurring-monthly", "one-time-initial"] as FreeFilterTab[]).map((tab) => (
              <button
                key={tab}
                className={`free-tab ${freeFilterTab === tab ? "active" : ""}`}
                onClick={() => setFreeFilterTab(tab)}
              >
                {tab === "all" ? t("tokenMarket.all") : tab === "keyless" ? t("tokenMarket.keyless") : tab === "recurring-monthly" ? t("tokenMarket.recurringMonthly") : t("tokenMarket.oneTimeInitial")}
              </button>
            ))}
          </div>
          {importMsg && (
            <div className={importMsg.ok ? "form-success" : "form-error"}>{importMsg.text}</div>
          )}
          <div className="token-market-free-grid">
            {filteredFreeChannels.map((ch) => (
              <div key={ch.provider_id} className="token-market-free-card">
                <div className="token-market-free-card-top">
                  <div>
                    <div className="token-market-free-title">{ch.display_name}</div>
                    <div className="token-market-free-url">{ch.base_url}</div>
                  </div>
                  <span className={`status-badge auth-${ch.auth_type}`}>
                    {ch.auth_type === "noauth"
                      ? t("tokenMarket.authNoauth")
                      : ch.auth_type === "oauth"
                        ? t("tokenMarket.authOauth")
                        : ch.auth_type === "apikey"
                          ? t("tokenMarket.authApikey")
                          : t("tokenMarket.authWebCookie")}
                  </span>
                </div>
                <div className="token-market-free-meta">
                  <span>{t("tokenMarket.freeType")}: {ch.free_type}</span>
                  <span>{t("tokenMarket.monthlyTokens")}: {ch.monthly_tokens}</span>
                </div>
                {ch.models.length > 0 && (
                  <div className="token-market-tags">
                    {ch.models.map((model) => (
                      <span key={model.model_id} className="token-market-tag">{model.model_name}</span>
                    ))}
                  </div>
                )}
                <p className="token-market-free-desc">{ch.description}</p>
                <button
                  className="btn btn-sm btn-outline free-import-btn"
                  onClick={() => handleImportFreeChannel(ch)}
                  disabled={importingId === ch.provider_id}
                >
                  {importingId === ch.provider_id ? t("tokenMarket.importing") : t("tokenMarket.importAsChannel")}
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {loading && <div className="loading-state">{t("common.loading")}</div>}
      {error && <div className="form-error">{error}</div>}

      {!loading && !error && (
        <div className="token-market-filters" style={{ display: "flex", gap: "12px", flexWrap: "wrap", alignItems: "flex-end", marginBottom: "16px" }}>
          <div className="filter-field">
            <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.search") || "Search"}</label>
            <input
              type="text"
              className="form-input"
              placeholder={t("tokenMarket.searchPlaceholder") || "Token name or seller"}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleSearch()}
              style={{ minWidth: "200px" }}
            />
          </div>
          <div className="filter-field">
            <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.minPrice") || "Min Price"}</label>
            <input
              type="number"
              className="form-input"
              placeholder="0"
              value={minPrice}
              onChange={(e) => setMinPrice(e.target.value)}
              style={{ width: "100px" }}
            />
          </div>
          <div className="filter-field">
            <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.maxPrice") || "Max Price"}</label>
            <input
              type="number"
              className="form-input"
              placeholder="9999"
              value={maxPrice}
              onChange={(e) => setMaxPrice(e.target.value)}
              style={{ width: "100px" }}
            />
          </div>
          <div className="filter-field">
            <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.saleType") || "Type"}</label>
            <select className="form-input" value={saleType} onChange={(e) => setSaleType(e.target.value)} style={{ width: "130px" }}>
              <option value="">{t("tokenMarket.all") || "All"}</option>
              <option value="subscription">{t("tokenMarket.saleTypeSubscription")}</option>
              <option value="fixed_price">{t("tokenMarket.fixedPrice")}</option>
              <option value="discount">{t("tokenMarket.discount")}</option>
            </select>
          </div>
          <div className="filter-field">
            <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.filterProtocol")}</label>
            <select className="form-input" value={protocolFilter} onChange={(e) => setProtocolFilter(e.target.value)} style={{ width: "150px" }}>
              <option value="">{t("tokenMarket.any")}</option>
              <option value="openai-chat">openai-chat</option>
              <option value="anthropic">anthropic</option>
              <option value="openai-responses">openai-responses</option>
              <option value="gemini">gemini</option>
            </select>
          </div>
          <div className="filter-field">
            <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.filterModel")}</label>
            <input
              type="text"
              className="form-input"
              placeholder="claude-sonnet-4-5"
              value={modelFilter}
              onChange={(e) => setModelFilter(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleSearch()}
              style={{ width: "180px" }}
            />
          </div>
          <label className="filter-field" style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 13 }}>
            <input type="checkbox" checked={onlyCapacity} onChange={(e) => setOnlyCapacity(e.target.checked)} />
            {t("tokenMarket.filterCapacity")}
          </label>
          <div className="filter-field">
            <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.sortBy") || "Sort"}</label>
            <select className="form-input" value={sortBy} onChange={(e) => { setSortBy(e.target.value); handleSearch(); }} style={{ width: "140px" }}>
              <option value="newest">{t("tokenMarket.sortNewest") || "Newest"}</option>
              <option value="price_asc">{t("tokenMarket.sortPriceAsc") || "Price: Low to High"}</option>
              <option value="price_desc">{t("tokenMarket.sortPriceDesc") || "Price: High to Low"}</option>
              <option value="quota_desc">{t("tokenMarket.sortQuotaDesc") || "Quota: High to Low"}</option>
            </select>
          </div>
          <button className="btn btn-primary" onClick={handleSearch}>{t("tokenMarket.applyFilters") || "Apply"}</button>
        </div>
      )}

      {!loading && !error && marketListings.length === 0 && (
        <div className="empty-state">
          <div className="empty-icon">🛒</div>
          <p>{t("tokenMarket.emptyMarket")}</p>
        </div>
      )}

      {!loading && !error && marketListings.length > 0 && (
        <div className="token-market-section">
          <div className="token-market-section-header">
            <div>
              <h2>{t("tokenMarket.listingsTitle")}</h2>
              <p>{t("tokenMarket.listingsSubtitle", { count: total || marketListings.length })}</p>
            </div>
            <div className="token-market-view-switch" role="tablist" aria-label={t("tokenMarket.viewMode")}>
              <button
                type="button"
                className={`token-market-view-btn${viewMode === "card" ? " is-active" : ""}`}
                onClick={() => setViewMode("card")}
              >
                {t("tokenMarket.cardView")}
              </button>
              <button
                type="button"
                className={`token-market-view-btn${viewMode === "list" ? " is-active" : ""}`}
                onClick={() => setViewMode("list")}
              >
                {t("tokenMarket.listView")}
              </button>
            </div>
          </div>

          {viewMode === "card" ? (
            <div className="token-market-card-grid">
              {marketListings.map((listing) => (
                <div
                  key={listing.listing_id}
                  className="token-market-listing-card"
                  role="button"
                  tabIndex={0}
                  aria-label={`${listing.token_label} · ${t("tokenMarket.viewListing")}`}
                  onClick={() => navigate(`/token-market/listing/${listing.listing_id}`)}
                  onKeyDown={(e) => {
                    // 键盘用户可用 Enter/空格打开详情
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      navigate(`/token-market/listing/${listing.listing_id}`);
                    }
                  }}
                >
                  <div className="token-market-listing-top">
                    <div>
                      <div className="token-market-listing-title">{listing.token_label}</div>
                      <div className="token-market-listing-subtitle">{listing.seller_username}</div>
                    </div>
                    <span className={`status-badge status-${listing.status}`}>
                      {listing.status === "active" ? t("tokenMarket.statusActive") : listing.status}
                    </span>
                  </div>

                  <div className="token-market-price-row">
                    <div>
                      <span className="token-market-mini-label">{t("tokenMarket.price")}</span>
                      <div className="token-market-price-value">
                        {listing.sale_type === "fixed_price"
                          ? `${listing.price} ${listing.currency}`
                          : listing.sale_type === "subscription"
                            ? `${listing.price} ${listing.currency} / ${t("tokenMarket.weekly")}`
                            : `${(listing.discount_rate * 100).toFixed(0)}% ${t("tokenMarket.off")}`}
                      </div>
                    </div>
                    <div className="token-market-price-side">
                      <span className="token-market-mini-label">{t("tokenMarket.availableQuota")}</span>
                      <div className="token-market-side-value">{listing.available_quota || "-"}</div>
                    </div>
                  </div>

                  <div className="token-market-card-footer">
                    <span>{t("tokenMarket.saleType")}: {listing.sale_type === "subscription" ? t("tokenMarket.saleTypeSubscription") : listing.sale_type === "fixed_price" ? t("tokenMarket.fixedPrice") : t("tokenMarket.discount")}</span>
                    {listing.sale_type === "subscription" && (
                      <span>
                        {t("tokenMarket.remainingSlots", {
                          left: Math.max((listing.capacity ?? 0) - (listing.active_subscriptions ?? 0), 0),
                          total: listing.capacity ?? 0,
                        })}
                      </span>
                    )}
                    <span>{t("tokenMarket.createdAt")}: {listing.created_at}</span>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="token-market-table-wrap">
              <table className="diag-table token-market-table">
                <thead>
                  <tr>
                    <th>{t("tokenMarket.tokenLabel")}</th>
                    <th>{t("tokenMarket.seller")}</th>
                    <th>{t("tokenMarket.saleType")}</th>
                    <th>{t("tokenMarket.price")}</th>
                    <th>{t("tokenMarket.availableQuota")}</th>
                    <th>{t("tokenMarket.status")}</th>
                    <th>{t("tokenMarket.createdAt")}</th>
                  </tr>
                </thead>
                <tbody>
                  {marketListings.map((listing) => (
                    <tr
                      key={listing.listing_id}
                      role="button"
                      tabIndex={0}
                      aria-label={`${listing.token_label} · ${t("tokenMarket.viewListing")}`}
                      style={{ cursor: "pointer" }}
                      onClick={() => navigate(`/token-market/listing/${listing.listing_id}`)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          navigate(`/token-market/listing/${listing.listing_id}`);
                        }
                      }}
                    >
                      <td>
                        <div className="token-market-table-title">{listing.token_label}</div>
                      </td>
                      <td>{listing.seller_username}</td>
                      <td>{listing.sale_type === "fixed_price" ? t("tokenMarket.fixedPrice") : listing.sale_type === "subscription" ? t("tokenMarket.saleTypeSubscription") : t("tokenMarket.discount")}</td>
                      <td className="token-market-table-price">
                        {listing.sale_type === "fixed_price"
                          ? `${listing.price} ${listing.currency}`
                          : listing.sale_type === "subscription"
                            ? `${listing.price} ${listing.currency} / ${t("tokenMarket.weekly")}`
                            : `${(listing.discount_rate * 100).toFixed(0)}% ${t("tokenMarket.off")}`}
                      </td>
                      <td>{listing.available_quota || "-"}</td>
                      <td>
                        <span className={`status-badge status-${listing.status}`}>
                          {listing.status === "active" ? t("tokenMarket.statusActive") : listing.status}
                        </span>
                      </td>
                      <td>{listing.created_at}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {page < totalPages && (
            <div className="token-market-load-more">
              <button className="btn btn-secondary" onClick={handleLoadMore} disabled={loadingMore}>
                {loadingMore ? t("common.loading") : `${t("tokenMarket.loadMore")} (${marketListings.length}/${total})`}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
