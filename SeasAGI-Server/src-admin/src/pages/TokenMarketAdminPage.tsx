import { useEffect, useState } from "react";
import { apiFetch } from "../utils/api";

interface RevenueSummary {
  total_commission: number;
  total_transaction_volume: number;
  active_sellers: number;
  total_orders: number;
  settled_orders: number;
}

interface MarketConfig {
  [key: string]: string;
}

interface RiskRule {
  rule_id: string;
  rule_type: string;
  threshold_value: string;
  action: string;
  enabled: number;
  updated_at: string;
}

export function TokenMarketAdminPage() {
  const [tab, setTab] = useState<"revenue" | "config" | "risk">("revenue");
  const [revenue, setRevenue] = useState<RevenueSummary | null>(null);
  const [config, setConfig] = useState<MarketConfig>({});
  const [riskRules, setRiskRules] = useState<RiskRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    Promise.all([
      apiFetch<{ data: RevenueSummary }>("/token-market/revenue-summary"),
      apiFetch<{ data: MarketConfig }>("/token-market/config"),
      apiFetch<{ data: RiskRule[] }>("/token-market/risk-rules"),
    ])
      .then(([rev, cfg, rules]) => {
        setRevenue(rev.data || rev);
        setConfig(cfg.data || cfg);
        setRiskRules(rules.data || rules);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  const handleSaveConfig = async () => {
    setSaving(true);
    setError("");
    try {
      await apiFetch("/token-market/config", {
        method: "POST",
        body: JSON.stringify(config),
      });
    } catch (e: any) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const handleToggleRule = async (ruleID: string, currentEnabled: number) => {
    try {
      await apiFetch(`/token-market/risk-rules/${ruleID}`, {
        method: "POST",
        body: JSON.stringify({ enabled: currentEnabled === 1 ? 0 : 1 }),
      });
      setRiskRules(riskRules.map((r) => r.rule_id === ruleID ? { ...r, enabled: currentEnabled === 1 ? 0 : 1 } : r));
    } catch (e: any) {
      setError(e.message);
    }
  };

  if (loading) return <div className="loading">Loading...</div>;
  if (error) return <div className="error-msg">{error}</div>;

  return (
    <>
      <h1 className="page-title">Token Market</h1>

      <div className="tab-bar" style={{ display: "flex", gap: "8px", marginBottom: "20px" }}>
        <button className={`btn ${tab === "revenue" ? "btn-primary" : "btn-secondary"}`} onClick={() => setTab("revenue")}>
          Revenue
        </button>
        <button className={`btn ${tab === "config" ? "btn-primary" : "btn-secondary"}`} onClick={() => setTab("config")}>
          Config
        </button>
        <button className={`btn ${tab === "risk" ? "btn-primary" : "btn-secondary"}`} onClick={() => setTab("risk")}>
          Risk Rules
        </button>
      </div>

      {tab === "revenue" && revenue && (
        <div className="card-grid">
          <div className="card">
            <div className="card-label">Total Commission</div>
            <div className="card-value green">${revenue.total_commission?.toFixed(2) || "0.00"}</div>
          </div>
          <div className="card">
            <div className="card-label">Transaction Volume</div>
            <div className="card-value">${revenue.total_transaction_volume?.toFixed(2) || "0.00"}</div>
          </div>
          <div className="card">
            <div className="card-label">Active Sellers</div>
            <div className="card-value blue">{revenue.active_sellers || 0}</div>
          </div>
          <div className="card">
            <div className="card-label">Total Orders</div>
            <div className="card-value">{revenue.total_orders || 0}</div>
          </div>
          <div className="card">
            <div className="card-label">Settled Orders</div>
            <div className="card-value">{revenue.settled_orders || 0}</div>
          </div>
        </div>
      )}

      {tab === "config" && (
        <div style={{ maxWidth: "600px" }}>
          <div className="form-group" style={{ marginBottom: "16px" }}>
            <label className="form-label">Default Commission Rate</label>
            <input
              type="text"
              className="form-input"
              value={config.default_commission_rate || "0.10"}
              onChange={(e) => setConfig({ ...config, default_commission_rate: e.target.value })}
            />
          </div>
          <div className="form-group" style={{ marginBottom: "16px" }}>
            <label className="form-label">Min Listing Price (USD)</label>
            <input
              type="text"
              className="form-input"
              value={config.min_listing_price || "0.50"}
              onChange={(e) => setConfig({ ...config, min_listing_price: e.target.value })}
            />
          </div>
          <button className="btn btn-primary" onClick={handleSaveConfig} disabled={saving}>
            {saving ? "Saving..." : "Save Config"}
          </button>
        </div>
      )}

      {tab === "risk" && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Rule ID</th>
              <th>Type</th>
              <th>Threshold</th>
              <th>Action</th>
              <th>Enabled</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {riskRules.map((rule) => (
              <tr key={rule.rule_id}>
                <td>{rule.rule_id}</td>
                <td>{rule.rule_type}</td>
                <td style={{ fontSize: "12px", maxWidth: "300px", overflow: "hidden", textOverflow: "ellipsis" }}>
                  {rule.threshold_value}
                </td>
                <td>{rule.action}</td>
                <td>
                  <button
                    className={`btn btn-sm ${rule.enabled === 1 ? "btn-primary" : "btn-secondary"}`}
                    onClick={() => handleToggleRule(rule.rule_id, rule.enabled)}
                  >
                    {rule.enabled === 1 ? "ON" : "OFF"}
                  </button>
                </td>
                <td style={{ fontSize: "12px" }}>{rule.updated_at}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}
