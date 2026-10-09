import type { Channel, DiscoveredModel, RequestLog, RuntimeStatus, AuthState, AppConfig, ModelCombo, CloudUsage, CloudBilling, CloudCombo, OptimizationPlan, OptimizationConfig, ModelStatsEntry, UsageSummary, ProviderHealthMetric, ProviderHealthSummary, BYOKPolicy, QuickStrategy, TaskProfile, MITMStatus, OverageRecord, Invoice, OAuthProvider } from "../utils/types";

type WailsAppApi = {
  StartLocalGateway(): Promise<void>;
  StopLocalGateway(): Promise<void>;
  GetRuntimeStatus(): Promise<RuntimeStatus>;
  Login(email: string, password: string): Promise<void>;
  Logout(): Promise<void>;
  Register(email: string, password: string, displayName: string): Promise<void>;
  GetOAuthProviders(): Promise<OAuthProvider[]>;
  StartOAuthLogin(provider: string): Promise<void>;
  GetCloudUsage(): Promise<CloudUsage>;
  GetCloudBilling(): Promise<CloudBilling>;
  GetOverageUsage(): Promise<OverageRecord | null>;
  PlatformRequest(method: string, path: string, body: string): Promise<{ status: number; body: any }>;
  GetRecommendedCombos(): Promise<CloudCombo[]>;
  GetOptimizationPlan(mode: string, taskType: string): Promise<OptimizationPlan>;
  GetUsageSummary(): Promise<UsageSummary>;
  GetOptimizationConfig(): Promise<OptimizationConfig>;
  SetOptimizationConfig(cfg: Record<string, any>): Promise<void>;
  ApplyRecommendation(toModel: string, preset: string): Promise<void>;
  PreviewComboOptimization(mode: string, taskType: string): Promise<any>;
  ApplyComboOptimization(toModel: string, preset: string, updateExisting: boolean): Promise<void>;
  GetComboRouteMetrics(): Promise<any[]>;
  SimulateIntentRouting(prompt: string, model: string): Promise<any>;
  GetIntentScenarioStats(): Promise<any[]>;
  GetDefaultComboName(): Promise<string>;
  SetDefaultComboName(name: string): Promise<void>;
  GetComboByName(name: string): Promise<ModelCombo>;
  SyncOptimizationConfigToCloud(): Promise<void>;
  SyncOptimizationConfigFromCloud(): Promise<OptimizationConfig | null>;
  FetchModelStats(): Promise<ModelStatsEntry[]>;
  FetchModelsFromURL(modelsURL: string, apiKey: string): Promise<{ models: string[]; total: number }>;
  GetAuthState(): Promise<AuthState>;
  ListChannels(): Promise<Channel[]>;
  SaveCustomChannel(channel: Channel): Promise<string>;
  DeleteCustomChannel(channelId: string): Promise<void>;
  TestCustomChannel(channelId: string): Promise<Record<string, any>>;
  TestChannelDirect(channelId: string): Promise<Record<string, any>>;
  DiscoverModels(channelId: string): Promise<DiscoveredModel[]>;
  ListLogs(limit: number, offset: number): Promise<RequestLog[]>;
  ListLogsFiltered(limit: number, offset: number, status: string, channelId: string, timeFrom: string, timeTo: string, keyword: string): Promise<RequestLog[]>;
  UpdateDefaultModel(modelName: string, channelId: string): Promise<void>;
  GetLocalAccessToken(): Promise<string>;
  ResetLocalAccessToken(): Promise<string>;
  SetAutoLaunch(enabled: boolean): Promise<void>;
  UpdateRoutingSettings(strategy: string, stickyUses: number): Promise<void>;
  ListModelCombos(): Promise<ModelCombo[]>;
  SaveModelCombo(combo: ModelCombo): Promise<void>;
  DeleteModelCombo(name: string): Promise<void>;
  ApplyComboSortPreset(comboName: string, preset: string): Promise<void>;
  ListComboTemplates(): Promise<ModelCombo[]>;
  SaveComboTemplate(template: ModelCombo): Promise<void>;
  DeleteComboTemplate(name: string): Promise<void>;
  RenameComboTemplate(oldName: string, newName: string): Promise<void>;
  GetAppConfig(): Promise<AppConfig>;
  SetRTKSettings(rtkEnabled: boolean, rtkMaxOutputChars: number, cavemanEnabled: boolean, cavemanStyle: string): Promise<void>;
  QuitApp(): Promise<void>;
  SetLocale(locale: string): Promise<void>;
  GetLocale(): Promise<string>;
  GetPlans(): Promise<Record<string, any>[]>;
  FetchActiveGrants(): Promise<Record<string, any>[]>;
  SetSelectedGrant(grantID: string, relayURL: string): Promise<void>;
  GetSelectedGrant(): Promise<Record<string, any>>;
  ClearSelectedGrant(): Promise<void>;
  ChatCompletion(messages: Record<string, any>[], model: string): Promise<Record<string, any>>;
  ChatCompletionForChannel(channelId: string, messages: Record<string, any>[], model: string): Promise<Record<string, any>>;
  ImageGeneration(prompt: string, model: string, params: Record<string, any>): Promise<Record<string, any>>;
  ImageGenerationForChannel(channelId: string, prompt: string, model: string, params: Record<string, any>): Promise<Record<string, any>>;
  VideoGeneration(prompt: string, model: string, params: Record<string, any>): Promise<Record<string, any>>;
  VideoGenerationForChannel(channelId: string, prompt: string, model: string, params: Record<string, any>): Promise<Record<string, any>>;
  SyncCustomChannelsToCloud(channels: Record<string, any>[]): Promise<{ message: string; count: number }>;
  GetOfficialComboTemplates(): Promise<ModelCombo[]>;
  FetchCloudCombos(): Promise<Record<string, any>[]>;
  PushCloudCombo(logicalName: string, displayName: string, description: string, strategy: string, stickyUses: number, quickStrategy: string, taskProfile: Record<string, any>, stepsJSON: string): Promise<Record<string, any>>;
  UpdateCloudCombo(comboID: string, displayName: string, description: string, strategy: string, stickyUses: number, status: string, quickStrategy: string, taskProfile: Record<string, any>, stepsJSON: string): Promise<void>;
  DeleteCloudCombo(comboID: string): Promise<void>;
  ClearLogs(): Promise<void>;
  DeleteSession(app: string, sessionID: string): Promise<void>;
  SyncPlatformChannels(): Promise<void>;
  SyncLocalDataToCloud(): Promise<void>;
  OpenInBrowser(url: string): Promise<void>;
  CreateCheckoutSession(planId: string, quantity?: number): Promise<Record<string, any>>;
  GetPlatformAPIBaseURL(): Promise<string>;
  SetPlatformAPIBaseURL(url: string): Promise<void>;
  GetPlatformToken(): Promise<string>;
  StartTunnel(tunnelType: string): Promise<void>;
  StopTunnel(): Promise<void>;
  IsTunnelRunning(): Promise<boolean>;
  GetTunnelStatus(): Promise<Record<string, unknown>>;
  GetTunnelURL(): Promise<string>;
  GetProviderHealthMetrics(providerId?: string): Promise<ProviderHealthMetric[]>;
  GetProviderHealthSummary(): Promise<ProviderHealthSummary[]>;
  GetByokPolicies(): Promise<BYOKPolicy[]>;
  SetByokPolicy(policy: Record<string, any>): Promise<BYOKPolicy>;
  GetQuickStrategies(): Promise<QuickStrategy[]>;
  GetTaskProfiles(): Promise<TaskProfile[]>;
  StartMITM(): Promise<void>;
  StopMITM(): Promise<void>;
  IsMITMRunning(): Promise<boolean>;
  GetMITMStatus(): Promise<MITMStatus>;
  GetMITMRules(): Promise<string[]>;
  AddMITMRule(domain: string): Promise<void>;
  RemoveMITMRule(domain: string): Promise<void>;
  GetMITMRecentIntercepts(n: number): Promise<Record<string, any>[]>;
  GetMITMEnvHint(): Promise<Record<string, string>>;
  TestMITMDomain(domain: string): Promise<Record<string, any>>;
  RunDiagnostics(): Promise<Record<string, any>>;
  ListEvalSuites(): Promise<Record<string, any>[]>;
  CreateEvalSuite(suite: Record<string, any>): Promise<Record<string, any>>;
  RunEvalSuite(suiteID: string): Promise<Record<string, any>>;
  GetEvalScorecard(suiteID: string): Promise<Record<string, any>>;
  ListPlugins(): Promise<Record<string, any>[]>;
  TogglePlugin(pluginName: string, enabled: boolean): Promise<boolean>;
  GetPluginHooks(): Promise<Record<string, any>[]>;
  GetPluginAuditLog(): Promise<Record<string, any>[]>;
  GetMITMTargets(): Promise<Record<string, any>[]>;
  SearchNotion(apiKey: string, query: string): Promise<Record<string, any>>;
  SearchObsidian(apiKey: string, baseURL: string, query: string): Promise<Record<string, any>[]>;
  GetPerfAuditReport(): Promise<Record<string, any>>;
  GetMCPGatewayTools(): Promise<Record<string, any>[]>;
  GetMCPAuditLog(): Promise<Record<string, any>[]>;
  GetLogRotationConfig(): Promise<Record<string, any>>;
  SetLogRotationConfig(config: Record<string, any>): Promise<void>;
  GetCloudSyncStatus(): Promise<Record<string, any>>;
  FetchFreeChannels(): Promise<Record<string, any>[]>;
  FetchEnterpriseChannels(): Promise<Record<string, any>[]>;
  FetchModelCatalog(): Promise<Record<string, any>[]>;
  FetchModelIndex(category: string): Promise<Record<string, any>>;
  FetchMITMTargetsFromEnterprise(): Promise<Record<string, any>>;
  GetRateLimitConfig(): Promise<Record<string, any>>;
  SetRateLimitConfig(config: Record<string, any>): Promise<void>;
};

