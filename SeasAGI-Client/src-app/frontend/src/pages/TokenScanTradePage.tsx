import { useEffect, useState } from "react";
import { useSearchParams, useNavigate } from "react-router-dom";
import QRCode from "qrcode";
import { platformRequest } from "../utils/commands";
import { useTranslation } from "../i18n";
import { useMarketStore, type MarketOrder } from "../stores/marketStore";

export function TokenScanTradePage() {
  const [searchParams] = useSearchParams();
  const orderId = searchParams.get("order") || "";
  const { t } = useTranslation();
  const navigate = useNavigate();
  const setTradeSession = useMarketStore((s) => s.setTradeSession);
  const setCommissionPreview = useMarketStore((s) => s.setCommissionPreview);
  const [order, setOrder] = useState<MarketOrder | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  // 确认/取消操作错误（独立于页面级 error，避免整页被错误信息替换）
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    const fetchOrder = async () => {
      if (!orderId) {
        setError(t("tokenMarket.invalidOrder"));
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      try {
        const resp = await platformRequest("GET", `/token-market/orders/${orderId}`);
        if (resp.status >= 400) throw new Error(`HTTP ${resp.status}`);
        const data = resp.body;
        setOrder(data.data);
        setTradeSession(data.data);
        setCommissionPreview({
          amount: data.data.amount,
          commission: data.data.commission_amount,
          payout: data.data.settlement_amount,
        });
        // 渲染真实二维码图形，便于面对面扫码核对
        if (data.data.scan_code) {
          try {
            const url = await QRCode.toDataURL(data.data.scan_code, { width: 220, margin: 1 });
            setQrDataUrl(url);
          } catch {
            setQrDataUrl(null);
          }
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    };
    fetchOrder();
  }, [orderId]);

  const handleConfirm = async () => {
    if (!order) return;
    setConfirming(true);
    setActionError(null);
    try {
      const resp = await platformRequest("POST", `/token-market/orders/${orderId}/confirm`);
      if (resp.status >= 400) {
        const errData = resp.body || {};
        throw new Error(errData.error || `HTTP ${resp.status}`);
      }
      const data = resp.body;
      navigate(`/token-market/my-orders`, { state: { confirmedOrder: data.data } });
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setConfirming(false);
    }
  };

  const handleCancel = async () => {
    if (!order) return;
    setConfirming(true);
    setActionError(null);
    try {
      const resp = await platformRequest("POST", `/token-market/orders/${orderId}/cancel`);
      if (resp.status >= 400) {
        const errData = resp.body || {};
        throw new Error(errData.error || `HTTP ${resp.status}`);
      }
      navigate("/token-market");
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setConfirming(false);
    }
  };

  if (loading) return <div className="page-container"><div className="loading-state">{t("common.loading")}</div></div>;
  if (error) return <div className="page-container"><div className="form-error">{error}</div></div>;
  if (!order) return <div className="page-container"><div className="empty-state"><p>{t("tokenMarket.orderNotFound")}</p></div></div>;

  // P5: 解析二维码过期时间
  const expMatch = order.scan_code.match(/exp=(\d+)/);
  const isExpired = expMatch ? Date.now() / 1000 > parseInt(expMatch[1]) : false;

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t("tokenMarket.scanTrade")}</h1>
        <p className="page-subtitle">{t("tokenMarket.scanTradeSubtitle")}</p>
      </div>

      <div className="scan-trade-container">
        {/* 二维码区域 */}
        <div className="qr-section">
          <div className="qr-placeholder">
            {qrDataUrl ? (
              <img src={qrDataUrl} alt="QR" style={{ width: 220, height: 220, borderRadius: 8 }} />
            ) : (
              <div className="qr-icon">📱</div>
            )}
            <p className="qr-code-text">{order.scan_code}</p>
            <p className="qr-hint">{t("tokenMarket.scanCodeHint")}</p>
            {isExpired && order.status === "pending" && (
              <div className="risk-warning" style={{ marginTop: 12, color: "#e74c3c", fontWeight: 600 }}>
                ⚠️ {t("tokenMarket.qrExpired")}
              </div>
            )}
          </div>
        </div>

        {/* 交易确认区域 */}
        <div className="trade-confirm-section">
          <h2>{t("tokenMarket.tradeSummary")}</h2>
          <div className="detail-row">
            <span className="detail-key">{t("tokenMarket.orderId")}</span>
            <span className="detail-value">{order.order_id.substring(0, 12)}...</span>
          </div>
          <div className="detail-row">
            <span className="detail-key">{t("tokenMarket.amount")}</span>
            <span className="detail-value price-value">{order.amount}</span>
          </div>
          <div className="detail-row">
            <span className="detail-key">{t("tokenMarket.commission")} ({order.commission_rate ? (order.commission_rate * 100).toFixed(1) : "0"}%)</span>
            <span className="detail-value">{order.commission_amount}</span>
          </div>
          <div className="detail-row">
            <span className="detail-key">{t("tokenMarket.sellerPayout")}</span>
            <span className="detail-value price-value">{order.settlement_amount}</span>
          </div>
          <div className="detail-row">
            <span className="detail-key">{t("tokenMarket.status")}</span>
            <span className={`status-badge status-${order.status}`}>{order.status}</span>
          </div>

          {order.status === "pending" && (
            <div className="trade-actions">
              <button className="btn btn-danger" onClick={handleCancel} disabled={confirming}>
                {confirming ? t("common.processing") : t("tokenMarket.cancelOrder")}
              </button>
              <button className="btn btn-primary" onClick={handleConfirm} disabled={confirming}>
                {confirming ? t("common.processing") : t("tokenMarket.confirmTrade")}
              </button>
            </div>
          )}
          {order.status === "confirmed" && (
            <div className="trade-success">
              ✅ {t("tokenMarket.tradeConfirmed")}
            </div>
          )}
          {actionError && <div className="form-error">{actionError}</div>}
        </div>
      </div>
    </div>
  );
}
