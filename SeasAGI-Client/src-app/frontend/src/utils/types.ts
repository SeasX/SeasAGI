export interface Channel {
  channel_id: string;
  channel_type: "platform" | "custom";
  provider_type: string;
  display_name: string;
  base_url: string;
  enabled: boolean;
  health_status: "unknown" | "healthy" | "unhealthy";
  source?: "enterprise" | "platform" | "";
  provider_specific_config: Record<string, string>;
  supported_modalities?: string[];
  models?: string[];
  api_key?: string;
  api_keys?: string[];
  retry_config?: RetryConfig;
  last_error?: string;
  extra?: Record<string, string>;
}

export interface RetryConfig {
  max_retries: number;
  initial_backoff_ms: number;
  max_backoff_ms: number;
  jitter: boolean;
}

export interface ModelMapping {
  logical_model_name: string;
  channel_id: string;
  upstream_model_name: string;
  is_default: boolean;
}

export interface DiscoveredModel {
  channel_id: string;
  model_id: string;
  model_name: string;
  discovered_at: string;
}

export interface RouteStep {
  channel_id: string;
  upstream_model: string;
  status: string;
  error?: string;
}

export interface RequestLog {
  request_id: string;
  created_at: string;
  logical_model_name: string;
  channel_id: string;
  channel_type: string; // "local" | "platform" | "relay" | "unknown"
  upstream_model: string;
  route_trace: string;
  route_steps: RouteStep[];
  status: "success" | "failure";
  duration_ms: number;
  error_code: string | null;
  error_message: string | null;
}

export interface CandidateProvider {
  channel_id: string;
  model: string;
  priority: number;
  health_status?: "unknown" | "healthy" | "unhealthy";
}

export interface RequestConstraints {
  max_price?: number;
  max_latency_ms?: number;
  min_throughput?: number;
  data_policy?: "local_only" | "cloud_only" | "any";
  zdr_required?: boolean;
  allow_cross_provider_fallback?: boolean;
  allow_cross_step_fallback?: boolean;
}

export interface ModelComboStep {
  channel_id?: string;
  model: string;
  step_role?: "primary" | "backup" | "last_resort";
  providers?: CandidateProvider[];
  channels?: string[];
  selection_policy?: "stability" | "cost" | "latency" | "throughput";
  allow_provider_fallback?: boolean;
  allow_cross_provider_fallback?: boolean;
}

export interface ModelCombo {
  combo_id?: string;
  name: string;
  logical_name?: string;
  display_name?: string;
  description?: string;
  tags?: string[];
  steps: ModelComboStep[];
  models?: string[];
  strategy: string;
  sticky_uses: number;
  quick_strategy?: QuickStrategy["alias"];
  task_profile?: TaskProfile;
  status?: "draft" | "active" | "archived";
  source?: "local" | "official" | "tenant" | "custom" | "cloud";
  version?: number;
}

export interface AppConfig {
  listen_port: number;
  default_model: string;
  default_channel_id: string;
  default_combo_name?: string;
  routing_strategy: string;
  sticky_channel_use: number;
  auto_launch: boolean;
  auto_update: boolean;
  log_retention_days: number;
  analytics_enabled: boolean;
  locale: string;
  rtk_enabled: boolean;
  rtk_max_output_chars: number;
  caveman_enabled: boolean;
  caveman_style: string;
  model_combos: ModelCombo[];
  combo_templates: ModelCombo[];
  platform_api_base_url: string;
}

export interface AuthState {
  is_logged_in: boolean;
  user_id: string | null;
  email: string | null;
}

export interface LoginStatus {
  IsLoggedIn: boolean;
  UserID?: string;
  Email?: string;
}

export interface LoginResult {
  accessToken: string;
  refreshToken: string;
}

export interface CloudUsage {
  month_requests: number;
  month_input_tokens: number;
  month_output_tokens: number;
  total_cost_usd: number;
}

export interface CloudBilling {
  plan_id: string;
  plan_name: string;
  price: number;
  quota: number;
  used_quota: number;
  renewal_date: string;
  relay_enabled: boolean;
  relay_gateways?: RelayGateway[];
  overage_cost?: number;
  overage_requests?: number;
}

export interface PolicyConflict {
  field: string;
  local_value: string;
  remote_value: string;
  severity: "info" | "warning" | "error";
}

