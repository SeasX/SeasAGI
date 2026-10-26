import { useEffect, useState } from "react";
import { platformRequest, asPlatformRecord } from "../utils/commands";
import { useTranslation } from "../i18n";
import { ConfirmDialog } from "../components/ConfirmDialog";

type Batch = {
  batch_id: string;
  period_start: string;
  period_end: string;
  status: string;
  total_gross: number;
  total_relay_fee: number;
  total_net: number;
  item_count: number;
};

type Item = {
  item_id: string;
  batch_id: string;
  subscription_id: string;
  gross: number;
  relay_fee: number;
  net: number;
  created_at: string;
};

type Subscriber = {
  subscription_id: string;
  buyer_user_id: string;
  token_label?: string;
  price: number;
  currency?: string;
  quota_usd: number;
  used_usd: number;
  status: string;
  current_period_end: string;
};

const STATUS_KEY: Record<string, string> = {
  pending: "tokenMarket.pendingApproval",
  active: "tokenMarket.statusActiveSub",
  past_due: "tokenMarket.statusPastDue",
  expired: "tokenMarket.statusExpired",
  cancelled: "tokenMarket.statusCancelledSub",
};

export function TokenProviderEarningsPage() {
  const { t } = useTranslation();
  const [batches, setBatches] = useState<Batch[]>([]);
  const [items, setItems] = useState<Item[]>([]);
  const [subs, setSubs] = useState<Subscriber[]>([]);
  const [settled, setSettled] = useState(0);
  const [pending, setPending] = useState(0);
  const [rate, setRate] = useState(0.05);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [confirmSubId, setConfirmSubId] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [earn, subsResp] = await Promise.all([
        platformRequest("GET", "/token-market/provider/earnings"),
        platformRequest("GET", "/token-market/provider/subscriptions"),
      ]);
      const earnBody = asPlatformRecord(earn.body);
      const subsBody = asPlatformRecord(subsResp.body);
      if (earn.status >= 400) throw new Error(String(earnBody.error || `HTTP ${earn.status}`));
      if (subsResp.status >= 400) throw new Error(String(subsBody.error || `HTTP ${subsResp.status}`));

      const data = earnBody.data || {};
      setBatches((data.batches || []) as Batch[]);
      setItems((data.items || []) as Item[]);
      setSettled(data.settled_net_total || 0);
      setPending(data.pending_net || 0);
      setRate(data.relay_fee_rate || 0.05);
      setSubs((subsBody.data || []) as Subscriber[]);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const confirm = async (id: string) => {
    setConfirming(true);
    try {
      const resp = await platformRequest("POST", `/token-market/subscriptions/${id}/confirm`);
      if (resp.status >= 400) throw new Error(String(asPlatformRecord(resp.body).error || `HTTP ${resp.status}`));
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setConfirming(false);
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t("tokenMarket.providerEarnings")}</h1>
      </div>

      {error && <div className="form-error">{error}</div>}
      {loading && <p className="form-hint">{t("common.loading")}</p>}

      <div className="stats-grid" style={{ display: "flex", gap: 24, marginBottom: 24 }}>
        <div>
          <div className="stat-label">{t("tokenMarket.settledNetTotal")}</div>
          <div className="stat-value">{settled.toFixed(2)} USD</div>
        </div>
        <div>
          <div className="stat-label">{t("tokenMarket.pendingNet")}</div>
          <div className="stat-value">{pending.toFixed(2)} USD</div>
        </div>
        <div>
          <div className="stat-label">{t("tokenMarket.relayFee")}</div>
          <div className="stat-value">{(rate * 100).toFixed(0)}%</div>
        </div>
      </div>

      <h2>{t("tokenMarket.providerSubscriptions")}</h2>
      {subs.length === 0 ? (
        <p className="form-hint">{t("tokenMarket.noData")}</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>{t("tokenMarket.tokenLabel")}</th>
              <th>{t("tokenMarket.seller")}</th>
              <th>{t("tokenMarket.price")}</th>
              <th>{t("tokenMarket.usedQuota")}</th>
              <th>{t("tokenMarket.status")}</th>
              <th>{t("tokenMarket.nextCharge")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {subs.map((s) => (
              <tr key={s.subscription_id}>
                <td>{s.token_label || "-"}</td>
                <td>{s.buyer_user_id}</td>
                <td>{s.price.toFixed(2)} {s.currency || "USD"}</td>
                <td>
                  {s.used_usd.toFixed(2)} / {s.quota_usd.toFixed(2)}
                </td>
                <td>{t(STATUS_KEY[s.status] || "tokenMarket.status")}</td>
                <td>{s.current_period_end || "-"}</td>
                <td>
                  {s.status === "pending" && (
                    <button
                      className="btn btn-primary"
                      onClick={() => setConfirmSubId(s.subscription_id)}
                      disabled={confirming}
                    >
                      {confirming ? t("common.processing") : t("tokenMarket.statusActiveSub")}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2 style={{ marginTop: 32 }}>{t("tokenMarket.payoutBatches")}</h2>
      {batches.length === 0 ? (
        <p className="form-hint">{t("tokenMarket.noData")}</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>{t("tokenMarket.batch")}</th>
              <th>{t("tokenMarket.createdAt")}</th>
              <th>{t("tokenMarket.gross")}</th>
              <th>{t("tokenMarket.relayFee")}</th>
              <th>{t("tokenMarket.net")}</th>
            </tr>
          </thead>
          <tbody>
            {batches.map((b) => (
              <tr key={b.batch_id}>
                <td>{b.batch_id}</td>
                <td>{b.period_start} ~ {b.period_end}</td>
                <td>{b.total_gross.toFixed(2)}</td>
                <td>{b.total_relay_fee.toFixed(2)}</td>
                <td>{b.total_net.toFixed(2)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {items.length > 0 && (
        <>
          <h3 style={{ marginTop: 24 }}>{t("tokenMarket.mySettlements")}</h3>
          <table className="data-table">
            <thead>
              <tr>
                <th>{t("tokenMarket.batch")}</th>
                <th>{t("tokenMarket.gross")}</th>
                <th>{t("tokenMarket.relayFee")}</th>
                <th>{t("tokenMarket.net")}</th>
                <th>{t("tokenMarket.createdAt")}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((i) => (
                <tr key={i.item_id}>
                  <td>{i.batch_id}</td>
                  <td>{i.gross.toFixed(2)}</td>
                  <td>{i.relay_fee.toFixed(2)}</td>
                  <td>{i.net.toFixed(2)}</td>
                  <td>{i.created_at}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
      <ConfirmDialog
        open={confirmSubId !== null}
        title={t("tokenMarket.statusActiveSub")}
        message={t("tokenMarket.confirmConfirmSubscription")}
        pending={confirming}
        onCancel={() => setConfirmSubId(null)}
        onConfirm={() => {
          const id = confirmSubId;
          setConfirmSubId(null);
          if (id) void confirm(id);
        }}
      />
    </div>
  );
}
