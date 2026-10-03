import { useEffect, useState } from "react";
import { useTranslation } from "../i18n";
import { useAppStore } from "../stores/appStore";
import { createCheckoutSession, getPlans, getCloudBilling, openInBrowser } from "../utils/commands";

interface PlanInfo {
  plan_id: string;
  name: string;
  description: string;
  price: number;
  monthly_quota: number;
  max_rpm: number;
  max_tpm: number;
  sort_order: number;
  relay_enabled?: boolean;
}

const PLAN_AUDIENCES: Record<string, string> = {
  free: "subscription.freeAudience",
  pro: "subscription.proAudience",
  teams: "subscription.teamsAudience",
  enterprise: "subscription.enterpriseAudience",
};

const PLAN_VALUES: Record<string, string> = {
  free: "subscription.freeValue",
  pro: "subscription.proValue",
  teams: "subscription.teamsValue",
  enterprise: "subscription.enterpriseValue",
};

export function SubscriptionPage() {
  const { t } = useTranslation();
  const auth = useAppStore((s) => s.auth);
  const cloudBilling = useAppStore((s) => s.cloudBilling);
  const setCloudBilling = useAppStore((s) => s.setCloudBilling);
  const [plans, setPlans] = useState<PlanInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [subscribing, setSubscribing] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [messageType, setMessageType] = useState<"success" | "error">("success");
  const [teamSeats, setTeamSeats] = useState<Record<string, number>>({});

  useEffect(() => {
    (async () => {
      if (!auth.is_logged_in) {
        setLoading(false);
        return;
      }
      try {
        const items = await getPlans();
        setPlans(items.map((p: any) => ({
          plan_id: p.plan_id,
          name: p.name,
          description: p.description,
          price: p.price,
          monthly_quota: p.monthly_quota,
          max_rpm: p.max_rpm,
          max_tpm: p.max_tpm,
          sort_order: p.sort_order || 0,
          relay_enabled: p.relay_enabled,
        })));
      } catch {
        setPlans([]);
      }
      setLoading(false);
    })();
  }, [auth.is_logged_in]);

  // 付款回跳后切回客户端窗口时，自动刷新订阅状态（webhook 已在服务端激活订阅）
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === "visible" && auth.is_logged_in) {
        getCloudBilling().then(setCloudBilling).catch(() => {});
      }
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [auth.is_logged_in, setCloudBilling]);

  const handleSubscribe = async (planId: string, quantity?: number) => {
    if (!auth.is_logged_in) {
      setMessageType("error");
      setMessage(t("subscription.pleaseLogin"));
      return;
    }
    setSubscribing(planId);
    setMessage(null);
    try {
      const result = await createCheckoutSession(planId, quantity || 1);
      const url = result?.checkout_url;
      if (url) {
        setMessageType("success");
        setMessage(t("subscription.openingPayment"));
        await openInBrowser(url);
      } else {
        setMessageType("success");
        setMessage(t("subscription.switchedTo", { plan: planId }));
        setTimeout(() => window.location.reload(), 1000);
      }
    } catch (e: any) {
      setMessageType("error");
      setMessage(e.message || t("subscription.operationFailed"));
    }
    setSubscribing(null);
  };

  if (loading) return <div className="page"><div className="loading">{t("subscription.loading")}</div></div>;

  if (!auth.is_logged_in) {
    return (
      <div className="page subscription-page">
        <div className="page-header page-hero">
          <div>
            <h1>{t("subscription.title")}</h1>
            <p className="page-subtitle">{t("subscription.pageSubtitle")}</p>
          </div>
        </div>
        <div className="section section-card" style={{ textAlign: "center", padding: 48 }}>
          <p style={{ fontSize: 16, marginBottom: 16 }}>{t("subscription.pleaseLogin")}</p>
        </div>
      </div>
    );
  }

  const currentPlan = cloudBilling?.plan_id || "free";

  const sorted = [...plans].sort((a, b) => a.sort_order - b.sort_order);

  return (
    <div className="page subscription-page">
      <div className="page-header page-hero">
        <div>
          <h1>{t("subscription.title")}</h1>
          <p className="page-subtitle">{t("subscription.pageSubtitle")}</p>
        </div>
        <div className="hero-metrics">
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("subscription.currentPlanLabel")}</span>
            <strong className="hero-metric-value">{currentPlan}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("subscription.availablePlansLabel")}</span>
            <strong className="hero-metric-value">{plans.length}</strong>
          </div>
        </div>
      </div>

      {message && (
        <div className="status-card" style={{ marginBottom: 16, padding: 12, borderColor: messageType === "error" ? "var(--red)" : "var(--green)" }}>
          {message}
        </div>
      )}

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("subscription.planListTitle")}</h2>
          <p className="hint">{t("subscription.planListHint")}</p>
        </div>
        <div className="plan-cards">
        {sorted.map((plan) => {
          const isCurrent = currentPlan === plan.plan_id;
          const audienceKey = PLAN_AUDIENCES[plan.plan_id] || "";
          const valueKey = PLAN_VALUES[plan.plan_id] || "";
          const isEnterprise = plan.plan_id === "enterprise";
          const isFree = plan.plan_id === "free";
          const isTeams = plan.plan_id === "teams";
          const seats = teamSeats[plan.plan_id] || 3;
          const totalPrice = plan.price * seats;

          return (
            <div key={plan.plan_id} className={`plan-card ${isCurrent ? "plan-current" : ""} ${isEnterprise ? "plan-enterprise" : ""}`}>
              <div className="plan-card-header">
                <h2 className="plan-name">{plan.name}</h2>
                {isCurrent && <span className="badge badge-green">{t("subscription.currentBadge")}</span>}
              </div>
              {isEnterprise ? (
                <div className="plan-price">
                  <span className="plan-amount">{t("subscription.contactSales")}</span>
                </div>
              ) : isTeams ? (
                <div className="plan-price">
                  <span className="plan-amount">${plan?.price || 0}</span>
                  <span className="plan-period">{t("subscription.perSeatMonth")}</span>
                </div>
              ) : (
                <div className="plan-price">
                  <span className="plan-amount">${plan?.price || 0}</span>
                  <span className="plan-period">{isFree ? "" : t("subscription.perMonth")}</span>
                </div>
              )}
              <div className="plan-audience">{audienceKey ? t(audienceKey) : ""}</div>
              <div className="plan-value-tag">{valueKey ? t(valueKey) : ""}</div>
              <p className="plan-desc">{plan.description}</p>
              <ul className="plan-features">
                <li><strong>{plan?.monthly_quota ? (plan.monthly_quota >= 1000000 ? `${(plan.monthly_quota / 1000000).toFixed(0)}M` : plan.monthly_quota.toLocaleString()) : (isEnterprise ? t("subscription.customQuota") : t("subscription.unlimited"))}</strong> {t("subscription.monthlyRequests")}</li>
                <li>{t("subscription.maxRPM")}: <strong>{plan?.max_rpm || 0}</strong></li>
                <li>{t("subscription.maxTPM")}: <strong>{plan?.max_tpm?.toLocaleString() || 0}</strong></li>
                {plan.relay_enabled && <li className="feature-yes">{t("subscription.cloudRelay")}</li>}
                {!plan.relay_enabled && !isFree && <li className="feature-no">{t("subscription.noCloudRelay")}</li>}
              </ul>
              {isEnterprise ? (
                <button
                  className="btn-outline"
                  onClick={() => openInBrowser("https://github.com/neeke/seasagi/issues")}
                  style={{ width: "100%" }}
                >
                  {t("subscription.contactSalesBtn")}
                </button>
              ) : isFree ? (
                <a
                  href="https://github.com/neeke/seasagi"
                  className="btn-outline"
                  style={{ width: "100%", justifyContent: "center", display: "flex", textDecoration: "none" }}
                >
                  {t("subscription.freeUsage")}
                </a>
              ) : isTeams ? (
                <div style={{ width: "100%" }}>
                  <div className="seat-selector">
                    <button
                      className="seat-btn"
                      onClick={() => setTeamSeats(prev => ({ ...prev, [plan.plan_id]: Math.max(1, (prev[plan.plan_id] || 3) - 1) }))}
                      disabled={seats <= 1}
                    >−</button>
                    <span className="seat-count">{seats} {t("subscription.seats")}</span>
                    <button
                      className="seat-btn"
                      onClick={() => setTeamSeats(prev => ({ ...prev, [plan.plan_id]: Math.min(50, (prev[plan.plan_id] || 3) + 1) }))}
                      disabled={seats >= 50}
                    >+</button>
                  </div>
                  <div className="seat-total">${totalPrice.toFixed(2)}/mo {t("subscription.total")}</div>
                  <button
                    className={isCurrent ? "btn-secondary" : "btn-primary"}
                    onClick={() => handleSubscribe(plan.plan_id, seats)}
                    disabled={subscribing === plan.plan_id || isCurrent}
                    style={{ width: "100%", marginTop: 8 }}
                  >
                    {subscribing === plan.plan_id ? t("subscription.processing") :
                     isCurrent ? t("subscription.currentBadge") :
                     `${t("subscription.subscribe")} $${totalPrice.toFixed(2)}/mo`}
                  </button>
                </div>
              ) : (
                <button
                  className={isCurrent ? "btn-secondary" : "btn-primary"}
                  onClick={() => handleSubscribe(plan.plan_id)}
                  disabled={subscribing === plan.plan_id || isCurrent}
                  style={{ width: "100%" }}
                >
                  {subscribing === plan.plan_id ? t("subscription.processing") :
                   isCurrent ? t("subscription.currentBadge") :
                   `${t("subscription.subscribe")} $${plan.price}/mo`}
                </button>
              )}
            </div>
          );
        })}
        </div>
      </div>
    </div>
  );
}
