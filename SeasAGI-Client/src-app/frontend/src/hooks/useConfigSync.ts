import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "../stores/appStore";
import { platformRequest, asPlatformRecord } from "../utils/commands";

// 轮询间隔为常量，保证 effect 依赖收敛到稳定值（不随渲染变化而重置定时器）。
const SYNC_INTERVAL_MS = 60000;

export function useConfigSync() {
  const cloudBilling = useAppStore((s) => s.cloudBilling);
  const setSyncState = useAppStore((s) => s.setSyncState);
  const appConfig = useAppStore((s) => s.appConfig);
  const syncState = useAppStore((s) => s.syncState);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // 用 ref 持有最新配置，供同步逻辑读取；避免把配置写回自身依赖导致重复同步。
  const appConfigRef = useRef(appConfig);
  appConfigRef.current = appConfig;

  const isTeamsOrAbove = cloudBilling?.plan_id === "teams" || cloudBilling?.plan_id === "enterprise";
  const isEnterprise = cloudBilling?.plan_id === "enterprise";
  // 配置异步加载完成后再开始同步，避免首个定时器建立在空配置上。
  const configReady = appConfig !== null;

  const doSync = useCallback(async () => {
    if (!isTeamsOrAbove) return;
    setSyncState({ status: "syncing" });
    try {
      const currentConfig = appConfigRef.current;
      const syncR = await platformRequest("GET", "/tenant-admin/config/sync");
      if (syncR.status >= 400) { setSyncState({ status: "error" }); return; }
      const syncData = asPlatformRecord(syncR.body);
      const remoteVersion = syncData.config_version || 0;

      const conflicts: { field: string; local_value: string; remote_value: string; severity: "info" | "warning" | "error" }[] = [];

      if (currentConfig && syncData.strategies?.length > 0) {
        const localStrategy = currentConfig.routing_strategy || "fallback";
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
          const pushR = await platformRequest("GET", "/enterprise/config/push");
          if (pushR.status < 400) {
            const pushData = asPlatformRecord(pushR.body);
            if (pushData.model_policy?.allowed_models) {
              const allowed = pushData.model_policy.allowed_models;
              if (currentConfig && !JSON.stringify(currentConfig.model_combos).includes(allowed.replace(/^\[|\]$/g, ""))) {
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
  }, [isTeamsOrAbove, isEnterprise, setSyncState]);

  useEffect(() => {
    if (!isTeamsOrAbove || !configReady) return;
    void doSync();
    if (intervalRef.current) clearInterval(intervalRef.current);
    intervalRef.current = setInterval(() => {
      void doSync();
    }, SYNC_INTERVAL_MS);
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [isTeamsOrAbove, configReady, doSync]);

  return { doSync, conflicts: syncState.policy_conflicts };
}
