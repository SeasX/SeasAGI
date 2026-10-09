import { useEffect, useState } from "react";
import { apiFetch } from "../utils/api";
import { useTranslation } from "../i18n";

/* ============================================================
   Types
   ============================================================ */
interface Channel {
  channel_id: string;
  channel_type: string;
  provider_type: string;
  display_name: string;
  base_url: string;
  enabled: boolean;
  sort_order: number;
  weight: number;
  priority: number;
  gray_percent: number;
  read_only: boolean;
  encrypted_api_key: string;
  models: { model_id: string; model_name: string; capability: string }[];
}

interface CustomChannel {
  channel_id: string;
  user_id: string;
  tenant_id: string;
  provider_type: string;
  display_name: string;
  base_url: string;
  models: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

interface UserSummary {
  user_id: string;
  email: string;
}

/* ============================================================
   Component
   ============================================================ */
export function ChannelsPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<"recommended" | "custom">("recommended");

  /* Platform-recommended channels */
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  /* Editing state */
  const [editing, setEditing] = useState<Partial<Channel> | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);

  /* Models editing */
  const [modelsText, setModelsText] = useState("");

  /* Custom channels per user */
  const [users, setUsers] = useState<UserSummary[]>([]);
  const [selectedUserId, setSelectedUserId] = useState("");
  const [customChannels, setCustomChannels] = useState<CustomChannel[]>([]);
  const [loadingCustom, setLoadingCustom] = useState(false);

  /* -------- Data loaders -------- */
  const loadChannels = () => {
    setLoading(true);
    setError("");
    apiFetch<{ data: Channel[] }>("/channels")
      .then((r) => setChannels(r.data))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };

  const loadUsers = () => {
    apiFetch<{ data: UserSummary[] }>("/users")
      .then((r) => setUsers(r.data))
      .catch(() => {});
  };

  const loadCustomChannels = (userId: string) => {
    if (!userId) { setCustomChannels([]); return; }
    setLoadingCustom(true);
    apiFetch<{ data: CustomChannel[] }>(`/users/${userId}/custom-channels`)
      .then((r) => setCustomChannels(r.data))
      .catch(() => setCustomChannels([]))
      .finally(() => setLoadingCustom(false));
  };

  useEffect(() => { loadChannels(); loadUsers(); }, []);

  useEffect(() => { loadCustomChannels(selectedUserId); }, [selectedUserId]);

  /* -------- CRUD operations -------- */
  const handleCreate = () => {
    setEditing({
      channel_id: "",
      channel_type: "platform",
      provider_type: "openai",
      display_name: "",
      base_url: "",
      enabled: true,
      sort_order: 0,
    });
    setModelsText("");
    setEditingId(null);
  };

  const handleEdit = (ch: Channel) => {
    setEditing({ ...ch });
    setModelsText(ch.models.map((m) => `${m.model_id}|${m.model_name}|${m.capability}`).join("\n"));
    setEditingId(ch.channel_id);
  };

  const handleSave = async () => {
    if (!editing) return;
    const models = modelsText
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => {
        const parts = line.split("|");
        return { model_id: parts[0], model_name: parts[1] || parts[0], capability: parts[2] || "chat" };
      });

    try {
      const body: Record<string, any> = {
        channel_id: editing.channel_id,
        provider_type: editing.provider_type,
        display_name: editing.display_name,
        base_url: editing.base_url,
        enabled: editing.enabled,
        sort_order: editing.sort_order || 0,
        models,
      };
      if (editingId) {
        await apiFetch(`/channels/${editingId}`, { method: "PUT", body: JSON.stringify(body) });
      } else {
        await apiFetch("/channels", { method: "POST", body: JSON.stringify(body) });
      }
      setEditing(null);
      setEditingId(null);
      loadChannels();
    } catch (e: any) { setError(e.message); }
  };

  const handleToggle = async (ch: Channel) => {
    try {
      await apiFetch(`/channels/${ch.channel_id}`, {
        method: "PUT",
        body: JSON.stringify({ enabled: !ch.enabled }),
      });
      loadChannels();
    } catch (e: any) { setError(e.message); }
  };

  const handleDelete = async (ch: Channel) => {
    if (!confirm(t("admin.gatewayConfirmDelete").replace("{{name}}", ch.display_name))) return;
    try {
      await apiFetch(`/channels/${ch.channel_id}`, { method: "DELETE" });
      loadChannels();
    } catch (e: any) { setError(e.message); }
  };

  /* -------- Render -------- */
  return (
    <>
      <div className="page-header">
        <h1 className="page-title">{t("admin.channels")}</h1>
        <div className="action-bar">
          <div className="tabs">
            <button
              className={`tab ${tab === "recommended" ? "active" : ""}`}
              onClick={() => setTab("recommended")}
            >
              {t("admin.combosPlatform")}
            </button>
            <button
              className={`tab ${tab === "custom" ? "active" : ""}`}
              onClick={() => setTab("custom")}
            >
              {t("admin.combosUser")}
            </button>
          </div>
        </div>
      </div>

      {error && <div className="error-msg">{error}</div>}

      {/* ---- Tab: 平台推荐通道 ---- */}
      {tab === "recommended" && (
        <>
          <div className="action-bar" style={{ marginBottom: 16 }}>
            <button className="btn btn-primary btn-sm" onClick={handleCreate}>
              {t("admin.channelCreateBtn")}
            </button>
            <button className="btn btn-ghost btn-sm" onClick={loadChannels}>
              {t("admin.channelRefreshBtn")}
            </button>
          </div>

          {/* Edit / Create form */}
          {editing && (
            <div className="card" style={{ marginBottom: 16, padding: 16 }}>
              <h3 style={{ fontSize: 15, fontWeight: 600, marginBottom: 12 }}>
                {editingId ? t("admin.channelEditTitle") : t("admin.channelCreateTitle")}
              </h3>
              <div className="form-grid">
                {!editingId && (
                  <div className="form-group">
                    <label>{t("admin.channelId")}</label>
                    <input
                      value={editing.channel_id || ""}
                      onChange={(e) => setEditing({ ...editing, channel_id: e.target.value })}
                      placeholder={t("admin.channelIdPlaceholder")}
                    />
                  </div>
                )}
                <div className="form-group">
                  <label>{t("admin.channelProviderType")}</label>
                  <select
                    value={editing.provider_type || ""}
                    onChange={(e) => setEditing({ ...editing, provider_type: e.target.value })}
                  >
                    <option value="openai">OpenAI</option>
                    <option value="anthropic">Anthropic</option>
                    <option value="google">Google</option>
                    <option value="azure">Azure</option>
                    <option value="deepseek">DeepSeek</option>
                    <option value="ollama">Ollama</option>
                    <option value="custom">{t("admin.channelCustomProvider")}</option>
                  </select>
                </div>
                <div className="form-group">
                  <label>{t("admin.channelNameCol")}</label>
                  <input
                    value={editing.display_name || ""}
                    onChange={(e) => setEditing({ ...editing, display_name: e.target.value })}
                    placeholder={t("admin.channelNamePlaceholder")}
                  />
                </div>
                <div className="form-group">
                  <label>{t("admin.channelBaseUrl")}</label>
                  <input
                    value={editing.base_url || ""}
                    onChange={(e) => setEditing({ ...editing, base_url: e.target.value })}
                    placeholder={t("admin.channelBaseUrlPlaceholder")}
                  />
                </div>
                <div className="form-group">
                  <label>{t("admin.channelSortOrder")}</label>
                  <input
                    type="number"
                    value={editing.sort_order ?? 0}
                    onChange={(e) => setEditing({ ...editing, sort_order: parseInt(e.target.value, 10) })}
                  />
                </div>
                <div className="form-group">
                  <label>{t("admin.channelEnabled")}</label>
                  <input
                    type="checkbox"
                    checked={!!editing.enabled}
                    onChange={(e) => setEditing({ ...editing, enabled: e.target.checked })}
                  />
                </div>
              </div>
              <div className="form-group" style={{ marginTop: 8 }}>
                <label>{t("admin.channelModelList")}（{t("admin.channelModelFormat")}）</label>
                <textarea
                  value={modelsText}
                  onChange={(e) => setModelsText(e.target.value)}
                  rows={5}
                  placeholder={"gpt-4o|GPT-4o|chat\ngpt-4o-mini|GPT-4o Mini|chat"}
                  style={{ width: "100%", fontFamily: "monospace", fontSize: 12 }}
                />
              </div>
              <div className="action-bar" style={{ marginTop: 12 }}>
                <button className="btn btn-primary btn-sm" onClick={handleSave}>{t("admin.channelSave")}</button>
                <button className="btn btn-ghost btn-sm" onClick={() => { setEditing(null); setEditingId(null); }}>{t("admin.channelCancel")}</button>
              </div>
            </div>
          )}

          {/* Table */}
          {loading ? (
            <div className="loading">{t("admin.channelLoading")}</div>
          ) : channels.length === 0 ? (
            <div className="empty">{t("admin.channelEmptyPlatform")}</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>{t("admin.channelNameCol")}</th>
                  <th>{t("admin.channelProviderCol")}</th>
                  <th>{t("admin.channelBaseUrl")}</th>
                  <th>{t("admin.channelModelCount")}</th>
                  <th>{t("admin.channelSortOrder")}</th>
                  <th>{t("admin.channelStatus")}</th>
                  <th>{t("admin.channelActions")}</th>
                </tr>
              </thead>
              <tbody>
                {channels.map((ch) => (
                  <tr key={ch.channel_id}>
                    <td><code style={{ fontSize: 11 }}>{ch.channel_id}</code></td>
                    <td>{ch.display_name}</td>
                    <td>{ch.provider_type}</td>
                    <td style={{ fontSize: 11, maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis" }}>
                      {ch.base_url}
                    </td>
                    <td>{ch.models.length}</td>
                    <td>{ch.sort_order}</td>
                    <td>
                      <span className={`badge ${ch.enabled ? "badge-green" : "badge-red"}`}>
                        {ch.enabled ? t("admin.channelEnabled") : t("admin.channelDisabled")}
                      </span>
                    </td>
                    <td>
                      <div className="action-bar">
                        <button className="btn btn-icon" onClick={() => handleEdit(ch)} title={t("admin.channelEditTooltip")}>✏️</button>
                        <button className="btn btn-icon" onClick={() => handleToggle(ch)} title={ch.enabled ? t("admin.channelToggleDisableTooltip") : t("admin.channelToggleEnableTooltip")}>
                          {ch.enabled ? "⏸" : "▶️"}
                        </button>
                        <button className="btn btn-icon danger" onClick={() => handleDelete(ch)} title={t("admin.channelDeleteTooltip")}>🗑️</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}

      {/* ---- Custom tab ---- */}
      {tab === "custom" && (
        <>
          <div className="action-bar" style={{ marginBottom: 16 }}>
            <label style={{ marginRight: 8, fontWeight: 500 }}>{t("admin.channelSelectUserLabel")}</label>
            <select
              value={selectedUserId}
              onChange={(e) => setSelectedUserId(e.target.value)}
              style={{ minWidth: 280 }}
            >
              <option value="">{t("admin.channelSelectUserPlaceholder")}</option>
              {users.map((u) => (
                <option key={u.user_id} value={u.user_id}>
                  {u.email} ({u.user_id.slice(0, 8)}…)
                </option>
              ))}
            </select>
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => loadCustomChannels(selectedUserId)}
              disabled={!selectedUserId}
            >
              {t("admin.channelRefreshBtn")}
            </button>
          </div>

          {!selectedUserId ? (
            <div className="empty">{t("admin.channelNoUserSelected")}</div>
          ) : loadingCustom ? (
            <div className="loading">{t("admin.channelLoading")}</div>
          ) : customChannels.length === 0 ? (
            <div className="empty">{t("admin.channelNoCustomChannels")}</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>{t("admin.channelId")}</th>
                  <th>{t("admin.channelNameCol")}</th>
                  <th>{t("admin.channelProviderCol")}</th>
                  <th>{t("admin.channelBaseUrl")}</th>
                  <th>{t("admin.channelModelsCol")}</th>
                  <th>{t("admin.channelStatus")}</th>
                  <th>{t("admin.channelCreatedAt")}</th>
                </tr>
              </thead>
              <tbody>
                {customChannels.map((ch) => (
                  <tr key={ch.channel_id}>
                    <td><code style={{ fontSize: 11 }}>{ch.channel_id}</code></td>
                    <td>{ch.display_name}</td>
                    <td>{ch.provider_type}</td>
                    <td style={{ fontSize: 11, maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis" }}>
                      {ch.base_url}
                    </td>
                    <td style={{ fontSize: 11 }}>
                      {(() => {
                        try { return JSON.parse(ch.models).join(", "); } catch { return ch.models; }
                      })()}
                    </td>
                    <td>
                      <span className={`badge ${ch.enabled ? "badge-green" : "badge-red"}`}>
                        {ch.enabled ? t("admin.channelEnabled") : t("admin.channelDisabled")}
                      </span>
                    </td>
                    <td style={{ fontSize: 12 }}>{new Date(ch.created_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </>
  );
}