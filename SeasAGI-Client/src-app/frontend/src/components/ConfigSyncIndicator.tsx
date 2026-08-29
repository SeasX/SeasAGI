import { useAppStore } from "../stores/appStore";
import { useTranslation } from "../i18n";

export function ConfigSyncIndicator() {
  const syncState = useAppStore((s) => s.syncState);
  const cloudBilling = useAppStore((s) => s.cloudBilling);
  const { t } = useTranslation();

  const isTeamsOrAbove = cloudBilling?.plan_id === "teams" || cloudBilling?.plan_id === "enterprise";
  if (!isTeamsOrAbove) return null;

  const statusMap = {
    idle: { label: "", color: "transparent", bg: "transparent" },
    syncing: { label: t("layout.syncing"), color: "var(--accent)", bg: "rgba(59,130,246,0.08)" },
    synced: { label: t("layout.syncedVersion", { version: syncState.config_version }), color: "var(--green)", bg: "rgba(34,197,94,0.08)" },
    error: { label: t("layout.syncFailed"), color: "var(--red)", bg: "rgba(239,68,68,0.08)" },
  };

  const info = statusMap[syncState.status];
  if (!info.label) return null;

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 6,
        padding: "3px 10px",
        fontSize: 11,
        fontWeight: 500,
        borderRadius: "var(--radius)",
        background: info.bg,
        color: info.color,
        flexShrink: 0,
      }}
    >
      <span style={{ width: 6, height: 6, borderRadius: "50%", background: info.color, flexShrink: 0 }} />
      <span>{info.label}</span>
      {syncState.policy_conflicts.length > 0 && (
        <span title={syncState.policy_conflicts.map((c) => `${c.field}: ${c.local_value} → ${c.remote_value}`).join("; ")}>
          ⚠️ {syncState.policy_conflicts.length}
        </span>
      )}
      {syncState.enterprise_locked && <span title={t("layout.policyLocked")}>🔒</span>}
      {syncState.force_applied && <span title={t("layout.forceApplied")}>📋</span>}
    </div>
  );
}
