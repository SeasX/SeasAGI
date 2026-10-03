import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { platformRequest } from "../utils/commands";
import { useTranslation } from "../i18n";
import { useMarketStore } from "../stores/marketStore";

export function TokenMyListingsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const myListings = useMarketStore((s) => s.myListings);
  const setMyListings = useMarketStore((s) => s.setMyListings);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [cancellingId, setCancellingId] = useState<string | null>(null);
  const [earningsMap, setEarningsMap] = useState<Record<string, { payout: number; commission: number }>>({});

  const fetchListings = async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await platformRequest("GET", "/token-market/my-listings");
      if (resp.status >= 400) throw new Error(`HTTP ${resp.status}`);
      const data = resp.body;
      setMyListings(data.data || []);

      // 获取已售出上架的结算信息
      const soldListings = (data.data || []).filter((l: any) => l.status === "sold");
      if (soldListings.length > 0) {
        const settlementsResp = await platformRequest("GET", "/token-market/settlements");
        if (settlementsResp.status < 400) {
          const settlementsData = settlementsResp.body;
          const map: Record<string, { payout: number; commission: number }> = {};
          for (const s of (settlementsData.data || [])) {
            map[s.listing_id] = { payout: s.seller_payout, commission: s.platform_commission };
          }
          setEarningsMap(map);
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchListings();
  }, []);

  const handleCancel = async (listingId: string) => {
    setCancellingId(listingId);
    try {
      const resp = await platformRequest("POST", `/token-market/listings/${listingId}/cancel`);
      if (resp.status >= 400) {
        const errData = resp.body || {};
        throw new Error(errData.error || `HTTP ${resp.status}`);
      }
      // 更新本地状态
      setMyListings(myListings.map((l) => l.listing_id === listingId ? { ...l, status: "cancelled" } : l));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setCancellingId(null);
    }
  };

  const statusLabel = (status: string) => {
    const map: Record<string, string> = {
      active: t("tokenMarket.statusActive"),
      locked: t("tokenMarket.statusLocked"),
      sold: t("tokenMarket.statusSold"),
      cancelled: t("tokenMarket.statusCancelled"),
    };
    return map[status] || status;
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t("tokenMarket.myListings")}</h1>
        <div className="page-header-actions">
          <button className="btn btn-primary" onClick={() => navigate("/token-market/create")}>
            {t("tokenMarket.createListing")}
          </button>
        </div>
      </div>

      {loading && <div className="loading-state">{t("common.loading")}</div>}
      {error && <div className="form-error">{error}</div>}

      {!loading && myListings.length === 0 && (
        <div className="empty-state">
          <div className="empty-icon">📦</div>
          <p>{t("tokenMarket.noListings")}</p>
          <button className="btn btn-primary" onClick={() => navigate("/token-market/create")}>
            {t("tokenMarket.createListing")}
          </button>
        </div>
      )}

      {!loading && myListings.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>{t("tokenMarket.tokenLabel")}</th>
              <th>{t("tokenMarket.saleType")}</th>
              <th>{t("tokenMarket.price")}</th>
              <th>{t("tokenMarket.availableQuota")}</th>
              <th>{t("tokenMarket.earnings")}</th>
              <th>{t("tokenMarket.status")}</th>
              <th>{t("tokenMarket.createdAt")}</th>
              <th>{t("common.actions")}</th>
            </tr>
          </thead>
          <tbody>
            {myListings.map((listing) => {
              const earnings = earningsMap[listing.listing_id];
              return (
                <tr key={listing.listing_id}>
                  <td>{listing.token_label}</td>
                  <td>{listing.sale_type === "fixed_price" ? t("tokenMarket.fixedPrice") : t("tokenMarket.discount")}</td>
                  <td>
                    {listing.sale_type === "fixed_price"
                      ? `${listing.price} ${listing.currency}`
                      : `${(listing.discount_rate * 100).toFixed(0)}% off`}
                  </td>
                  <td>{listing.available_quota || "-"}</td>
                  <td>
                    {listing.status === "sold" && earnings ? (
                      <span className="earnings-info">
                        {earnings.payout} {listing.currency}
                        <span className="text-muted"> ({t("tokenMarket.commission")}: {earnings.commission})</span>
                      </span>
                    ) : "-"}
                  </td>
                  <td>
                    <span className={`status-badge status-${listing.status}`}>{statusLabel(listing.status)}</span>
                  </td>
                  <td>{listing.created_at}</td>
                  <td>
                    {listing.status === "active" && (
                      <button
                        className="btn btn-sm btn-danger"
                        onClick={() => handleCancel(listing.listing_id)}
                        disabled={cancellingId === listing.listing_id}
                      >
                        {cancellingId === listing.listing_id ? t("common.processing") : t("tokenMarket.cancelListing")}
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