function getAppApi(): WailsAppApi {
  const win = window as unknown as {
    go?: {
      main?: { App?: WailsAppApi };
      seasagi?: { App?: WailsAppApi };
    };
  };
  const api = win.go?.main?.App ?? win.go?.seasagi?.App;
  if (!api) {
    throw new Error("Wails App bindings are not available");
  }
  return api;
}

export async function startLocalGateway(): Promise<void> {
  return getAppApi().StartLocalGateway();
}

export async function stopLocalGateway(): Promise<void> {
  return getAppApi().StopLocalGateway();
}

export async function getRuntimeStatus(): Promise<RuntimeStatus> {
  return getAppApi().GetRuntimeStatus();
}

export async function login(email: string, password: string): Promise<void> {
  return getAppApi().Login(email, password);
}

export async function logout(): Promise<void> {
  return getAppApi().Logout();
}

export async function getAuthState(): Promise<AuthState> {
  return getAppApi().GetAuthState();
}

export async function listChannels(): Promise<Channel[]> {
  return getAppApi().ListChannels();
}

export async function saveCustomChannel(channel: Channel): Promise<string> {
  return getAppApi().SaveCustomChannel(channel);
}

export async function deleteCustomChannel(channelId: string): Promise<void> {
  return getAppApi().DeleteCustomChannel(channelId);
}

