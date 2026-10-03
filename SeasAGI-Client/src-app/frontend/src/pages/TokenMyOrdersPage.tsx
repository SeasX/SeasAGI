import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { platformRequest } from "../utils/commands";
import { useTranslation } from "../i18n";
import { useMarketStore, type MarketOrder } from "../stores/marketStore";

export function TokenMyOrdersPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const myOrders = useMarketStore((s) => s.myOrders);
  const setMyOrders = useMarketStore((s) => s.setMyOrders);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [settlingId, setSettlingId] = useState<string | null>(null);
  // C6: 分页和状态筛选
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);
  const [totalPages, setTotalPages] = useState(1);
  const [, setTotal] = useState(0);
  const [statusFilter, setStatusFilter] = useState("");
  // P11: 评价功能
  const [reviewOrder, setReviewOrder] = useState<string | null>(null);
  const [reviewRating, setReviewRating] = useState(5);
  const [reviewComment, setReviewComment] = useState("");
  const [submittingReview, setSubmittingReview] = useState(false);

  const fetchOrders = async (pageNum?: number, status?: string) => {
    setLoading(true);
    setError(null);
    try {
      const p = pageNum ?? page;
      const st = status ?? statusFilter;
      const params = new URLSearchParams();
      params.set("page", String(p));
      params.set("page_size", String(pageSize));
      if (st) params.set("status", st);
      const resp = await platformRequest("GET", `/token-market/orders?${params}`);
      if (resp.status >= 400) throw new Error(`HTTP ${resp.status}`);
      const data = resp.body;
      setMyOrders(data.data || []);
      setTotal(data.total || 0);
      setTotalPages(data.total_pages || 1);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchOrders(1, "");
  }, []);

  const handleSettle = async (orderId: string) => {
    setSettlingId(orderId);
    try {
      const resp = await platformRequest("POST", `/token-market/orders/${orderId}/settle`);
      if (resp.status >= 400) {
        const errData = resp.body || {};
        throw new Error(errData.error || `HTTP ${resp.status}`);
      }
      // 更新本地状态
      setMyOrders(myOrders.map((o) => o.order_id === orderId ? { ...o, status: "settled" } : o));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSettlingId(null);
    }
  };

  // P11: 提交评价
  const handleSubmitReview = async () => {
    if (!reviewOrder) return;
    setSubmittingReview(true);
    setError(null);
    try {
      const resp = await platformRequest("POST", `/token-market/orders/${reviewOrder}/review`, { rating: reviewRating, comment: reviewComment });
      if (resp.status >= 400) {
        const errData = resp.body || {};
        throw new Error(errData.error || `HTTP ${resp.status}`);
      }
      setReviewOrder(null);
      setReviewRating(5);
      setReviewComment("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmittingReview(false);
    }
  };

  const statusLabel = (status: string) => {
    const map: Record<string, string> = {
      pending: t("tokenMarket.orderPending"),
      confirmed: t("tokenMarket.orderConfirmed"),
      cancelled: t("tokenMarket.orderCancelled"),
      settled: t("tokenMarket.orderSettled"),
    };
    return map[status] || status;
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t("tokenMarket.myOrders")}</h1>
        <div className="page-header-actions">
          <button className="btn btn-secondary" onClick={() => navigate("/token-market/my-settlements")}>
            {t("tokenMarket.mySettlements")}
          </button>
          <button className="btn btn-secondary" onClick={() => navigate("/token-market")}>
            ← {t("tokenMarket.backToMarket")}
          </button>
        </div>
      </div>

      {/* C6: Status filter */}
      <div className="token-market-filters" style={{ display: "flex", gap: "12px", alignItems: "flex-end", marginBottom: "16px" }}>
        <div className="filter-field">
          <label className="filter-label" style={{ fontSize: "12px", display: "block", marginBottom: "4px" }}>{t("tokenMarket.status")}</label>
          <select
            className="form-input"
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value);
              setPage(1);
              fetchOrders(1, e.target.value);
            }}
            style={{ width: "140px" }}
          >
            <option value="">{t("tokenMarket.all")}</option>
            <option value="pending">{t("tokenMarket.orderPending")}</option>
            <option value="confirmed">{t("tokenMarket.orderConfirmed")}</option>
            <option value="cancelled">{t("tokenMarket.orderCancelled")}</option>
            <option value="settled">{t("tokenMarket.orderSettled")}</option>
          </select>
        </div>
      </div>

      {loading && <div className="loading-state">{t("common.loading")}</div>}
      {error && <div className="form-error">{error}</div>}

      {!loading && myOrders.length === 0 && (
        <div className="empty-state">
          <div className="empty-icon">📋</div>
          <p>{t("tokenMarket.noOrders")}</p>
        </div>
      )}

      {!loading && myOrders.length > 0 && (
        <>
        <table className="data-table">
          <thead>
            <tr>
              <th>{t("tokenMarket.orderId")}</th>
              <th>{t("tokenMarket.role")}</th>
              <th>{t("tokenMarket.amount")}</th>
              <th>{t("tokenMarket.commission")}</th>
              <th>{t("tokenMarket.sellerPayout")}</th>
              <th>{t("tokenMarket.status")}</th>
              <th>{t("tokenMarket.createdAt")}</th>
              <th>{t("common.actions")}</th>
            </tr>
          </thead>
          <tbody>
            {myOrders.map((order: MarketOrder) => (
              <tr key={order.order_id}>
                <td>{order.order_id.substring(0, 12)}...</td>
                <td>
                  <span className={`role-badge role-${order.role}`}>
                    {order.role === "seller" ? t("tokenMarket.roleSeller") : t("tokenMarket.roleBuyer")}
                  </span>
                </td>
                <td>{order.amount}</td>
                <td>{order.commission_amount}</td>
                <td>{order.settlement_amount}</td>
                <td>
                  <span className={`status-badge status-${order.status}`}>{statusLabel(order.status)}</span>
                </td>
                <td>{order.created_at}</td>
                <td>
                  {order.status === "pending" && order.role === "buyer" && (
                    <button
                      className="btn btn-sm btn-primary"
                      onClick={() => navigate(`/token-market/scan-trade?order=${order.order_id}`)}
                    >
                      {t("tokenMarket.continueTrade")}
                    </button>
                  )}
                  {order.status === "confirmed" && order.role === "buyer" && (
                    <span className="text-muted">{t("tokenMarket.awaitingSettlement")}</span>
                  )}
                  {order.status === "confirmed" && order.role === "seller" && (
                    <button
                      className="btn btn-sm btn-primary"
                      onClick={() => handleSettle(order.order_id)}
                      disabled={settlingId === order.order_id}
                    >
                      {settlingId === order.order_id ? t("common.processing") : t("tokenMarket.settle")}
                    </button>
                  )}
                  {order.status === "settled" && order.role === "buyer" && (
                    <button
                      className="btn btn-sm btn-secondary"
                      onClick={() => { setReviewOrder(order.order_id); setReviewRating(5); setReviewComment(""); }}
                    >
                      {t("tokenMarket.leaveReview") || "Review"}
                    </button>
                  )}
                  {order.status === "settled" && order.role === "seller" && (
                    <span className="text-muted">{t("tokenMarket.settlementSettled")}</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>

        {/* C6: Pagination */}
        {totalPages > 1 && (
          <div className="token-market-load-more" style={{ marginTop: "16px", display: "flex", gap: "8px", justifyContent: "center" }}>
            <button
              className="btn btn-secondary"
              disabled={page <= 1}
              onClick={() => { const np = page - 1; setPage(np); fetchOrders(np); }}
            >
              ← Prev
            </button>
            <span style={{ padding: "8px 12px", alignSelf: "center" }}>{page} / {totalPages}</span>
            <button
              className="btn btn-secondary"
              disabled={page >= totalPages}
              onClick={() => { const np = page + 1; setPage(np); fetchOrders(np); }}
            >
              Next →
            </button>
          </div>
        )}
        </>
      )}

      {/* P11: 评价弹窗 */}
      {reviewOrder && (
        <div className="modal-overlay" onClick={() => setReviewOrder(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()} style={{ maxWidth: "400px" }}>
            <h3 style={{ marginBottom: "16px" }}>{t("tokenMarket.leaveReview") || "Leave a Review"}</h3>
            <div className="form-group" style={{ marginBottom: "16px" }}>
              <label className="form-label">{t("tokenMarket.rating") || "Rating"}</label>
              <div style={{ display: "flex", gap: "8px", fontSize: "24px" }}>
                {[1, 2, 3, 4, 5].map((star) => (
                  <span
                    key={star}
                    style={{ cursor: "pointer", color: star <= reviewRating ? "var(--color-warning)" : "var(--text-tertiary)" }}
                    onClick={() => setReviewRating(star)}
                  >
                    {"★"}
                  </span>
                ))}
              </div>
            </div>
            <div className="form-group" style={{ marginBottom: "16px" }}>
              <label className="form-label">{t("tokenMarket.comment") || "Comment"}</label>
              <textarea
                className="form-input"
                rows={3}
                value={reviewComment}
                onChange={(e) => setReviewComment(e.target.value)}
                placeholder={t("tokenMarket.commentPlaceholder") || "Share your experience..."}
              />
            </div>
            <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}>
              <button className="btn btn-secondary" onClick={() => setReviewOrder(null)} disabled={submittingReview}>
                {t("common.cancel")}
              </button>
              <button className="btn btn-primary" onClick={handleSubmitReview} disabled={submittingReview}>
                {submittingReview ? t("common.processing") : t("common.submit") || "Submit"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
