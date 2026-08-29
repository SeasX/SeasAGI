import { useState, useEffect } from "react";
import { apiFetch } from "../utils/api";
import { useTranslation } from "../i18n";

interface ComboStep {
  model: string;
  channel_id?: string;
  step_role?: string;
}

interface AdminCombo {
  combo_id: string;
  scope: string;
  owner_id: string;
  tenant_id: string;
  logical_name: string;
  display_name: string;
  description: string;
  tags: string;
  strategy: string;
  sticky_uses: number;
  steps_json: string;
  steps?: ComboStep[];
  status: string;
  source: string;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export function AdminCombosPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<"platform" | "tenant" | "user-custom">("platform");
  const [combos, setCombos] = useState<AdminCombo[]>([]);
  const [users, setUsers] = useState<{ user_id: string; email: string }[]>([]);
  const [selectedUserId, setSelectedUserId] = useState("");
  const [userCombos, setUserCombos] = useState<AdminCombo[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<Partial<AdminCombo> | null>(null);
  const [stepLines, setStepLines] = useState("");

  useEffect(() => {
    if (tab === "platform" || tab === "tenant") { loadCombos(); }
    if (tab === "user-custom") { loadUsers(); }
  }, [tab]);

  const loadCombos = async () => {
    setLoading(true);
    setError("");
    try {
      const scope = tab === "platform" ? "platform" : "tenant";
      const res = await apiFetch<{ data: AdminCombo[] }>(`/combos?scope=${scope}`);
      setCombos(res.data || []);
    } catch (e: any) { setError(e.message); }
    setLoading(false);
  };

  const loadUsers = async () => {
    try {
      const res = await apiFetch<{ data: { user_id: string; email: string }[] }>("/users");
      setUsers(res.data || []);
    } catch {}
  };

  const loadUserCombos = async (userId: string) => {
    if (!userId) { setUserCombos([]); return; }
    setLoading(true);
    try {
      const res = await apiFetch<{ data: AdminCombo[] }>(`/users/${userId}/custom-combos`);
      setUserCombos(res.data || []);
    } catch { setUserCombos([]); }
    setLoading(false);
  };

  const openCreate = () => {
    setEditing({ scope: tab === "platform" ? "platform" : "tenant", strategy: "fallback", status: "draft" });
    setStepLines("");
  };

  const openEdit = (c: AdminCombo) => {
    setEditing(c);
    const steps: ComboStep[] = c.steps_json ? JSON.parse(c.steps_json) : [];
    setStepLines(steps.map(s => `${s.step_role || "backup"}|${s.model}${s.channel_id ? `|${s.channel_id}` : ""}`).join("\n"));
  };

  const handleSave = async () => {
    if (!editing) return;
    setError("");
    const lines = stepLines.split("\n").map(line => line.trim()).filter(Boolean);
    const steps: ComboStep[] = lines.map((line) => {
      const parts = line.split("|");
      const step: ComboStep = { model: parts[1] || parts[0], step_role: parts[0] || "backup" };
      if (parts[2]) step.channel_id = parts[2];
      return step;
    });

    const body: Record<string, any> = {
      logical_name: editing.logical_name,
      display_name: editing.display_name,
      description: editing.description || "",
      strategy: editing.strategy || "fallback",
      steps,
      status: editing.status || "draft",
    };

    try {
      if (editing.combo_id) {
        await apiFetch(`/combos/${editing.combo_id}`, { method: "PUT", body: JSON.stringify(body) });
      } else {
        body.scope = editing.scope || "platform";
        await apiFetch("/combos", { method: "POST", body: JSON.stringify(body) });
      }
      setEditing(null);
      loadCombos();
    } catch (e: any) { setError(e.message); }
  };

  const handleToggle = async (c: AdminCombo) => {
    try {
      const newStatus = c.status === "active" ? "draft" : "active";
      await apiFetch(`/combos/${c.combo_id}`, {
        method: "PUT",
        body: JSON.stringify({ status: newStatus }),
      });
      loadCombos();
    } catch (e: any) { setError(e.message); }
  };

  const handleDelete = async (c: AdminCombo) => {
    if (!confirm(t("admin.comboConfirmArchive").replace("{{name}}", c.display_name || c.logical_name))) return;
    try {
      await apiFetch(`/combos/${c.combo_id}`, { method: "DELETE" });
      loadCombos();
    } catch (e: any) { setError(e.message); }
  };

  const parseSteps = (json: string): ComboStep[] => {
    try { return JSON.parse(json); } catch { return []; }
  };

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">{t("admin.comboPageTitle")}</h1>
        <div className="action-bar">
          <div className="tabs">
            <button className={`tab ${tab === "platform" ? "active" : ""}`} onClick={() => setTab("platform")}>
              {t("admin.comboPlatformTab")}
            </button>
            <button className={`tab ${tab === "tenant" ? "active" : ""}`} onClick={() => setTab("tenant")}>
              {t("admin.comboTenantTab")}
            </button>
            <button className={`tab ${tab === "user-custom" ? "active" : ""}`} onClick={() => setTab("user-custom")}>
              {t("admin.comboUserTab")}
            </button>
          </div>
        </div>
      </div>

      {error && <div className="error-msg">{error}</div>}

      {/* ---- Platform/Tenant list ---- */}
      {(tab === "platform" || tab === "tenant") && (
        <>
          <div className="action-bar" style={{ marginBottom: 16 }}>
            <button className="btn btn-primary btn-sm" onClick={openCreate}>{t("admin.comboCreateBtn")}</button>
          </div>

          {loading ? <div className="loading">{t("admin.comboLoading")}</div> : (
            <table>
              <thead>
                <tr>
                  <th>{t("admin.comboNameCol")}</th>
                  <th>{t("admin.comboStrategyCol")}</th>
                  <th>{t("admin.comboStepsCol")}</th>
                  <th>{t("admin.comboStatusCol")}</th>
                  <th>{t("admin.comboVersionCol")}</th>
                  <th>{t("admin.comboActionsCol")}</th>
                </tr>
              </thead>
              <tbody>
                {combos.map(c => (
                  <tr key={c.combo_id}>
                    <td>
                      <strong>{c.display_name || c.logical_name}</strong>
                      <br /><span style={{ fontSize: 11, color: "var(--text-dim)" }}>{c.scope}</span>
                    </td>
                    <td>{c.strategy}</td>
                    <td>{t("admin.comboStepsFormat").replace("{{count}}", String(parseSteps(c.steps_json).length))}</td>
                    <td><span className={`badge ${c.status === "active" ? "badge-green" : c.status === "draft" ? "badge-yellow" : "badge-red"}`}>{c.status}</span></td>
                    <td>v{c.version}</td>
                    <td>
                      <div className="action-bar">
                        <button className="btn btn-icon" onClick={() => openEdit(c)} title={t("admin.comboEditTooltip")}>✏️</button>
                        <button className="btn btn-icon" onClick={() => handleToggle(c)} title={c.status === "active" ? t("admin.comboToggleOff") : t("admin.comboToggle")}>
                          {c.status === "active" ? "⏸" : "▶️"}
                        </button>
                        <button className="btn btn-icon danger" onClick={() => handleDelete(c)} title={t("admin.comboArchiveTooltip")}>🗑️</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}

      {/* ---- User Custom ---- */}
      {tab === "user-custom" && (
        <div>
          <div className="form-group" style={{ marginBottom: 16 }}>
            <label className="form-label">{t("admin.comboSelectUser")}</label>
            <select className="form-select" value={selectedUserId} onChange={(e) => { setSelectedUserId(e.target.value); loadUserCombos(e.target.value); }}>
              <option value="">{t("admin.comboSelectUserPlace")}</option>
              {users.map(u => (
                <option key={u.user_id} value={u.user_id}>{u.email}</option>
              ))}
            </select>
          </div>

          {loading ? <div className="loading">{t("admin.comboLoading")}</div> : (
            <table>
              <thead>
                <tr>
                  <th>{t("admin.comboNameCol")}</th>
                  <th>{t("admin.comboStrategyCol")}</th>
                  <th>{t("admin.comboStepsCol")}</th>
                  <th>{t("admin.comboStatusCol")}</th>
                  <th>{t("admin.comboSource")}</th>
                </tr>
              </thead>
              <tbody>
                {userCombos.map(c => (
                  <tr key={c.combo_id}>
                    <td><strong>{c.display_name || c.logical_name}</strong></td>
                    <td>{c.strategy}</td>
                    <td>{t("admin.comboStepsFormat").replace("{{count}}", String(parseSteps(c.steps_json).length))}</td>
                    <td><span className={`badge ${c.status === "active" ? "badge-green" : c.status === "draft" ? "badge-yellow" : "badge-red"}`}>{c.status}</span></td>
                    <td>{c.source}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {/* ---- Edit/Create modal ---- */}
      {editing && (
        <div className="modal-overlay" onClick={() => setEditing(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-title">{editing.combo_id ? t("admin.comboEditTitle") : t("admin.comboCreateTitle")}</div>
            {error && <div className="error-msg">{error}</div>}
            <div className="form-group">
              <label className="form-label">{t("admin.comboFormDisplayName")}</label>
              <input className="form-input" value={editing.display_name || ""} onChange={(e) => setEditing({ ...editing, display_name: e.target.value })} />
            </div>
            <div className="form-group">
              <label className="form-label">{t("admin.comboFormLogicalName")}</label>
              <input className="form-input" value={editing.logical_name || ""} onChange={(e) => setEditing({ ...editing, logical_name: e.target.value })} />
            </div>
            <div className="form-group">
              <label className="form-label">{t("admin.comboFormDescription")}</label>
              <textarea className="form-textarea" value={editing.description || ""} onChange={(e) => setEditing({ ...editing, description: e.target.value })} rows={2} />
            </div>
            <div className="form-group">
              <label className="form-label">{t("admin.comboFormStrategy")}</label>
              <select className="form-select" value={editing.strategy || "fallback"} onChange={(e) => setEditing({ ...editing, strategy: e.target.value })}>
                <option value="fallback">fallback - {t("admin.comboStrategyCol")}</option>
                <option value="parallel">parallel</option>
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">{t("admin.comboFormSteps")}</label>
              <p style={{ fontSize: 12, color: "var(--text-dim)", marginBottom: 4 }}>{t("admin.comboStepRole")}: primary / backup / last_resort</p>
              <textarea
                className="form-textarea"
                value={stepLines}
                onChange={(e) => setStepLines(e.target.value)}
                rows={5}
                placeholder={t("admin.comboFormStepsPlaceholder")}
              />
            </div>
            <div className="modal-actions">
              <button className="btn btn-ghost" onClick={() => setEditing(null)}>{t("admin.comboCancelBtn")}</button>
              <button className="btn btn-primary" onClick={handleSave}>{t("admin.comboSaveBtn")}</button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}