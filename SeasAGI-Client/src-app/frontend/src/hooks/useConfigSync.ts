import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "../stores/appStore";
import { getPlatformAPIBaseURL, getPlatformToken } from "../utils/commands";

export function useConfigSync() {
  const cloudBilling = useAppStore((s) => s.cloudBilling);
  const setSyncState = useAppStore((s) => s.setSyncState);
  const appConfig = useAppStore((s) => s.appConfig);
  const syncState = useAppStore((s) => s.syncState);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isTeamsOrAbove = cloudBilling?.plan_id === "teams" || cloudBilling?.plan_id === "enterprise";
  const isEnterprise = cloudBilling?.plan_id === "enterprise";

  const doSync = useCallback(async () => {
    if (!isTeamsOrAbove) return;
    setSyncState({ status: "syncing" });
    try {
      const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
      if (!token) { setSyncState({ status: "idle" }); return; }
      const headers: Record<string, string> = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
      const syncR = await fetch(baseURL + "/tenant-admin/config/sync", { headers });
      if (!syncR.ok) { setSyncState({ status: "error" }); return; }
      const syncData = await syncR.json();
      const remoteVersion = syncData.config_version || 0;

      const conflicts: { field: string; local_value: string; remote_value: string; severity: "info" | "warning" | "error" }[] = [];

      if (appConfig && syncData.strategies?.length > 0) {
        const localStrategy = appConfig.routing_strategy || "fallback";
        const remoteStrategy = syncData.strategies[0]?.strategy_type || "";
        if (remoteStrategy && localStrategy !== remoteStrategy) {
          conflicts.push({
            field: "routing_strategy",
            local_value: localStrategy,
            remote_value: remoteStrategy,
            severity: isEnterprise ? "error" : "warning",
          });
        }
      }

      if (isEnterprise) {
        try {
          const pushR = await fetch(baseURL + "/enterprise/config/push", { headers });
          if (pushR.ok) {
            const pushData = await pushR.json();
            if (pushData.model_policy?.allowed_models) {
              const allowed = pushData.model_policy.allowed_models;
              if (appConfig && !JSON.stringify(appConfig.model_combos).includes(allowed.replace(/^\[|\]$/g, ""))) {
                conflicts.push({
                  field: "allowed_models",
                  local_value: "custom",
                  remote_value: allowed,
                  severity: "error",
                });
              }
            }
          }
        } catch {}
      }

      setSyncState({
        status: "synced",
        config_version: remoteVersion,
        last_sync_at: new Date().toISOString(),
        policy_conflicts: conflicts,
        enterprise_locked: isEnterprise && conflicts.some((c) => c.severity === "error"),
        force_applied: false,
      });
    } catch {
      setSyncState({ status: "error" });
    }
  }, [isTeamsOrAbove, isEnterprise, appConfig, syncState.config_version, setSyncState]);

  useEffect(() => {
    if (!isTeamsOrAbove) return;
    doSync();
    intervalRef.current = setInterval(doSync, 60000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [isTeamsOrAbove, doSync]);

  return { doSync, conflicts: syncState.policy_conflicts };
}