import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getPlatformAPIBaseURL, getPlatformToken } from "../utils/commands";
import { useTranslation } from "../i18n";
import { useMarketStore, type MarketSettlement } from "../stores/marketStore";

export function TokenMySettlementsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const mySettlements = useMarketStore((s) => s.mySettlements);
  const setMySettlements = useMarketStore((s) => s.setMySettlements);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchSettlements = async () => {
    setLoading(true);
    setError(null);
    try {
      const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
      const resp = await fetch(`${baseURL}/token-market/settlements`, {
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setMySettlements(data.data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSettlements();
  }, []);

  const statusLabel = (status: string) => {
    const map: Record<string, string> = {
      pending: t("tokenMarket.settlementPending"),
      settled: t("tokenMarket.settlementSettled"),
      failed: t("tokenMarket.settlementFailed"),
    };
    return map[status] || status;
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t("tokenMarket.mySettlements")}</h1>
        <div className="page-header-actions">
          <button className="btn btn-secondary" onClick={() => navigate("/token-market")}>
            ← {t("tokenMarket.backToMarket")}
          </button>
        </div>
      </div>

      {loading && <div className="loading-state">{t("common.loading")}</div>}
      {error && <div className="form-error">{error}</div>}

      {!loading && mySettlements.length === 0 && (
        <div className="empty-state">
          <div className="empty-icon">💰</div>
          <p>{t("tokenMarket.noSettlements")}</p>
        </div>
      )}

      {!loading && mySettlements.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>{t("tokenMarket.tokenLabel")}</th>
              <th>{t("tokenMarket.tradeAmount")}</th>
              <th>{t("tokenMarket.commission")}</th>
              <th>{t("tokenMarket.sellerPayout")}</th>
              <th>{t("tokenMarket.status")}</th>
              <th>{t("tokenMarket.settledAt")}</th>
            </tr>
          </thead>
          <tbody>
            {mySettlements.map((s: MarketSettlement) => (
              <tr key={s.settlement_id}>
                <td>{s.token_label}</td>
                <td>{s.trade_amount}</td>
                <td>{s.platform_commission}</td>
                <td>{s.seller_payout}</td>
                <td>
                  <span className={`status-badge status-${s.settlement_status}`}>
                    {statusLabel(s.settlement_status)}
                  </span>
                </td>
                <td>{s.settled_at || "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
