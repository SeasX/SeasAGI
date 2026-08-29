import { create } from "zustand";
import type { AuthState, RuntimeStatus, Channel, RequestLog, AppConfig, ModelCombo, CloudBilling, CloudUsage, SyncState, ProviderHealthSummary, BYOKPolicy, QuickStrategy, TaskProfile } from "../utils/types";

interface AppStore {
  auth: AuthState;
  runtime: RuntimeStatus;
  channels: Channel[];
  logs: RequestLog[];
  appConfig: AppConfig | null;
  combos: ModelCombo[];
  templates: ModelCombo[];
  cloudBilling: CloudBilling | null;
  cloudUsage: CloudUsage | null;
  syncState: SyncState;
  defaultComboName: string;
  taskType: "chat" | "tools" | "json" | "long_context";
  providerHealth: ProviderHealthSummary[];
  byokPolicies: BYOKPolicy[];
  quickStrategies: QuickStrategy[];
  taskProfiles: TaskProfile[];
  activeGrants: Record<string, any>[];
  selectedGrant: Record<string, any> | null;
  setAuth: (auth: AuthState) => void;
  setRuntime: (runtime: RuntimeStatus) => void;
  setChannels: (channels: Channel[]) => void;
  setLogs: (logs: RequestLog[]) => void;
  setAppConfig: (config: AppConfig) => void;
  setCombos: (combos: ModelCombo[]) => void;
  setTemplates: (templates: ModelCombo[]) => void;
  setCloudBilling: (billing: CloudBilling | null) => void;
  setCloudUsage: (usage: CloudUsage | null) => void;
  setSyncState: (sync: Partial<SyncState>) => void;
  setDefaultComboName: (name: string) => void;
  setTaskType: (taskType: "chat" | "tools" | "json" | "long_context") => void;
  setProviderHealth: (providerHealth: ProviderHealthSummary[]) => void;
  setByokPolicies: (byokPolicies: BYOKPolicy[]) => void;
  setQuickStrategies: (quickStrategies: QuickStrategy[]) => void;
  setTaskProfiles: (taskProfiles: TaskProfile[]) => void;
  setActiveGrants: (grants: Record<string, any>[]) => void;
  setSelectedGrant: (grant: Record<string, any> | null) => void;
}

export const useAppStore = create<AppStore>((set) => ({
  auth: { is_logged_in: false, user_id: null, email: null },
  runtime: { gateway_running: false, listen_port: 4318, default_model: null, default_channel_id: null, default_combo_name: null },
  channels: [],
  logs: [],
  appConfig: null,
  combos: [],
  templates: [],
  cloudBilling: null,
  cloudUsage: null,
  defaultComboName: "",
  taskType: "chat",
  providerHealth: [],
  byokPolicies: [],
  quickStrategies: [],
  taskProfiles: [],
  activeGrants: [],
  selectedGrant: null,
  syncState: {
    status: "idle",
    config_version: 0,
    last_sync_at: null,
    force_applied: false,
    policy_conflicts: [],
    enterprise_locked: false,
  },
  setAuth: (auth) => set({ auth }),
  setRuntime: (runtime) => set({ runtime }),
  setChannels: (channels) => set({ channels }),
  setLogs: (logs) => set({ logs }),
  setAppConfig: (appConfig) => set({ appConfig }),
  setCombos: (combos) => set({ combos }),
  setTemplates: (templates) => set({ templates }),
  setCloudBilling: (cloudBilling) => set({ cloudBilling }),
  setCloudUsage: (cloudUsage) => set({ cloudUsage }),
  setSyncState: (sync) => set((s) => ({ syncState: { ...s.syncState, ...sync } })),
  setDefaultComboName: (name) => set({ defaultComboName: name }),
  setTaskType: (taskType) => set({ taskType }),
  setProviderHealth: (providerHealth) => set({ providerHealth }),
  setByokPolicies: (byokPolicies) => set({ byokPolicies }),
  setQuickStrategies: (quickStrategies) => set({ quickStrategies }),
  setTaskProfiles: (taskProfiles) => set({ taskProfiles }),
  setActiveGrants: (activeGrants) => set({ activeGrants }),
  setSelectedGrant: (selectedGrant) => set({ selectedGrant }),
}));
