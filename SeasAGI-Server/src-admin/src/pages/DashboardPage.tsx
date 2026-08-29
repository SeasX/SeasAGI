import { useEffect, useState } from "react";
import { apiFetch } from "../utils/api";
import type { User, UsageStats } from "../utils/types";
import { useTranslation } from "../i18n";

export function DashboardPage() {
  const { t } = useTranslation();
  const [users, setUsers] = useState<User[]>([]);
  const [usageStats, setUsageStats] = useState<UsageStats | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    Promise.all([
      apiFetch<{ data: User[] }>("/users"),
      apiFetch<UsageStats>("/usage/stats"),
    ])
      .then(([u, s]) => {
        setUsers(u.data);
        setUsageStats(s);
      })
      .catch((e) => setErr(e.message));
  }, []);

  if (err) return <div className="error-msg">{err}</div>;
  if (!usageStats) return <div className="loading">{t("admin.loading")}</div>;

  return (
    <>
      <h1 className="page-title">{t("admin.dashboard")}</h1>
      <div className="card-grid">
        <div className="card">
          <div className="card-label">{t("admin.totalUsers")}</div>
          <div className="card-value blue">{users.length}</div>
        </div>
        <div className="card">
          <div className="card-label">{t("admin.totalRequests")}</div>
          <div className="card-value">{usageStats.total_requests.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">{t("admin.totalInputTokens")}</div>
          <div className="card-value">{usageStats.total_input_tokens.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">{t("admin.totalOutputTokens")}</div>
          <div className="card-value">{usageStats.total_output_tokens.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">{t("admin.totalTokens")}</div>
          <div className="card-value">{usageStats.total_tokens.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">{t("admin.totalErrors")}</div>
          <div className="card-value red">{usageStats.total_errors.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="card-label">{t("admin.distinctUsers")}</div>
          <div className="card-value green">{usageStats.distinct_users}</div>
        </div>
      </div>
    </>
  );
}