export async function testCustomChannel(channelId: string): Promise<Record<string, any>> {
  return getAppApi().TestCustomChannel(channelId);
}

export async function testChannelDirect(channelId: string): Promise<Record<string, any>> {
  return getAppApi().TestChannelDirect(channelId);
}

export async function syncPlatformChannels(): Promise<void> {
  return getAppApi().SyncPlatformChannels();
}

export async function syncCustomChannelsToCloud(channels: Record<string, any>[]): Promise<{ message: string; count: number }> {
  return getAppApi().SyncCustomChannelsToCloud(channels);
}

export async function getPlans(): Promise<Record<string, any>[]> {
  return getAppApi().GetPlans();
}

export async function chatCompletion(messages: Record<string, any>[], model: string): Promise<Record<string, any>> {
  return getAppApi().ChatCompletion(messages, model);
}

export async function chatCompletionForChannel(channelId: string, messages: Record<string, any>[], model: string): Promise<Record<string, any>> {
  return getAppApi().ChatCompletionForChannel(channelId, messages, model);
}

export async function imageGeneration(prompt: string, model: string, params: Record<string, any> = {}): Promise<Record<string, any>> {
  return getAppApi().ImageGeneration(prompt, model, params);
}

export async function imageGenerationForChannel(channelId: string, prompt: string, model: string, params: Record<string, any> = {}): Promise<Record<string, any>> {
  return getAppApi().ImageGenerationForChannel(channelId, prompt, model, params);
}

