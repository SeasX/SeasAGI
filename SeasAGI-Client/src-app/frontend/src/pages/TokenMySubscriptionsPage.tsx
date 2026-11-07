import { useEffect, useState } from "react";
import { platformRequest, asPlatformRecord } from "../utils/commands";
import { useTranslation } from "../i18n";
import { ConfirmDialog } from "../components/ConfirmDialog";

type Subscription = {
  subscription_id: string;
  listing_id: string;
  seller_user_id: string;
  period: string;
  price: number;
  currency: string;
  quota_usd: number;
  used_usd: number;
  remaining_usd: number;
  status: string;
  current_period_end: string;
  auto_renew: boolean;
  cancel_at_period_end: boolean;
  grant_id: string;
  token_label?: string;
  wire_protocol?: string;
};

const STATUS_KEY: Record<string, string> = {
  pending: "tokenMarket.pendingApproval",
  active: "tokenMarket.statusActiveSub",
  past_due: "tokenMarket.statusPastDue",
  expired: "tokenMarket.statusExpired",
  cancelled: "tokenMarket.statusCancelledSub",
};

export function TokenMySubscriptionsPage() {
  const { t } = useTranslation();
  const [items, setItems] = useState<Subscription[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [confirmCancelId, setConfirmCancelId] = useState<string | null>(null);
  const [cancelling, setCancelling] = useState(false);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await platformRequest("GET", "/token-market/subscriptions/my");
      const body = asPlatformRecord(resp.body);
      if (resp.status >= 400) {
        throw new Error(String(body.error || `HTTP ${resp.status}`));
      }
      setItems((body.data || []) as Subscription[]);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const cancel = async (id: string) => {
    setCancelling(true);
    try {
      const resp = await platformRequest("POST", `/token-market/subscriptions/${id}/cancel`);
      if (resp.status >= 400) {
        throw new Error(String(asPlatformRecord(resp.body).error || `HTTP ${resp.status}`));
      }
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCancelling(false);
    }
  };

  // 争议申诉（P2-5）：由买家发起，管理后台裁定退款/违约金。
  const openDispute = async (id: string) => {
    const reason = window.prompt(t("tokenMarket.disputeReasonPrompt"));
    if (!reason || !reason.trim()) return;
    try {
      const resp = await platformRequest("POST", `/token-market/subscriptions/${id}/dispute`, {
        reason: reason.trim(),
      });
      if (resp.status >= 400) {
        throw new Error(String(asPlatformRecord(resp.body).error || `HTTP ${resp.status}`));
      }
      window.alert(t("tokenMarket.disputeSubmitted"));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t("tokenMarket.mySubscriptions")}</h1>
      </div>

      {loading && <p className="form-hint">{t("common.loading")}</p>}
      {error && <div className="form-error">{error}</div>}
      {!loading && items.length === 0 && <p className="form-hint">{t("tokenMarket.noData")}</p>}

      {items.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>{t("tokenMarket.tokenLabel")}</th>
              <th>{t("tokenMarket.seller")}</th>
              <th>{t("tokenMarket.period")}</th>
              <th>{t("tokenMarket.price")}</th>
              <th>{t("tokenMarket.remainingQuota")}</th>
              <th>{t("tokenMarket.status")}</th>
              <th>{t("tokenMarket.nextCharge")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {items.map((s) => (
              <tr key={s.subscription_id}>
                <td>{s.token_label || s.listing_id}</td>
                <td>{s.seller_user_id}</td>
                <td>{s.period === "monthly" ? t("tokenMarket.monthly") : t("tokenMarket.weekly")}</td>
                <td>
                  {s.price.toFixed(2)} {s.currency}
                </td>
                <td>
                  {s.remaining_usd.toFixed(2)} / {s.quota_usd.toFixed(2)}
                </td>
                <td>{t(STATUS_KEY[s.status] || "tokenMarket.status")}</td>
                <td>{s.current_period_end || "-"}</td>
                <td>
                  {(s.status === "active" || s.status === "past_due") && (
                    <button className="btn btn-secondary" onClick={() => openDispute(s.subscription_id)}>
                      {t("tokenMarket.openDispute")}
                    </button>
                  )}
                  {(s.status === "active" || s.status === "past_due" || s.status === "pending") && (
                    <button
                      className="btn btn-secondary"
                      onClick={() => setConfirmCancelId(s.subscription_id)}
                      disabled={cancelling}
                    >
                      {cancelling ? t("common.processing") : t("tokenMarket.cancelSubscription")}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <ConfirmDialog
        open={confirmCancelId !== null}
        title={t("tokenMarket.cancelSubscription")}
        message={t("tokenMarket.confirmCancelSubscription")}
        danger
        pending={cancelling}
        onCancel={() => setConfirmCancelId(null)}
        onConfirm={() => {
          const id = confirmCancelId;
          setConfirmCancelId(null);
          if (id) void cancel(id);
        }}
      />
    </div>
  );
}
