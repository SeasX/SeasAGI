import { useEffect, useState } from "react";
import { apiFetch } from "../utils/api";
import { useTranslation } from "../i18n";

interface RelayGateway {
  gateway_id: string;
  name: string;
  host: string;
  port: number;
  region: string;
  enabled: boolean;
  created_at: string;
}

export function RelayGatewaysPage() {
  const { t } = useTranslation();
  const [gateways, setGateways] = useState<RelayGateway[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<RelayGateway | null>(null);

  const load = () => {
    setLoading(true);
    setError("");
    apiFetch<{ data: RelayGateway[] }>("/relay-gateways")
      .then((r) => setGateways(r.data))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  const handleCreate = async () => {
    const gatewayId = prompt(t("admin.gatewayIdPrompt"));
    if (!gatewayId) return;
    const name = prompt(t("admin.gatewayNamePrompt")) || gatewayId;
    const host = prompt(t("admin.gatewayHostPrompt")) || "";
    const port = parseInt(prompt(t("admin.gatewayPortPrompt"), "8318") || "8318", 10);
    const region = prompt(t("admin.gatewayRegionPrompt")) || "";
    try {
      await apiFetch("/relay-gateways", {
        method: "POST",
        body: JSON.stringify({ gateway_id: gatewayId, name, host, port, region }),
      });
      load();
    } catch (e: any) { setError(e.message); }
  };

  const handleToggle = async (gw: RelayGateway) => {
    try {
      await apiFetch(`/relay-gateways/${gw.gateway_id}`, {
        method: "PUT",
        body: JSON.stringify({ enabled: !gw.enabled }),
      });
      load();
    } catch (e: any) { setError(e.message); }
  };

  const handleEdit = (gw: RelayGateway) => {
    setEditing({ ...gw });
  };

  const handleSaveEdit = async () => {
    if (!editing) return;
    try {
      await apiFetch(`/relay-gateways/${editing.gateway_id}`, {
        method: "PUT",
        body: JSON.stringify({
          name: editing.name,
          host: editing.host,
          port: editing.port,
          region: editing.region,
        }),
      });
      setEditing(null);
      load();
    } catch (e: any) { setError(e.message); }
  };

  const handleDelete = async (gw: RelayGateway) => {
    if (!confirm(t("admin.gatewayConfirmDeleteName").replace("{{name}}", gw.name))) return;
    try {
      await apiFetch(`/relay-gateways/${gw.gateway_id}`, { method: "DELETE" });
      load();
    } catch (e: any) { setError(e.message); }
  };

  if (loading) return <div className="loading">{t("admin.gatewayLoading")}</div>;

  return (
    <>
      <div className="page-header">
        <h1 className="page-title">{t("admin.gatewayPageTitle")}</h1>
        <div className="action-bar">
          <button className="btn btn-primary btn-sm" onClick={handleCreate}>{t("admin.gatewayCreateBtn")}</button>
          <button className="btn btn-ghost btn-sm" onClick={load}>{t("admin.gatewayRefreshBtn")}</button>
        </div>
      </div>
      <p style={{ color: "var(--text-dim)", fontSize: 13, marginBottom: 16 }}>{t("admin.gatewaySubtitle")}</p>

      {error && <div className="error-msg">{error}</div>}

      {gateways.length === 0 ? (
        <div className="empty">{t("admin.gatewayEmpty")}</div>
      ) : (
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>{t("admin.gatewayNameCol")}</th>
              <th>{t("admin.gatewayHostCol")}</th>
              <th>{t("admin.gatewayPortCol")}</th>
              <th>{t("admin.gatewayRegionCol")}</th>
              <th>{t("admin.gatewayStatusCol")}</th>
              <th>{t("admin.gatewayCreatedAtCol")}</th>
              <th>{t("admin.gatewayActionsCol")}</th>
            </tr>
          </thead>
          <tbody>
            {gateways.map((gw) => (
              <tr key={gw.gateway_id}>
                <td><code style={{ fontSize: 11 }}>{gw.gateway_id}</code></td>
                <td>
                  {editing?.gateway_id === gw.gateway_id ? (
                    <input value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
                  ) : gw.name}
                </td>
                <td>
                  {editing?.gateway_id === gw.gateway_id ? (
                    <input value={editing.host} onChange={(e) => setEditing({ ...editing, host: e.target.value })} />
                  ) : gw.host}
                </td>
                <td>
                  {editing?.gateway_id === gw.gateway_id ? (
                    <input type="number" value={editing.port} onChange={(e) => setEditing({ ...editing, port: parseInt(e.target.value, 10) })} />
                  ) : gw.port}
                </td>
                <td>
                  {editing?.gateway_id === gw.gateway_id ? (
                    <input value={editing.region} onChange={(e) => setEditing({ ...editing, region: e.target.value })} />
                  ) : gw.region}
                </td>
                <td>
                  <span className={`badge ${gw.enabled ? "badge-green" : "badge-red"}`}>
                    {gw.enabled ? t("admin.channelEnabled") : t("admin.channelDisabled")}
                  </span>
                </td>
                <td style={{ fontSize: 12 }}>{new Date(gw.created_at).toLocaleString()}</td>
                <td>
                  <div className="action-bar">
                    {editing?.gateway_id === gw.gateway_id ? (
                      <>
                        <button className="btn btn-primary btn-sm" onClick={handleSaveEdit}>{t("admin.gatewaySaveBtn")}</button>
                        <button className="btn btn-ghost btn-sm" onClick={() => setEditing(null)}>{t("admin.gatewayCancelBtn")}</button>
                      </>
                    ) : (
                      <>
                        <button className="btn btn-icon" onClick={() => handleEdit(gw)} title={t("admin.gatewayEditTooltip")}>✏️</button>
                        <button className="btn btn-icon" onClick={() => handleToggle(gw)} title={gw.enabled ? t("admin.gatewayToggleDisable") : t("admin.gatewayToggleEnable")}>
                          {gw.enabled ? "⏸" : "▶️"}
                        </button>
                        <button className="btn btn-icon danger" onClick={() => handleDelete(gw)} title={t("admin.gatewayDeleteTooltip")}>🗑️</button>
                      </>
                    )}
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