export async function videoGeneration(prompt: string, model: string, params: Record<string, any> = {}): Promise<Record<string, any>> {
  return getAppApi().VideoGeneration(prompt, model, params);
}

export async function videoGenerationForChannel(channelId: string, prompt: string, model: string, params: Record<string, any> = {}): Promise<Record<string, any>> {
  return getAppApi().VideoGenerationForChannel(channelId, prompt, model, params);
}

export async function getUsageSummary(): Promise<UsageSummary> {
  return getAppApi().GetUsageSummary();
}

export async function syncLocalDataToCloud(): Promise<void> {
  return getAppApi().SyncLocalDataToCloud();
}

export async function openInBrowser(url: string): Promise<void> {
  return getAppApi().OpenInBrowser(url);
}

export async function createCheckoutSession(planId: string, quantity?: number): Promise<Record<string, any>> {
  if (quantity && quantity > 1) {
    return getAppApi().CreateCheckoutSession(planId, quantity);
  }
  return getAppApi().CreateCheckoutSession(planId);
}

export interface PlatformResponse {
  status: number;
  body: any;
}

// platformRequest 统一经 Go 后端代理平台 API：前端不再持有并外发平台 Token。
// method: HTTP 方法；path: 平台相对路径（须以 / 开头）；body: 可选请求体（会自动 JSON 序列化）。
export async function platformRequest(method: string, path: string, body?: unknown): Promise<PlatformResponse> {
  const payload = body === undefined || body === null ? "" : JSON.stringify(body);
  return getAppApi().PlatformRequest(method, path, payload);
}

export async function getPlatformAPIBaseURL(): Promise<string> {
  return getAppApi().GetPlatformAPIBaseURL();
}

export async function getPlatformToken(): Promise<string> {
  return getAppApi().GetPlatformToken();
}

export async function setPlatformAPIBaseURL(url: string): Promise<void> {
  return getAppApi().SetPlatformAPIBaseURL(url);
}

export async function discoverModels(channelId: string): Promise<DiscoveredModel[]> {
  return getAppApi().DiscoverModels(channelId);
}

export async function listLogs(limit: number, offset: number): Promise<RequestLog[]> {
  return getAppApi().ListLogs(limit, offset);
}

export async function listLogsFiltered(
  limit: number, offset: number,
  status: string, channelId: string,
  timeFrom: string, timeTo: string,
  keyword: string
): Promise<RequestLog[]> {
  return getAppApi().ListLogsFiltered(limit, offset, status, channelId, timeFrom, timeTo, keyword);
}

export async function updateDefaultModel(modelName: string, channelId: string): Promise<void> {
  return getAppApi().UpdateDefaultModel(modelName, channelId);
}

export async function getLocalAccessToken(): Promise<string> {
  return getAppApi().GetLocalAccessToken();
}

export async function resetLocalAccessToken(): Promise<string> {
  return getAppApi().ResetLocalAccessToken();
}

export async function setAutoLaunch(enabled: boolean): Promise<void> {
  return getAppApi().SetAutoLaunch(enabled);
}

export async function updateRoutingSettings(strategy: string, stickyUses: number): Promise<void> {
  return getAppApi().UpdateRoutingSettings(strategy, stickyUses);
}

export async function listModelCombos(): Promise<ModelCombo[]> {
  return getAppApi().ListModelCombos();
}

export async function saveModelCombo(combo: ModelCombo): Promise<void> {
  return getAppApi().SaveModelCombo(combo);
}

export async function deleteModelCombo(name: string): Promise<void> {
  return getAppApi().DeleteModelCombo(name);
}

export async function applyComboSortPreset(comboName: string, preset: string): Promise<void> {
  return getAppApi().ApplyComboSortPreset(comboName, preset);
}

export async function listComboTemplates(): Promise<ModelCombo[]> {
  return getAppApi().ListComboTemplates();
}

