import { useEffect, useState } from "react";
import { apiFetch } from "../utils/api";
import type { User } from "../utils/types";
import { useTranslation } from "../i18n";

export function UsersPage() {
  const { t } = useTranslation();
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = () => {
    setLoading(true);
    setError("");
    apiFetch<{ data: User[] }>("/users")
      .then((r) => setUsers(r.data))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  const handleDelete = async (user: User) => {
    if (!confirm(t("admin.userConfirmDelete").replace("{{email}}", user.email))) return;
    try {
      await apiFetch(`/users/${user.user_id}`, { method: "DELETE" });
      load();
    } catch (e: any) { setError(e.message); }
  };

  const handleUpdatePlan = async (user: User) => {
    const planId = prompt(t("admin.userPromptPlanId"), user.plan);
    if (!planId) return;
    try {
      await apiFetch(`/users/${user.user_id}/plan`, { method: "PUT", body: JSON.stringify({ plan: planId }) });
      load();
    } catch (e: any) { setError(e.message); }
  };

  if (loading) return <div className="loading">{t("admin.userLoading")}</div>;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">{t("admin.userPageTitle")}</h1>
        <div className="action-bar">
          <button className="btn btn-ghost btn-sm" onClick={load}>{t("admin.userRefreshBtn")}</button>
        </div>
      </div>

      {error && <div className="error-msg">{error}</div>}

      {users.length === 0 ? (
        <div className="empty">{t("admin.userEmpty")}</div>
      ) : (
        <table>
          <thead>
            <tr>
              <th>{t("admin.userIdCol")}</th>
              <th>{t("admin.userEmailCol")}</th>
              <th>{t("admin.userPlanCol")}</th>
              <th>{t("admin.userRegisteredAtCol")}</th>
              <th>{t("admin.userActionsCol")}</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.user_id}>
                <td style={{ fontFamily: "monospace", fontSize: 12 }}>{u.user_id.slice(0, 12)}</td>
                <td>{u.email}</td>
                <td><span className="badge badge-blue">{u.plan}</span></td>
                <td style={{ fontSize: 13 }}>{new Date(u.created_at).toLocaleString()}</td>
                <td>
                  <div className="action-bar">
                    <button className="btn btn-icon" onClick={() => handleUpdatePlan(u)} title={t("admin.userEditPlanTooltip")}>✏️</button>
                    <button className="btn btn-icon danger" onClick={() => handleDelete(u)} title={t("admin.userDeleteTooltip")}>🗑️</button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}