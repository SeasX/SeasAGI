import { useEffect, useState, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import { getPlatformAPIBaseURL, getPlatformToken, openInBrowser } from "../utils/commands";
import { useTranslation } from "../i18n";

interface AuditLogEntry {
  id: number;
  actor_type: string;
  actor_id: string;
  action: string;
  target_type: string;
  target_id: string;
  result: string;
  ip: string;
  trace_id: string;
  metadata: string;
  created_at: string;
}

interface SSOConfig {
  config_id: string;
  protocol: string;
  idp_metadata_url: string;
  idp_entity_id: string;
  sp_entity_id: string;
  acs_url: string;
  force_sso: boolean;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

interface SCIMConfig {
  config_id: string;
  scim_token: string;
  scim_endpoint: string;
  last_sync_at: string | null;
  sync_status: string;
  enabled: boolean;
}

interface RoleItem {
  role_id: string;
  name: string;
  description: string;
  permissions: string;
  is_builtin: boolean;
  created_at: string;
  updated_at: string;
}

interface ModelPolicy {
  policy_id: string;
  allowed_models: string;
  denied_models: string;
  default_action: string;
  updated_at: string;
}

interface IPEntry {
  id: number;
  cidr: string;
  description: string;
  created_at: string;
}

interface ComplianceReport {
  report_id: string;
  report_type: string;
  period_start: string;
  period_end: string;
  status: string;
  file_path: string;
  file_format: string;
  created_at: string;
}

interface ComplianceSchedule {
  frequency: string;
  target_email: string;
  format: string;
  enabled: boolean;
}

interface SLAStatus {
  tenant_id: string;
  tier: string;
  uptime_pct: number;
  p95_latency_ms: number;
  error_rate_pct: number;
  last_check: string;
  status: string;
}

interface SLAEvent {
  id: number;
  event_type: string;
  description: string;
  start_at: string;
  resolved_at: string;
  severity: string;
  created_at: string;
}

interface SLATrendPoint {
  date: string;
  uptime_pct: number;
  p95_latency_ms: number;
  error_rate_pct: number;
}

interface AlertRule {
  rule_id: string;
  name: string;
  condition_expr: string;
  threshold: number;
  severity: string;
  enabled: boolean;
  created_at: string;
}

interface AlertEvent {
  event_id: string;
  rule_id: string;
  severity: string;
  message: string;
  resolved: boolean;
  created_at: string;
}

interface AuditRetention {
  retention_days: number;
  auto_archive: boolean;
}

type TabId = "overview" | "audit" | "identity" | "policies" | "compliance" | "sla" | "alerts" | "support" | "deployment";

export function EnterprisePortalPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const auth = useAppStore((s) => s.auth);
  const cloudBilling = useAppStore((s) => s.cloudBilling);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<TabId>("overview");

  const [auditLogs, setAuditLogs] = useState<AuditLogEntry[]>([]);
  const [auditActionFilter, setAuditActionFilter] = useState("");
  const [auditSearchQuery, setAuditSearchQuery] = useState("");
  const [auditRetention, setAuditRetention] = useState<AuditRetention>({ retention_days: 365, auto_archive: true });

  const [ssoConfig, setSSOConfig] = useState<SSOConfig | null>(null);
  const [scimConfig, setSCIMConfig] = useState<SCIMConfig | null>(null);
  const [roles, setRoles] = useState<RoleItem[]>([]);

  const [modelPolicy, setModelPolicy] = useState<ModelPolicy | null>(null);
  const [ipWhitelist, setIpWhitelist] = useState<IPEntry[]>([]);

  const [complianceReports, setComplianceReports] = useState<ComplianceReport[]>([]);
  const [complianceSchedule, setComplianceSchedule] = useState<ComplianceSchedule>({ frequency: "monthly", target_email: "", format: "pdf", enabled: false });

  const [slaStatus, setSLAStatus] = useState<SLAStatus | null>(null);
  const [slaEvents, setSLAEvents] = useState<SLAEvent[]>([]);
  const [slaTrend, setSLATrend] = useState<SLATrendPoint[]>([]);
  const [slaTrendDays, setSLATrendDays] = useState(7);

  const [alertRules, setAlertRules] = useState<AlertRule[]>([]);
  const [alertEvents, setAlertEvents] = useState<AlertEvent[]>([]);

  const [editingSSO, setEditingSSO] = useState(false);
  const [ssoProtocol, setSSOProtocol] = useState("saml");
  const [ssoIdpMetaUrl, setSSOIdpMetaUrl] = useState("");
  const [ssoIdpEntityId, setSSOIdpEntityId] = useState("");
  const [ssoIdpCert, setSSOIdpCert] = useState("");
  const [ssoForceSSO, setSSOForceSSO] = useState(false);
  const [ssoEnabled, setSSOEnabled] = useState(false);

  const [editingSCIM, setEditingSCIM] = useState(false);
  const [scimEndpoint, setSCIMEndpoint] = useState("");
  const [scimEnabled, setSCIMEnabled] = useState(false);

  const [, setEditingRole] = useState<RoleItem | null>(null);
  const [, setRoleName] = useState("");
  const [, setRoleDesc] = useState("");
  const [, setRolePerms] = useState("{}");

  const [, setEditingModelAllowed] = useState("[]");
  const [, setEditingModelDenied] = useState("[]");
  const [, setEditingModelDefault] = useState("allow");
  const [, setEditingModelPolicy] = useState(false);

  const [newIP, setNewIP] = useState({ cidr: "", description: "" });

  const [, setEditingAlertRule] = useState<AlertRule | null>(null);
  const [, setAlertRuleName] = useState("");
  const [, setAlertRuleCondition] = useState("rate_limit_exceeded");
  const [, setAlertRuleThreshold] = useState(0);
  const [, setAlertRuleSeverity] = useState("warning");

  const [, setSLAEventDialog] = useState(false);

  const [contractInfo, setContractInfo] = useState<any>(null);
  const [trialStatus, setTrialStatus] = useState<any>(null);

  const [dataRegions, setDataRegions] = useState<any[]>([]);
  const [deploymentType, setDeploymentType] = useState("cloud");
  const [licenseInfo, setLicenseInfo] = useState<any>(null);
  const [deploymentStatus, setDeploymentStatus] = useState<any>(null);
  const [editingRegion, setEditingRegion] = useState(false);
  const [selectedRegion, setSelectedRegion] = useState("us-east");
  const [editingLicense, setEditingLicense] = useState(false);
  const [licenseKey, setLicenseKey] = useState("");
  const [controlPlaneURL, setControlPlaneURL] = useState("");
  const [editingDeploymentType, setEditingDeploymentType] = useState(false);
  const [newDeploymentType, setNewDeploymentType] = useState("cloud");

  const isEnterprise = cloudBilling?.plan_id === "enterprise";

  const api = useCallback(async () => {
    const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
    const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}`, "Content-Type": "application/json" } : { "Content-Type": "application/json" };
    return { baseURL, token, headers };
  }, []);

  const getJSON = async (path: string) => {
    const { baseURL, headers } = await api();
    const r = await fetch(baseURL + path, { headers });
    return r.ok ? r.json() : null;
  };

  const doPost = async (path: string, body: any) => {
    const { baseURL, headers } = await api();
    return fetch(baseURL + path, { method: "POST", headers, body: JSON.stringify(body) });
  };

  const doPut = async (path: string, body: any) => {
    const { baseURL, headers } = await api();
    return fetch(baseURL + path, { method: "PUT", headers, body: JSON.stringify(body) });
  };

  const doDelete = async (path: string) => {
    const { baseURL, headers } = await api();
    return fetch(baseURL + path, { method: "DELETE", headers });
  };

  const loadAll = useCallback(async () => {
    if (!auth.is_logged_in || !isEnterprise) { setLoading(false); return; }
    try {
      const [ssoData, scimData, rolesData, slaSt, slaEv, rulesData, evtsData, auditData, modelData, ipData, reportsData, schedData, retentionData] = await Promise.all([
        getJSON("/enterprise/sso/config"),
        getJSON("/enterprise/scim/config"),
        getJSON("/enterprise/roles"),
        getJSON("/enterprise/sla/trend?days=1").catch(() => getJSON("/usage/sla/status")),
        getJSON("/usage/sla/events"),
        getJSON("/enterprise/alert-rules"),
        getJSON("/enterprise/alert-events"),
        getJSON("/enterprise/audit/logs"),
        getJSON("/enterprise/policies/models"),
        getJSON("/enterprise/policies/ip-whitelist"),
        getJSON("/enterprise/compliance/reports"),
        getJSON("/enterprise/compliance/schedule"),
        getJSON("/enterprise/audit/retention"),
      ]);
      if (ssoData?.config_id) setSSOConfig(ssoData);
      if (scimData?.config_id) setSCIMConfig(scimData);
      if (rolesData?.data) setRoles(rolesData.data);
      if (slaSt?.uptime_pct !== undefined) setSLAStatus(slaSt); else if (slaSt?.data?.length) setSLAStatus(slaSt.data[0]);
      if (slaEv?.data) setSLAEvents(slaEv.data);
      if (rulesData?.data) setAlertRules(rulesData.data);
      if (evtsData?.data) setAlertEvents(evtsData.data);
      if (auditData?.data) setAuditLogs(auditData.data);
      if (modelData?.allowed_models !== undefined) setModelPolicy(modelData);
      if (ipData?.data) setIpWhitelist(ipData.data);
      if (reportsData?.data) setComplianceReports(reportsData.data);
      if (schedData?.frequency) setComplianceSchedule(schedData);
      if (retentionData?.retention_days) setAuditRetention(retentionData);
      const contract = await getJSON("/enterprise/contract");
      if (contract?.contract_id) setContractInfo(contract);
      const trial = await getJSON("/enterprise/trial/status");
      if (trial) setTrialStatus(trial);

      const regions = await getJSON("/enterprise/data-regions");
      if (regions?.data) setDataRegions(regions.data);
      const active = await getJSON("/enterprise/data-regions/active");
      if (active?.region) { setDeploymentType(active.deployment_type); }
      const lic = await getJSON("/enterprise/license");
      if (lic) setLicenseInfo(lic);
      const depStatus = await getJSON("/enterprise/deployment/status");
      if (depStatus) setDeploymentStatus(depStatus);
    } catch {} finally { setLoading(false); }
  }, [auth.is_logged_in, isEnterprise]);

  useEffect(() => { loadAll(); }, [loadAll]);

  const loadAuditLogs = async (actionFilter?: string, searchQuery?: string) => {
    const af = actionFilter !== undefined ? actionFilter : auditActionFilter;
    const sq = searchQuery !== undefined ? searchQuery : auditSearchQuery;
    let path = "/enterprise/audit/logs";
    const params: string[] = [];
    if (af) params.push("action=" + encodeURIComponent(af));
    if (sq) params.push("q=" + encodeURIComponent(sq));
    if (params.length) path += "?" + params.join("&");
    const d = await getJSON(path);
    if (d?.data) setAuditLogs(d.data);
  };

  const loadSLATrend = async (days: number) => {
    const d = await getJSON("/enterprise/sla/trend?days=" + days);
    if (d?.data) setSLATrend(d.data);
  };

  if (!auth.is_logged_in) {
    return (
      <div className="page enterprise-page">
        <div className="empty-state">
          <h2>{t("enterprise.pleaseLogin")}</h2>
          <p>{t("enterprise.loginToViewPortal")}</p>
          <button className="btn-primary" onClick={() => navigate("/auth")}>{t("enterprise.goLogin")}</button>
        </div>
      </div>
    );
  }

  if (!isEnterprise) {
    return (
      <div className="page enterprise-page">
        <div className="page-header">
          <h1>{t("enterprise.portal")}</h1>
          <p className="page-subtitle">{t("enterprise.subtitle")}</p>
        </div>
        <div className="empty-state">
          <div className="empty-icon">🏢</div>
          <h2>{t("enterprise.notInPlan")}</h2>
          <p>{t("enterprise.enterpriseDesc")}</p>
          <button className="btn-primary" onClick={() => navigate("/subscription")}>{t("enterprise.contactSales")}</button>
        </div>
      </div>
    );
  }

  if (loading) return <div className="page"><div className="loading">{t("enterprise.loading")}</div></div>;

  const tabList: { id: TabId; label: string; icon: string }[] = [
    { id: "overview", label: t("enterprise.overview"), icon: "📊" },
    { id: "audit", label: t("enterprise.audit"), icon: "🔍" },
    { id: "identity", label: t("enterprise.identity"), icon: "🔐" },
    { id: "policies", label: t("enterprise.policies"), icon: "🛡️" },
    { id: "compliance", label: t("enterprise.compliance"), icon: "📋" },
    { id: "sla", label: t("enterprise.sla"), icon: "📈" },
    { id: "alerts", label: t("enterprise.alerts"), icon: "🔔" },
    { id: "support", label: t("enterprise.support"), icon: "💬" },
    { id: "deployment", label: t("enterprise.deployment"), icon: "☁️" },
  ];

  const renderOverview = () => (
    <>
      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.governanceOverview")}</h2></div>
        <div className="ent-governance-grid">
          <div className="governance-card">
            <div className="gov-icon">🔍</div>
            <div className="gov-title">{t("enterprise.auditLogs")}</div>
            <div className="gov-desc">{t("enterprise.auditLogsDesc", { count: auditLogs.length, days: auditRetention.retention_days })}</div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("audit")}>{t("enterprise.view")}</button>
          </div>
          <div className="governance-card">
            <div className="gov-icon">🔐</div>
            <div className="gov-title">{t("enterprise.ssoStatus", { status: ssoConfig?.enabled ? t("enterprise.enabled") : t("enterprise.notConfigured") })}</div>
            <div className="gov-desc">{ssoConfig?.protocol?.toUpperCase() || t("enterprise.notConfigured")} · {ssoConfig?.force_sso ? t("enterprise.forceSSO") : t("enterprise.optional")}</div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("identity")}>{t("enterprise.configure")}</button>
          </div>
          <div className="governance-card">
            <div className="gov-icon">👤</div>
            <div className="gov-title">{t("enterprise.scimStatus", { status: scimConfig?.enabled ? t("enterprise.enabled") : t("enterprise.notConfigured") })}</div>
            <div className="gov-desc">{scimConfig?.sync_status === "idle" ? t("enterprise.ready") : scimConfig?.sync_status}</div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("identity")}>{t("enterprise.configure")}</button>
          </div>
          <div className="governance-card">
            <div className="gov-icon">🛡️</div>
            <div className="gov-title">{t("enterprise.policyControl")}</div>
            <div className="gov-desc">{t("enterprise.policyControlDesc", { count: ipWhitelist.length })}</div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("policies")}>{t("enterprise.manage")}</button>
          </div>
          <div className="governance-card">
            <div className="gov-icon">📋</div>
            <div className="gov-title">{t("enterprise.complianceReports")}</div>
            <div className="gov-desc">{t("enterprise.complianceReportsDesc", { count: complianceReports.length, enabled: complianceSchedule.enabled ? t("enterprise.scheduledPush") : t("enterprise.noPush") })}</div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("compliance")}>{t("enterprise.view")}</button>
          </div>
          <div className="governance-card">
            <div className="gov-icon">📈</div>
            <div className="gov-title">{t("enterprise.slaTitle", { status: slaStatus?.status === "healthy" ? t("enterprise.normal") : t("enterprise.abnormal") })}</div>
            <div className="gov-desc">{t("enterprise.slaDesc", { uptime: slaStatus?.uptime_pct || 99.9, latency: slaStatus?.p95_latency_ms || 0 })}</div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("sla")}>{t("enterprise.view")}</button>
          </div>
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.slaRealTimeMetrics")}</h2></div>
        <div className="sla-grid">
          <div className="sla-item">
            <div className="sla-metric" style={{ color: slaStatus?.uptime_pct !== undefined && slaStatus.uptime_pct < 99.0 ? "var(--red)" : "var(--accent)" }}>
              {(slaStatus?.uptime_pct || 99.9).toFixed(1)}%
            </div>
            <div className="sla-label">{t("enterprise.slaUptime")}</div>
          </div>
          <div className="sla-item">
            <div className="sla-metric" style={{ color: slaStatus?.p95_latency_ms !== undefined && slaStatus.p95_latency_ms > 300 ? "var(--red)" : "var(--accent)" }}>
              {(slaStatus?.p95_latency_ms || 0)}ms
            </div>
            <div className="sla-label">{t("enterprise.p95Latency")}</div>
          </div>
          <div className="sla-item">
            <div className="sla-metric">{(slaStatus?.status || "healthy") === "healthy" ? t("enterprise.normal") : t("enterprise.abnormal")}</div>
            <div className="sla-label">{t("enterprise.status")}</div>
          </div>
          <div className="sla-item">
            <div className="sla-metric">{slaEvents.length}</div>
            <div className="sla-label">{t("enterprise.slaEvents")}</div>
          </div>
        </div>
      </div>

      {contractInfo && (
        <div className="section section-card">
          <div className="section-heading"><h2>{t("enterprise.contractInfo")}</h2></div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, fontSize: 13 }}>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.contractNumber")}:</span> {contractInfo.contract_number || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.poNumber")}:</span> {contractInfo.po_number || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.totalAmount")}:</span> ${(contractInfo.total_amount || 0).toFixed(2)} {contractInfo.currency || "USD"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.billingCycle")}:</span> {contractInfo.billing_cycle || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.seats")}:</span> {contractInfo.seats || 0}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.seatPrice")}:</span> ${(contractInfo.seat_price || 0).toFixed(2)}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.startDate")}:</span> {contractInfo.start_date?.substring(0, 10) || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.endDate")}:</span> {contractInfo.end_date?.substring(0, 10) || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.status")}:</span> {contractInfo.status || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.salesContact")}:</span> {contractInfo.sales_contact || "-"}</div>
          </div>
        </div>
      )}

      {trialStatus && (
        <div className="section section-card">
          <div className="section-heading"><h2>{t("enterprise.trialStatus")}</h2></div>
          <div style={{ display: "flex", gap: 16, alignItems: "center", flexWrap: "wrap" }}>
            <div style={{ flex: 1, minWidth: 120 }}>
              <div style={{ fontSize: 24, fontWeight: 700, color: trialStatus.is_expired ? "var(--red)" : "var(--green)" }}>
                {trialStatus.remaining_days > 0 ? t("enterprise.trialRemaining", { days: trialStatus.remaining_days }) : t("enterprise.trialExpired")}
              </div>
              <div style={{ fontSize: 13, color: "var(--text-muted)" }}>{t("enterprise.trialInfo", { total: trialStatus.trial_days, used: trialStatus.used_days })}</div>
            </div>
            {trialStatus.is_expired && (
              <button className="btn-primary" onClick={async () => {
                await doPost("/enterprise/trial/convert", { plan_id: "teams" });
                loadAll();
              }}>{t("enterprise.convertToPaid")}</button>
            )}
          </div>
        </div>
      )}

      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.quickActions")}</h2></div>
        <div className="shared-section" style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <div className="shared-item">
            <div className="shared-icon">🔐</div>
            <div className="shared-content">
              <div className="shared-title">{t("enterprise.configureSSO")}</div>
              <div className="shared-desc">{t("enterprise.ssoDesc")}</div>
            </div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("identity")}>{t("enterprise.configure")}</button>
          </div>
          <div className="shared-item">
            <div className="shared-icon">🔍</div>
            <div className="shared-content">
              <div className="shared-title">{t("enterprise.viewAuditLogs")}</div>
              <div className="shared-desc">{t("enterprise.auditLogsShortDesc")}</div>
            </div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("audit")}>{t("enterprise.view")}</button>
          </div>
          <div className="shared-item">
            <div className="shared-icon">📋</div>
            <div className="shared-content">
              <div className="shared-title">{t("enterprise.exportComplianceReports")}</div>
              <div className="shared-desc">{t("enterprise.complianceReportsShortDesc")}</div>
            </div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("compliance")}>{t("enterprise.manage")}</button>
          </div>
          <div className="shared-item">
            <div className="shared-icon">💬</div>
            <div className="shared-content">
              <div className="shared-title">{t("enterprise.contactSupport")}</div>
              <div className="shared-desc">{t("enterprise.supportDesc")}</div>
            </div>
            <button className="btn-secondary btn-sm" onClick={() => setActiveTab("support")}>{t("enterprise.contact")}</button>
          </div>
        </div>
      </div>
    </>
  );

  const renderAudit = () => (
    <div className="section section-card">
      <div className="section-heading">
        <h2>{t("enterprise.auditLogs")}</h2>
        <button className="btn-secondary btn-sm" onClick={async () => {
          const {baseURL} = await api();
          openInBrowser(baseURL + "/enterprise/audit/export?format=csv");
        }}>{t("enterprise.exportCSV")}</button>
      </div>
      <div className="enterprise-toolbar">
        <input type="text" className="form-input" style={{ width: 200 }} placeholder={t("enterprise.filterByAction")} value={auditActionFilter} onChange={(e) => setAuditActionFilter(e.target.value)} />
        <input type="text" className="form-input" style={{ width: 240 }} placeholder={t("enterprise.searchPlaceholder")} value={auditSearchQuery} onChange={(e) => setAuditSearchQuery(e.target.value)} />
        <button className="btn-primary btn-sm" onClick={() => loadAuditLogs()}>{t("enterprise.search")}</button>
        <button className="btn-secondary btn-sm" onClick={() => { setAuditActionFilter(""); setAuditSearchQuery(""); loadAuditLogs("", ""); }}>{t("enterprise.reset")}</button>
      </div>
      <div className="section-meta-text" style={{ marginBottom: 12 }}>
        {t("enterprise.retentionPolicy", { days: auditRetention.retention_days, autoArchive: auditRetention.auto_archive ? t("enterprise.on") : t("enterprise.off") })}
      </div>
      {auditLogs.length === 0 ? (
        <div className="empty-hint">{t("enterprise.noAuditLogs")}</div>
      ) : (
        <div className="audit-list" style={{ maxHeight: 500 }}>
          {auditLogs.map((log) => (
            <div key={log.id} className="audit-item">
              <div className="audit-head">
                <span className="audit-action">{log.action}</span>
                <span className={"audit-result " + (log.result === "200 OK" || log.result === "201 Created" || log.result === "OK" ? "result-ok" : "result-fail")}>
                  {log.result}
                </span>
              </div>
              <div className="audit-meta">
                <span>{log.actor_type}:{log.actor_id}</span>
                <span>{log.target_type}:{log.target_id}</span>
                <span>{log.ip}</span>
                <span>{log.created_at}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );

  const renderIdentity = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.ssoConfig")}</h2>
          <button className="btn-primary btn-sm" onClick={() => {
            setSSOProtocol(ssoConfig?.protocol || "saml");
            setSSOIdpMetaUrl(ssoConfig?.idp_metadata_url || "");
            setSSOIdpEntityId(ssoConfig?.idp_entity_id || "");
            setSSOIdpCert("");
            setSSOForceSSO(ssoConfig?.force_sso || false);
            setSSOEnabled(ssoConfig?.enabled || false);
            setEditingSSO(true);
          }}>{t("enterprise.edit")}</button>
        </div>
        <div className="enterprise-info-grid">
          <div>
            <strong>{t("enterprise.protocol")}:</strong> {ssoConfig?.protocol?.toUpperCase() || t("enterprise.notConfigured")}
            <br /><strong>IdP Entity ID:</strong> {ssoConfig?.idp_entity_id || "-"}
            <br /><strong>SP Entity ID:</strong> {ssoConfig?.sp_entity_id || "-"}
          </div>
          <div>
            <strong>ACS URL:</strong> {ssoConfig?.acs_url || "-"}
            <br /><strong>{t("enterprise.forceSSO")}:</strong> {ssoConfig?.force_sso ? t("enterprise.yes") : t("enterprise.no")}
            <br /><strong>{t("enterprise.status")}:</strong> {ssoConfig?.enabled ? t("enterprise.enabled") : t("enterprise.disabled")}
          </div>
        </div>
        <div className="enterprise-mini-toolbar">
          <button className="btn-secondary btn-sm" onClick={async () => {
          const { baseURL } = await api();
          openInBrowser(baseURL + "/enterprise/sso/metadata");
          }}>{t("enterprise.viewSPMetadata")}</button>
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.scimConfig")}</h2>
          <button className="btn-primary btn-sm" onClick={() => {
            setSCIMEndpoint(scimConfig?.scim_endpoint || "");
            setSCIMEnabled(scimConfig?.enabled || false);
            setEditingSCIM(true);
          }}>{t("enterprise.edit")}</button>
        </div>
        {scimConfig ? (
          <div className="enterprise-info-grid">
            <div><strong>{t("enterprise.endpoint")}:</strong> <code>{scimConfig.scim_endpoint}</code></div>
            <div><strong>Token:</strong> <code>{scimConfig.scim_token?.substring(0, 20)}...</code></div>
            <div><strong>{t("enterprise.syncStatus")}:</strong> {scimConfig.sync_status}</div>
            <div><strong>{t("enterprise.lastSync")}:</strong> {scimConfig.last_sync_at || t("enterprise.never")} · <strong>{t("enterprise.enabled")}:</strong> {scimConfig.enabled ? t("enterprise.yes") : t("enterprise.no")}</div>
            <div className="enterprise-mini-toolbar">
              <button className="btn-secondary btn-sm" onClick={async () => {
                const r = await doPost("/enterprise/scim/token", {});
                if (r.ok) loadAll();
              }}>{t("enterprise.refreshToken")}</button>
            </div>
          </div>
        ) : (
          <div className="empty-hint">{t("enterprise.scimNotConfigured")}</div>
        )}
      </div>

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.customRoles")}</h2>
          <button className="btn-primary btn-sm" onClick={() => {
            setEditingRole(null);
            setRoleName("");
            setRoleDesc("");
            setRolePerms("{}");
          }}>{t("enterprise.addRole")}</button>
        </div>
        {roles.length === 0 ? (
          <div className="empty-hint">{t("enterprise.noCustomRoles")}</div>
        ) : (
          <div className="member-list">
            {roles.map((r) => (
              <div key={r.role_id} className="member-card" style={{ justifyContent: "space-between" }}>
                <div>
                  <div className="member-email">{r.name} {r.is_builtin && <span className="member-role role-member">{t("enterprise.preset")}</span>}</div>
                  <div className="member-meta">{r.description}</div>
                </div>
                <div style={{ display: "flex", gap: 4 }}>
                  <button className="btn-secondary btn-sm" onClick={() => {
                    setEditingRole(r);
                    setRoleName(r.name);
                    setRoleDesc(r.description);
                    setRolePerms(r.permissions);
                  }}>{t("enterprise.view")}</button>
                  {!r.is_builtin && <button className="btn-text btn-sm" onClick={async () => { await doDelete("/enterprise/roles/" + r.role_id); loadAll(); }}>🗑️</button>}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );

  const renderPolicies = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.modelPolicy")}</h2>
          <button className="btn-primary btn-sm" onClick={() => {
            setEditingModelAllowed(modelPolicy?.allowed_models || "[]");
            setEditingModelDenied(modelPolicy?.denied_models || "[]");
            setEditingModelDefault(modelPolicy?.default_action || "allow");
            setEditingModelPolicy(true);
          }}>{t("enterprise.edit")}</button>
        </div>
        <div className="enterprise-info-grid">
          <div><strong>{t("enterprise.allowedModels")}:</strong> <code>{modelPolicy?.allowed_models || "[]"}</code></div>
          <div><strong>{t("enterprise.deniedModels")}:</strong> <code>{modelPolicy?.denied_models || "[]"}</code></div>
          <div><strong>{t("enterprise.defaultAction")}:</strong> {modelPolicy?.default_action || "allow"}</div>
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.ipWhitelist")}</h2>
        </div>
        {ipWhitelist.length === 0 ? (
          <div className="empty-hint">{t("enterprise.noIpWhitelist")}</div>
        ) : (
          <div className="member-list">
            {ipWhitelist.map((e) => (
              <div key={e.id} className="member-card" style={{ justifyContent: "space-between" }}>
                <div>
                  <div className="member-email"><code>{e.cidr}</code></div>
                  <div className="member-meta">{e.description || "-"}</div>
                </div>
                <button className="btn-text btn-sm" onClick={async () => { await doDelete("/enterprise/policies/ip-whitelist/" + e.id); loadAll(); }}>🗑️</button>
              </div>
            ))}
          </div>
        )}
        <div className="enterprise-mini-toolbar">
          <input type="text" className="form-input" style={{ width: 180 }} placeholder={t("enterprise.cidrPlaceholder")} value={newIP.cidr} onChange={(e) => setNewIP({ ...newIP, cidr: e.target.value })} />
          <input type="text" className="form-input" style={{ width: 200 }} placeholder={t("enterprise.description")} value={newIP.description} onChange={(e) => setNewIP({ ...newIP, description: e.target.value })} />
          <button className="btn-primary btn-sm" onClick={async () => {
            if (!newIP.cidr) return;
            await doPost("/enterprise/policies/ip-whitelist", newIP);
            setNewIP({ cidr: "", description: "" });
            loadAll();
          }}>{t("enterprise.add")}</button>
        </div>
      </div>
    </>
  );

  const renderCompliance = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.complianceReports")}</h2>
          <button className="btn-primary btn-sm" onClick={async () => {
            await doPost("/enterprise/compliance/reports/generate", { report_type: "monthly", format: "pdf" });
            loadAll();
          }}>{t("enterprise.generateReport")}</button>
        </div>
        {complianceReports.length === 0 ? (
          <div className="empty-hint">{t("enterprise.noComplianceReports")}</div>
        ) : (
          <div className="member-list">
            {complianceReports.map((r) => (
              <div key={r.report_id} className="member-card" style={{ justifyContent: "space-between" }}>
                <div>
                  <div className="member-email">{r.report_type} · {r.period_start?.substring(0, 10)} ~ {r.period_end?.substring(0, 10)}</div>
                  <div className="member-meta">
                    <span className={"member-status " + (r.status === "completed" ? "status-active" : "status-inactive")}>{r.status}</span>
                    <span>{r.file_format}</span>
                  </div>
                </div>
                <div style={{ display: "flex", gap: 4 }}>
                  {r.status === "completed" && (
                    <button className="btn-secondary btn-sm" onClick={async () => {
                    const { baseURL } = await api();
                    openInBrowser(baseURL + "/enterprise/compliance/reports/" + r.report_id + "/export");
                    }}>{t("enterprise.download")}</button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.scheduleConfig")}</h2></div>
        <div className="enterprise-info-grid">
          <div><strong>{t("enterprise.frequency")}:</strong> {complianceSchedule.frequency} · <strong>{t("enterprise.format")}:</strong> {complianceSchedule.format}</div>
          <div><strong>{t("enterprise.targetEmail")}:</strong> {complianceSchedule.target_email || t("enterprise.notSet")} · <strong>{t("enterprise.enabled")}:</strong> {complianceSchedule.enabled ? t("enterprise.yes") : t("enterprise.no")}</div>
        </div>
      </div>
    </>
  );

  const renderSLA = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.slaRealTimeStatus")}</h2>
          <button className="btn-secondary btn-sm" onClick={() => setSLAEventDialog(true)}>{t("enterprise.reportEvent")}</button>
        </div>
        <div className="sla-grid">
          <div className="sla-item">
            <div className="sla-metric" style={{ color: slaStatus?.uptime_pct !== undefined && slaStatus.uptime_pct < 99.0 ? "var(--red)" : "var(--accent)" }}>
              {(slaStatus?.uptime_pct || 99.9).toFixed(2)}%
            </div>
            <div className="sla-label">{t("enterprise.slaUptime")}</div>
          </div>
          <div className="sla-item">
            <div className="sla-metric" style={{ color: slaStatus?.p95_latency_ms !== undefined && slaStatus.p95_latency_ms > 250 ? (slaStatus.p95_latency_ms > 300 ? "var(--red)" : "var(--orange)") : "var(--accent)" }}>
              {slaStatus?.p95_latency_ms || 0}ms
            </div>
            <div className="sla-label">{t("enterprise.p95Latency")}</div>
          </div>
          <div className="sla-item">
            <div className="sla-metric" style={{ color: slaStatus?.error_rate_pct !== undefined && slaStatus.error_rate_pct > 1 ? "var(--red)" : "var(--accent)" }}>
              {(slaStatus?.error_rate_pct || 0).toFixed(2)}%
            </div>
            <div className="sla-label">{t("enterprise.errorRate")}</div>
          </div>
          <div className="sla-item">
            <div className="sla-metric" style={{ color: (slaStatus?.status || "healthy") === "healthy" ? "var(--green)" : "var(--red)" }}>
              {(slaStatus?.status || "healthy") === "healthy" ? t("enterprise.normal") : t("enterprise.abnormal")}
            </div>
            <div className="sla-label">{t("enterprise.overallStatus")}</div>
          </div>
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.slaHistoryTrend")}</h2>
          <div style={{ display: "flex", gap: 4 }}>
            {[7, 30, 90].map((d) => (
              <button key={d} className={"btn-sm " + (slaTrendDays === d ? "btn-primary" : "btn-secondary")} onClick={() => { setSLATrendDays(d); loadSLATrend(d); }}>{t("enterprise.days", { count: d })}</button>
            ))}
          </div>
        </div>
        {slaTrend.length === 0 && <div className="empty-hint">{t("enterprise.noTrendData")}</div>}
        {slaTrend.length > 0 && (
          <div style={{ fontSize: 13 }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 4, fontWeight: 600, borderBottom: "1px solid var(--border)", paddingBottom: 8, marginBottom: 8 }}>
              <span>{t("enterprise.date")}</span>
              <span>{t("enterprise.uptimeLatencyErrorRate")}</span>
            </div>
            {slaTrend.map((p) => (
              <div key={p.date} style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 4, padding: "4px 0" }}>
                <span>{p.date}</span>
                <span>{p.uptime_pct.toFixed(2)}% · {p.p95_latency_ms}ms · {(p.error_rate_pct * 100).toFixed(1)}‰</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.slaEvents")}</h2></div>
        {slaEvents.length === 0 ? (
          <div className="empty-hint">{t("enterprise.noSlaEvents")}</div>
        ) : (
          <div className="audit-list">
            {slaEvents.map((e) => (
              <div key={e.id} className="audit-item">
                <div className="audit-head">
                  <span className="audit-action">{e.event_type}</span>
                  <span className={"audit-result " + (e.severity === "critical" || e.severity === "error" ? "result-fail" : "result-ok")}>{e.severity}</span>
                </div>
                <div className="audit-meta">{e.description} · {e.created_at}</div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );

  const renderAlerts = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("enterprise.alertRules")}</h2>
          <button className="btn-primary btn-sm" onClick={() => {
            setEditingAlertRule(null);
            setAlertRuleName("");
            setAlertRuleCondition("rate_limit_exceeded");
            setAlertRuleThreshold(0);
            setAlertRuleSeverity("warning");
          }}>{t("enterprise.addRule")}</button>
        </div>
        {alertRules.length === 0 ? (
          <div className="empty-hint">{t("enterprise.noAlertRules")}</div>
        ) : (
          <div className="member-list">
            {alertRules.map((r) => (
              <div key={r.rule_id} className="member-card" style={{ justifyContent: "space-between" }}>
                <div>
                  <div className="member-email">{r.name}</div>
                  <div className="member-meta">
                    <span className="member-role role-member">{r.condition_expr}</span>
                    {r.threshold > 0 && <span className="member-status status-active">{t("enterprise.threshold")}: {r.threshold}</span>}
                    <span className={"member-status " + (r.enabled ? "status-active" : "status-inactive")}>{r.enabled ? t("enterprise.enabled") : t("enterprise.disabled")}</span>
                    <span className={"member-status status-" + (r.severity === "critical" ? "inactive" : "active")}>{r.severity}</span>
                  </div>
                </div>
                <div style={{ display: "flex", gap: 4 }}>
                  <button className="btn-secondary btn-sm" onClick={() => {
                    setEditingAlertRule(r);
                    setAlertRuleName(r.name);
                    setAlertRuleCondition(r.condition_expr);
                    setAlertRuleThreshold(r.threshold);
                    setAlertRuleSeverity(r.severity);
                  }}>{t("enterprise.edit")}</button>
                  <button className="btn-text btn-sm" onClick={async () => { await doDelete("/enterprise/alert-rules/" + r.rule_id); loadAll(); }}>🗑️</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.alertEvents")}</h2></div>
        {alertEvents.length === 0 ? (
          <div className="empty-hint">{t("enterprise.noAlertEvents")}</div>
        ) : (
          <div className="member-list">
            {alertEvents.map((e) => (
              <div key={e.event_id} className="member-card" style={{ justifyContent: "space-between" }}>
                <div>
                  <div className="member-email">{e.message}</div>
                  <div className="member-meta">
                    <span className={"member-role role-" + (e.severity === "critical" ? "admin" : "member")}>{e.severity}</span>
                    <span className={"member-status " + (e.resolved ? "status-active" : "status-inactive")}>{e.resolved ? t("enterprise.resolved") : t("enterprise.unresolved")}</span>
                  </div>
                </div>
                {!e.resolved && (
                  <button className="btn-secondary btn-sm" onClick={async () => { await doPost("/enterprise/alert-events/" + e.event_id + "/resolve", {}); loadAll(); }}>{t("enterprise.resolve")}</button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );

  const renderSupport = () => (
    <div className="section section-card">
      <div className="section-heading"><h2>{t("enterprise.supportAndTickets")}</h2></div>
      <div className="support-items">
        <div className="support-item">
          <div className="support-icon">📧</div>
          <div className="support-content">
            <div className="support-title">{t("enterprise.dedicatedManager")}</div>
            <div className="support-desc">{t("enterprise.dedicatedManagerDesc")}</div>
          </div>
          <a href="mailto:enterprise@seasagi.com" className="btn-primary btn-sm">{t("enterprise.sendEmail")}</a>
        </div>
        <div className="support-item">
          <div className="support-icon">🐛</div>
          <div className="support-content">
            <div className="support-title">{t("enterprise.submitTicket")}</div>
            <div className="support-desc">{t("enterprise.submitTicketDesc")}</div>
          </div>
          <a href="https://github.com/neeke/seasagi/issues" target="_blank" rel="noopener noreferrer" className="btn-secondary btn-sm">{t("enterprise.submit")}</a>
        </div>
        <div className="support-item">
          <div className="support-icon">📖</div>
          <div className="support-content">
            <div className="support-title">{t("enterprise.enterpriseDocs")}</div>
            <div className="support-desc">{t("enterprise.enterpriseDocsDesc")}</div>
          </div>
          <button className="btn-secondary btn-sm" onClick={() => openInBrowser("https://github.com/neeke/seasagi")}>{t("enterprise.view")}</button>
        </div>
        <div className="support-item">
          <div className="support-icon">💬</div>
          <div className="support-content">
            <div className="support-title">{t("enterprise.slaEventReport")}</div>
            <div className="support-desc">{t("enterprise.slaEventReportDesc")}</div>
          </div>
          <button className="btn-secondary btn-sm" onClick={() => setSLAEventDialog(true)}>{t("enterprise.report")}</button>
        </div>
      </div>
    </div>
  );

  const renderDeployment = () => (
    <>
      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.dataResidencyRegions")}</h2></div>
        <div className="enterprise-region-grid">
          {dataRegions.map((r: any) => (
            <div
              key={r.region}
              className={`enterprise-region-card ${r.is_active ? "is-active" : "is-inactive"}`}
              onClick={() => {
                if (!r.is_active) { setSelectedRegion(r.region); setEditingRegion(true); }
              }}
            >
              <div className="enterprise-region-flag">{r.region === "us-east" ? "🇺🇸" : r.region === "us-west" ? "🇺🇸" : r.region === "eu" ? "🇪🇺" : "🇯🇵"}</div>
              <div className="enterprise-region-name">{r.region_name}</div>
              <div className="enterprise-region-code">{r.region}</div>
              {r.is_active && <div className="enterprise-region-active">{t("enterprise.currentRegion")}</div>}
            </div>
          ))}
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.deploymentMode")}</h2></div>
        <div className="ent-governance-grid">
          <div className="governance-card">
            <div className="gov-icon">☁️</div>
            <div className="gov-title">{t("enterprise.cloudDeployment")}</div>
            <div className="gov-desc">{t("enterprise.cloudDeploymentDesc")}</div>
            <div style={{ marginTop: 8, display: "flex", gap: 4 }}>
              <button className={deploymentType === "cloud" ? "btn-primary btn-sm" : "btn-secondary btn-sm"}
                onClick={() => { setNewDeploymentType("cloud"); setEditingDeploymentType(true); }}>{t("enterprise.select")}</button>
            </div>
            {deploymentType === "cloud" && <div style={{ fontSize: 11, color: "var(--accent)", marginTop: 4 }}>{t("enterprise.current")}</div>}
          </div>
          <div className="governance-card">
            <div className="gov-icon">🏠</div>
            <div className="gov-title">{t("enterprise.privateDeployment")}</div>
            <div className="gov-desc">{t("enterprise.privateDeploymentDesc")}</div>
            <div style={{ marginTop: 8, display: "flex", gap: 4 }}>
              <button className={deploymentType === "private" ? "btn-primary btn-sm" : "btn-secondary btn-sm"}
                onClick={() => { setNewDeploymentType("private"); setEditingDeploymentType(true); }}>{t("enterprise.select")}</button>
            </div>
            {deploymentType === "private" && <div style={{ fontSize: 11, color: "var(--accent)", marginTop: 4 }}>{t("enterprise.current")}</div>}
          </div>
          <div className="governance-card">
            <div className="gov-icon">🔗</div>
            <div className="gov-title">{t("enterprise.hybridDeployment")}</div>
            <div className="gov-desc">{t("enterprise.hybridDeploymentDesc")}</div>
            <div style={{ marginTop: 8, display: "flex", gap: 4 }}>
              <button className={deploymentType === "hybrid" ? "btn-primary btn-sm" : "btn-secondary btn-sm"}
                onClick={() => { setNewDeploymentType("hybrid"); setEditingDeploymentType(true); }}>{t("enterprise.select")}</button>
            </div>
            {deploymentType === "hybrid" && <div style={{ fontSize: 11, color: "var(--accent)", marginTop: 4 }}>{t("enterprise.current")}</div>}
          </div>
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading"><h2>{t("enterprise.licenseManagement")}</h2></div>
        {licenseInfo?.registered ? (
          <div className="enterprise-info-grid">
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.licenseKey")}:</span> {licenseInfo.license_key || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.licenseType")}:</span> {licenseInfo.license_type || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.status")}:</span> <span style={{ color: licenseInfo.status === "active" ? "var(--green)" : "var(--red)" }}>{licenseInfo.status}</span></div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.maxSeats")}:</span> {licenseInfo.max_seats || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.expiresAt")}:</span> {licenseInfo.expires_at?.substring(0, 10) || "-"}</div>
            <div><span style={{ color: "var(--text-muted)" }}>{t("enterprise.features")}:</span> {licenseInfo.features || "[]"}</div>
          </div>
        ) : (
          <div className="enterprise-license-empty">
            <div className="empty-hint">{t("enterprise.noLicense")}</div>
            <button className="btn-primary btn-sm" onClick={() => setEditingLicense(true)}>{t("enterprise.registerLicense")}</button>
          </div>
        )}
      </div>

      {deploymentStatus && (
        <div className="section section-card">
          <div className="section-heading"><h2>{t("enterprise.hybridDeploymentStatus")}</h2></div>
          <div className="hero-metrics enterprise-status-metrics">
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("enterprise.deploymentMode")}</span>
              <strong className="hero-metric-value">{deploymentStatus.deployment_type}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("enterprise.dataRegion")}</span>
              <strong className="hero-metric-value">{deploymentStatus.data_region}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("enterprise.license")}</span>
              <strong className="hero-metric-value" style={{ color: deploymentStatus.license_status === "active" ? "var(--green)" : "var(--red)" }}>{deploymentStatus.license_status}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("enterprise.gatewayStatus")}</span>
              <strong className="hero-metric-value" style={{ color: deploymentStatus.gateway_status === "connected" ? "var(--green)" : "var(--red)" }}>{deploymentStatus.gateway_status}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("enterprise.lastHeartbeat")}</span>
              <strong className="hero-metric-value" style={{ fontSize: 12 }}>{deploymentStatus.last_heartbeat?.substring(0, 19) || "-"}</strong>
            </div>
          </div>
        </div>
      )}

      {editingRegion && (
        <div className="dialog-overlay" onClick={() => setEditingRegion(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header"><h3>{t("enterprise.switchRegion")}</h3><button className="btn-text" onClick={() => setEditingRegion(false)}>✕</button></div>
            <div className="dialog-body">
              <p style={{ fontSize: 13, color: "var(--text-secondary)", marginBottom: 12 }}>{t("enterprise.switchRegionConfirm", { name: dataRegions.find((r: any) => r.region === selectedRegion)?.region_name })}</p>
              <label className="form-label">{t("enterprise.targetRegion")}</label>
              <select value={selectedRegion} onChange={(e) => setSelectedRegion(e.target.value)} className="form-input">
                {dataRegions.map((r: any) => (<option key={r.region} value={r.region}>{r.region_name} ({r.region})</option>))}
              </select>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setEditingRegion(false)}>{t("enterprise.cancel")}</button>
              <button className="btn-primary" onClick={async () => {
                await doPut("/enterprise/data-regions", { region: selectedRegion });
                setEditingRegion(false);
                loadAll();
              }}>{t("enterprise.confirmSwitch")}</button>
            </div>
          </div>
        </div>
      )}

      {editingLicense && (
        <div className="dialog-overlay" onClick={() => setEditingLicense(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header"><h3>{t("enterprise.registerLicense")}</h3><button className="btn-text" onClick={() => setEditingLicense(false)}>✕</button></div>
            <div className="dialog-body">
              <label className="form-label">{t("enterprise.licenseKey")}</label>
              <input type="text" value={licenseKey} onChange={(e) => setLicenseKey(e.target.value)} className="form-input" placeholder={t("enterprise.licenseKeyPlaceholder")} />
              <label className="form-label" style={{ marginTop: 12 }}>{t("enterprise.controlPlaneUrl")}</label>
              <input type="url" value={controlPlaneURL} onChange={(e) => setControlPlaneURL(e.target.value)} className="form-input" placeholder="https://control.yourcompany.com" />
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setEditingLicense(false)}>{t("enterprise.cancel")}</button>
              <button className="btn-primary" onClick={async () => {
                await doPost("/enterprise/license/register", { license_key: licenseKey, control_plane_url: controlPlaneURL });
                setEditingLicense(false);
                setLicenseKey("");
                setControlPlaneURL("");
                loadAll();
              }}>{t("enterprise.register")}</button>
            </div>
          </div>
        </div>
      )}

      {editingDeploymentType && (
        <div className="dialog-overlay" onClick={() => setEditingDeploymentType(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header"><h3>{t("enterprise.switchDeploymentMode")}</h3><button className="btn-text" onClick={() => setEditingDeploymentType(false)}>✕</button></div>
            <div className="dialog-body">
              <p style={{ fontSize: 13, color: "var(--text-secondary)", marginBottom: 12 }}>
                {newDeploymentType === "cloud" ? t("enterprise.switchToCloud") : newDeploymentType === "private" ? t("enterprise.switchToPrivate") : t("enterprise.switchToHybrid")}
              </p>
              <label className="form-label">{t("enterprise.deploymentMode")}</label>
              <select value={newDeploymentType} onChange={(e) => setNewDeploymentType(e.target.value)} className="form-input">
                <option value="cloud">{t("enterprise.cloudDeployment")}</option>
                <option value="private">{t("enterprise.privateDeployment")}</option>
                <option value="hybrid">{t("enterprise.hybridDeployment")}</option>
              </select>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setEditingDeploymentType(false)}>{t("enterprise.cancel")}</button>
              <button className="btn-primary" onClick={async () => {
                await doPut("/enterprise/deployment/type", { deployment_type: newDeploymentType });
                setEditingDeploymentType(false);
                loadAll();
              }}>{t("enterprise.confirm")}</button>
            </div>
          </div>
        </div>
      )}
    </>
  );

  const renderDialog = () => (
    <>
      {editingSSO && (
        <div className="dialog-overlay" onClick={() => setEditingSSO(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header"><h3>{t("enterprise.ssoConfig")}</h3><button className="btn-text" onClick={() => setEditingSSO(false)}>✕</button></div>
            <div className="dialog-body">
              <label className="form-label">{t("enterprise.protocol")}</label>
              <select value={ssoProtocol} onChange={(e) => setSSOProtocol(e.target.value)} className="form-input">
                <option value="saml">SAML 2.0</option>
                <option value="oidc">OIDC</option>
              </select>
              <label className="form-label" style={{ marginTop: 12 }}>{t("enterprise.idpMetadataUrl")}</label>
              <input type="url" value={ssoIdpMetaUrl} onChange={(e) => setSSOIdpMetaUrl(e.target.value)} className="form-input" placeholder="https://idp.example.com/metadata" />
              <label className="form-label" style={{ marginTop: 12 }}>IdP Entity ID</label>
              <input type="text" value={ssoIdpEntityId} onChange={(e) => setSSOIdpEntityId(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}>{t("enterprise.idpCert")}</label>
              <textarea value={ssoIdpCert} onChange={(e) => setSSOIdpCert(e.target.value)} className="form-input" rows={3} />
              <label className="form-label" style={{ marginTop: 12 }}><input type="checkbox" checked={ssoForceSSO} onChange={(e) => setSSOForceSSO(e.target.checked)} /> {t("enterprise.forceSSO")}</label>
              <label className="form-label"><input type="checkbox" checked={ssoEnabled} onChange={(e) => setSSOEnabled(e.target.checked)} /> {t("enterprise.enabled")}</label>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setEditingSSO(false)}>{t("enterprise.cancel")}</button>
              <button className="btn-primary" onClick={async () => {
                await doPut("/enterprise/sso/config", {
                  protocol: ssoProtocol,
                  idp_metadata_url: ssoIdpMetaUrl,
                  idp_entity_id: ssoIdpEntityId,
                  idp_certificate: ssoIdpCert,
                  force_sso: ssoForceSSO,
                  enabled: ssoEnabled,
                });
                setEditingSSO(false);
                loadAll();
              }}>{t("enterprise.save")}</button>
            </div>
          </div>
        </div>
      )}

      {editingSCIM && (
        <div className="dialog-overlay" onClick={() => setEditingSCIM(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header"><h3>{t("enterprise.scimConfig")}</h3><button className="btn-text" onClick={() => setEditingSCIM(false)}>✕</button></div>
            <div className="dialog-body">
              <label className="form-label">{t("enterprise.scimEndpointUrl")}</label>
              <input type="url" value={scimEndpoint} onChange={(e) => setSCIMEndpoint(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}><input type="checkbox" checked={scimEnabled} onChange={(e) => setSCIMEnabled(e.target.checked)} /> {t("enterprise.enableSCIM")}</label>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setEditingSCIM(false)}>{t("enterprise.cancel")}</button>
              <button className="btn-primary" onClick={async () => {
                await doPut("/enterprise/scim/config", { scim_endpoint: scimEndpoint, enabled: scimEnabled });
                setEditingSCIM(false);
                loadAll();
              }}>{t("enterprise.save")}</button>
            </div>
          </div>
        </div>
      )}
    </>
  );

  return (
    <div className="page enterprise-page">
      <div className="page-header">
        <h1>{t("enterprise.portal")}</h1>
        <p className="page-subtitle">{t("enterprise.subtitle")}</p>
      </div>

      <div className="tab-bar">
        {tabList.map((tab) => (
          <button key={tab.id} className={activeTab === tab.id ? "active" : ""} onClick={() => setActiveTab(tab.id)}>
            {tab.icon} {tab.label}
          </button>
        ))}
      </div>

      {activeTab === "overview" && renderOverview()}
      {activeTab === "audit" && renderAudit()}
      {activeTab === "identity" && renderIdentity()}
      {activeTab === "policies" && renderPolicies()}
      {activeTab === "compliance" && renderCompliance()}
      {activeTab === "sla" && renderSLA()}
      {activeTab === "alerts" && renderAlerts()}
      {activeTab === "support" && renderSupport()}
      {activeTab === "deployment" && renderDeployment()}

      {renderDialog()}
    </div>
  );
}