export async function saveComboTemplate(template: ModelCombo): Promise<void> {
  return getAppApi().SaveComboTemplate(template);
}

export async function deleteComboTemplate(name: string): Promise<void> {
  return getAppApi().DeleteComboTemplate(name);
}

export async function renameComboTemplate(oldName: string, newName: string): Promise<void> {
  return getAppApi().RenameComboTemplate(oldName, newName);
}

export async function getAppConfig(): Promise<AppConfig> {
  return getAppApi().GetAppConfig();
}

export async function setRTKSettings(
  rtkEnabled: boolean,
  rtkMaxOutputChars: number,
  cavemanEnabled: boolean,
  cavemanStyle: string
): Promise<void> {
  return getAppApi().SetRTKSettings(rtkEnabled, rtkMaxOutputChars, cavemanEnabled, cavemanStyle);
}

export async function quitApp(): Promise<void> {
  return getAppApi().QuitApp();
}

export async function setLocale(locale: string): Promise<void> {
  return getAppApi().SetLocale(locale);
}

export async function getLocale(): Promise<string> {
  return getAppApi().GetLocale();
}

export async function startTunnel(tunnelType: string): Promise<void> {
  return getAppApi().StartTunnel(tunnelType);
}

export async function stopTunnel(): Promise<void> {
  return getAppApi().StopTunnel();
}

export async function isTunnelRunning(): Promise<boolean> {
  return getAppApi().IsTunnelRunning();
}

export async function getTunnelStatus(): Promise<Record<string, unknown>> {
  return getAppApi().GetTunnelStatus();
}

export async function getTunnelURL(): Promise<string> {
  return getAppApi().GetTunnelURL();
}

export async function getOfficialComboTemplates(): Promise<ModelCombo[]> {
  return getAppApi().GetOfficialComboTemplates();
}

export async function fetchCloudCombos(): Promise<Record<string, any>[]> {
  return getAppApi().FetchCloudCombos();
}

export async function pushCloudCombo(
  logicalName: string,
  displayName: string,
  description: string,
  strategy: string,
  stickyUses: number,
  quickStrategy: string,
  taskProfile: Record<string, any>,
  stepsJSON: string
): Promise<Record<string, any>> {
  return getAppApi().PushCloudCombo(logicalName, displayName, description, strategy, stickyUses, quickStrategy, taskProfile, stepsJSON);
}

export async function updateCloudCombo(
  comboID: string,
  displayName: string,
  description: string,
  strategy: string,
  stickyUses: number,
  status: string,
  quickStrategy: string,
  taskProfile: Record<string, any>,
  stepsJSON: string
): Promise<void> {
  return getAppApi().UpdateCloudCombo(comboID, displayName, description, strategy, stickyUses, status, quickStrategy, taskProfile, stepsJSON);
}

export async function deleteCloudCombo(comboID: string): Promise<void> {
  return getAppApi().DeleteCloudCombo(comboID);
}

export async function clearLogs(): Promise<void> {
  return getAppApi().ClearLogs();
}

export async function deleteSession(app: string, sessionID: string): Promise<void> {
  return getAppApi().DeleteSession(app, sessionID);
}

export async function register(email: string, password: string, displayName: string): Promise<void> {
  return getAppApi().Register(email, password, displayName);
}

export async function getOAuthProviders(): Promise<OAuthProvider[]> {
  return getAppApi().GetOAuthProviders();
}

export async function startOAuthLogin(provider: string): Promise<void> {
  return getAppApi().StartOAuthLogin(provider);
}

export async function getCloudUsage(): Promise<CloudUsage> {
  return getAppApi().GetCloudUsage();
}

export async function getCloudBilling(): Promise<CloudBilling> {
  return getAppApi().GetCloudBilling();
}

export async function getOverageUsage(): Promise<OverageRecord | null> {
  return getAppApi().GetOverageUsage();
}

// getInvoices 经 Go 后端代理平台发票列表接口，返回当前用户的发票记录。
export async function getInvoices(): Promise<Invoice[]> {
  const res = await platformRequest("GET", "/invoices");
  if (res.status !== 200 || !res.body) return [];
  const data = Array.isArray(res.body) ? res.body : res.body.data;
  return (data as Invoice[]) || [];
}