export interface SyncState {
  status: "idle" | "syncing" | "synced" | "error";
  config_version: number;
  last_sync_at: string | null;
  force_applied: boolean;
  policy_conflicts: PolicyConflict[];
  enterprise_locked: boolean;
}

export interface RelayGateway {
  gateway_id: string;
  name: string;
  region: string;
  host: string;
  port: number;
  supports_federation?: boolean;
}

export interface OverageRecord {
  overage_id: string;
  user_id: string;
  plan_id: string;
  billing_period: string;
  overage_requests: number;
  overage_cost: number;
  currency: string;
  billed: boolean;
  invoice_id: string;
  created_at: string;
}

export type PlanTier = "free" | "pro" | "enterprise";

export interface CloudCombo {
  name: string;
  models: string[];
  description: string;
  strategy: string;
}

export interface OAuthProvider {
  name: string;
  displayName: string;
  authURL: string;
  iconURL: string;
}

export interface OAuthConnection {
  name: string;
  displayName: string;
  iconURL: string;
  connected: boolean;
  connecting: boolean;
  configured: boolean;
  clientIDMask: string;
  expiresAt: string | null;
  error: string;
}

export interface OptimizationRecommendation {
  from_model: string;
  to_model: string;
  model_tag: string;
  channel_id: string;
  channel_name: string;
  savings_usd: number;
  quality_diff: string;
  avg_latency_ms: number;
  error_rate: number;
  reason: string;
}

export interface OptimizationPlan {
  recommendations: OptimizationRecommendation[];
  monthly_savings: number;
  strategy: string;
  mode: string;
  task_type?: TaskProfile["task_type"];
}

export interface OptimizationConfig {
  mode: string;
  penalty_enabled: boolean;
  penalty_decay_sec: number;
  health_check_enabled: boolean;
  health_check_sec: number;
  health_max_failures: number;
  cooldown_enabled: boolean;
  cooldown_sec: number;
  sticky_enabled: boolean;
  sticky_ttl_sec: number;
  preset_enabled: boolean;
  default_preset: string;
}

export interface ModelStatsEntry {
  model: string;
  total_requests: number;
  total_errors: number;
  avg_latency_ms: number;
  error_rate: number;
}

export interface UsageSummary {
  month_requests: number;
  month_input_tokens: number;
  month_output_tokens: number;
  month_cost_usd: number;
}

export interface RuntimeStatus {
  gateway_running: boolean;
  listen_port: number;
  default_model: string | null;
  default_channel_id: string | null;
  default_combo_name?: string | null;
}

export interface ProviderHealthMetric {
  metric_id: string;
  provider_id: string;
  channel_id: string;
  tenant_id: string;
  success_rate: number;
  error_rate: number;
  avg_latency_ms: number;
  total_requests: number;
  total_errors: number;
  window_error_rate: number;
  is_circuit_open: boolean;
  cooldown_until?: string | null;
  last_error_at?: string | null;
  last_success_at?: string | null;
  penalty_score: number;
  recorded_at: string;
}

export interface ProviderHealthSummary {
  provider_id: string;
  total_requests: number;
  total_errors: number;
  avg_success_rate: number;
  avg_error_rate: number;
  avg_latency_ms: number;
  avg_window_error_rate: number;
  avg_penalty_score: number;
  circuit_open_count: number;
}

export interface BYOKPolicy {
  policy_id: string;
  enterprise_id: string;
  tenant_id: string;
  priority_channels: string[];
  allow_fallback: boolean;
  fallback_order: string[];
  cross_boundary_fallback: boolean;
  enterprise_only: boolean;
  max_cost_per_request: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface TaskProfile {
  task_type: "general_chat" | "tool_calling" | "structured_output" | "long_context" | "vision";
  priority_providers: string[];
  fallback_order: string[];
  max_cost_per_request?: number;
  max_latency_ms?: number;
  min_success_rate?: number;
}

export interface QuickStrategy {
  alias: "stable_first" | "cost_first" | "speed_first" | "tools_first";
  display_name: string;
  description: string;
  task_profile: TaskProfile;
  combo_constraints?: {
    min_steps?: number;
    max_steps?: number;
    allowed_strategies?: string[];
  };
}

// MITM 代理状态
export interface MITMStatus {
  state: "stopped" | "starting" | "running" | "stopping" | "error";
  proxy_port: number;
  ca_installed: boolean;
  rules_count: number;
  system_proxy: boolean;
  system_proxy_active: boolean;
  residual_system_proxy: boolean;
  last_error?: string;
}