export async function fetchActiveGrants(): Promise<Record<string, any>[]> {
  return getAppApi().FetchActiveGrants();
}

export async function setSelectedGrant(grantID: string, relayURL: string): Promise<void> {
  return getAppApi().SetSelectedGrant(grantID, relayURL);
}

export async function getSelectedGrant(): Promise<Record<string, any>> {
  return getAppApi().GetSelectedGrant();
}

export async function clearSelectedGrant(): Promise<void> {
  return getAppApi().ClearSelectedGrant();
}

export async function getRecommendedCombos(): Promise<CloudCombo[]> {
  return getAppApi().GetRecommendedCombos();
}

export async function getOptimizationPlan(mode?: string, taskType?: string): Promise<OptimizationPlan> {
  return getAppApi().GetOptimizationPlan(mode || "value_first", taskType || "general_chat");
}

export async function getQuickStrategies(): Promise<QuickStrategy[]> {
  return getAppApi().GetQuickStrategies();
}

export async function getTaskProfiles(): Promise<TaskProfile[]> {
  return getAppApi().GetTaskProfiles();
}

export async function getOptimizationConfig(): Promise<OptimizationConfig> {
  return getAppApi().GetOptimizationConfig();
}

export async function setOptimizationConfig(cfg: Record<string, any>): Promise<void> {
  return getAppApi().SetOptimizationConfig(cfg);
}

export async function applyRecommendation(toModel: string, preset: string): Promise<void> {
  return getAppApi().ApplyRecommendation(toModel, preset);
}

export async function previewComboOptimization(mode: string, taskType: string): Promise<any> {
  return getAppApi().PreviewComboOptimization(mode, taskType);
}

export async function applyComboOptimization(toModel: string, preset: string, updateExisting: boolean): Promise<void> {
  return getAppApi().ApplyComboOptimization(toModel, preset, updateExisting);
}

export async function getComboRouteMetrics(): Promise<any[]> {
  return getAppApi().GetComboRouteMetrics();
}

export async function simulateIntentRouting(prompt: string, model: string): Promise<any> {
  return getAppApi().SimulateIntentRouting(prompt, model);
}

export async function getIntentScenarioStats(): Promise<any[]> {
  return getAppApi().GetIntentScenarioStats();
}

export async function getDefaultComboName(): Promise<string> {
  return getAppApi().GetDefaultComboName();
}

export async function setDefaultComboName(name: string): Promise<void> {
  return getAppApi().SetDefaultComboName(name);
}

export async function getComboByName(name: string): Promise<ModelCombo> {
  return getAppApi().GetComboByName(name);
}

export async function syncOptimizationConfigToCloud(): Promise<void> {
  return getAppApi().SyncOptimizationConfigToCloud();
}

export async function syncOptimizationConfigFromCloud(): Promise<OptimizationConfig | null> {
  return getAppApi().SyncOptimizationConfigFromCloud();
}

export async function fetchModelStats(): Promise<ModelStatsEntry[]> {
  return getAppApi().FetchModelStats();
}

export async function discoverModelsFromURL(modelsURL: string, apiKey: string): Promise<{ models: string[]; total: number }> {
  return getAppApi().FetchModelsFromURL(modelsURL, apiKey);
}

export async function getProviderHealthMetrics(providerId?: string): Promise<ProviderHealthMetric[]> {
  return getAppApi().GetProviderHealthMetrics(providerId);
}

export async function getProviderHealthSummary(): Promise<ProviderHealthSummary[]> {
  return getAppApi().GetProviderHealthSummary();
}

export async function getByokPolicies(): Promise<BYOKPolicy[]> {
  return getAppApi().GetByokPolicies();
}

export async function setByokPolicy(policy: Record<string, any>): Promise<BYOKPolicy> {
  return getAppApi().SetByokPolicy(policy);
}

export async function startMITM(): Promise<void> {
  return getAppApi().StartMITM();
}

export async function stopMITM(): Promise<void> {
  return getAppApi().StopMITM();
}

export async function isMITMRunning(): Promise<boolean> {
  return getAppApi().IsMITMRunning();
}

export async function getMITMStatus(): Promise<MITMStatus> {
  return getAppApi().GetMITMStatus();
}

export async function getMITMRules(): Promise<string[]> {
  return getAppApi().GetMITMRules();
}

export async function addMITMRule(domain: string): Promise<void> {
  return getAppApi().AddMITMRule(domain);
}

export async function removeMITMRule(domain: string): Promise<void> {
  return getAppApi().RemoveMITMRule(domain);
}

export async function getMITMRecentIntercepts(n: number): Promise<Record<string, any>[]> {
  return getAppApi().GetMITMRecentIntercepts(n);
}

export async function getMITMEnvHint(): Promise<Record<string, string>> {
  return getAppApi().GetMITMEnvHint();
}

export async function testMITMDomain(domain: string): Promise<Record<string, any>> {
  return getAppApi().TestMITMDomain(domain);
}

export async function runDiagnostics(): Promise<Record<string, any>> {
  return getAppApi().RunDiagnostics();
}

export async function listEvalSuites(): Promise<Record<string, any>[]> {
  return getAppApi().ListEvalSuites();
}

export async function createEvalSuite(suite: Record<string, any>): Promise<Record<string, any>> {
  return getAppApi().CreateEvalSuite(suite);
}

export async function runEvalSuite(suiteID: string): Promise<Record<string, any>> {
  return getAppApi().RunEvalSuite(suiteID);
}

export async function getEvalScorecard(suiteID: string): Promise<Record<string, any>> {
  return getAppApi().GetEvalScorecard(suiteID);
}

export async function listPlugins(): Promise<Record<string, any>[]> {
  return getAppApi().ListPlugins();
}

export async function togglePlugin(pluginName: string, enabled: boolean): Promise<boolean> {
  return getAppApi().TogglePlugin(pluginName, enabled);
}

export async function getPluginHooks(): Promise<Record<string, any>[]> {
  return getAppApi().GetPluginHooks();
}

export async function getPluginAuditLog(): Promise<Record<string, any>[]> {
  return getAppApi().GetPluginAuditLog();
}

export async function getMITMTargets(): Promise<Record<string, any>[]> {
  return getAppApi().GetMITMTargets();
}

export async function fetchMITMTargetsFromEnterprise(): Promise<Record<string, any>> {
  return getAppApi().FetchMITMTargetsFromEnterprise();
}

export async function searchNotion(apiKey: string, query: string): Promise<Record<string, any>> {
  return getAppApi().SearchNotion(apiKey, query);
}

export async function searchObsidian(apiKey: string, baseURL: string, query: string): Promise<Record<string, any>[]> {
  return getAppApi().SearchObsidian(apiKey, baseURL, query);
}

export async function getPerfAuditReport(): Promise<Record<string, any>> {
  return getAppApi().GetPerfAuditReport();
}

export async function getMCPGatewayTools(): Promise<Record<string, any>[]> {
  return getAppApi().GetMCPGatewayTools();
}

export async function getMCPAuditLog(): Promise<Record<string, any>[]> {
  return getAppApi().GetMCPAuditLog();
}

export async function getLogRotationConfig(): Promise<Record<string, any>> {
  return getAppApi().GetLogRotationConfig();
}

export async function setLogRotationConfig(config: Record<string, any>): Promise<void> {
  return getAppApi().SetLogRotationConfig(config);
}

export async function getCloudSyncStatus(): Promise<Record<string, any>> {
  return getAppApi().GetCloudSyncStatus();
}

export async function fetchFreeChannels(): Promise<Record<string, any>[]> {
  return getAppApi().FetchFreeChannels();
}

export async function fetchEnterpriseChannels(): Promise<Record<string, any>[]> {
  return getAppApi().FetchEnterpriseChannels();
}

export async function fetchModelCatalog(): Promise<Record<string, any>[]> {
  return getAppApi().FetchModelCatalog();
}

export async function fetchModelIndex(category: string): Promise<Record<string, any>> {
  return getAppApi().FetchModelIndex(category);
}

export async function getRateLimitConfig(): Promise<Record<string, any>> {
  return getAppApi().GetRateLimitConfig();
}

export async function setRateLimitConfig(config: Record<string, any>): Promise<void> {
  return getAppApi().SetRateLimitConfig(config);
}
