import { useEffect, useState, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import { useTeamStore, type Strategy, type ComboTemplate, type WebhookItem } from "../stores/teamStore";
import { getPlatformAPIBaseURL, getPlatformToken } from "../utils/commands";
import { useTranslation } from "../i18n";

type TabId = "overview" | "members" | "usage" | "strategy" | "billing" | "webhook";

const apiFetch = async (path: string, options?: RequestInit) => {
  const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(baseURL + path, { ...options, headers: { ...headers, ...((options?.headers as Record<string,string>) || {}) } });
  if (!res.ok) {
    const errBody = await res.text().catch(() => "");
    throw new Error(`API error ${res.status}: ${errBody || res.statusText}`);
  }
  return res.json();
};

export function TeamWorkspacePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const auth = useAppStore((s) => s.auth);
  const cloudBilling = useAppStore((s) => s.cloudBilling);

  const store = useTeamStore();
  const profile = store.profile;
  const memberUsage = store.memberUsage;
  const strategies = store.strategies;
  const templates = store.templates;
  const webhooks = store.webhooks;
  const invoices = store.invoices;
  const overages = store.overages;
  const seatInfo = store.seatInfo;
  const billingSummary = store.getBillingSummary();
  const costAttribution = store.getCostAttribution();

  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<TabId>("overview");
  const [editingProfile, setEditingProfile] = useState(false);
  const [profileName, setProfileName] = useState("");
  const [profileBillingEmail, setProfileBillingEmail] = useState("");

  const [inviteDialog, setInviteDialog] = useState(false);
  const [inviteEmails, setInviteEmails] = useState("");
  const [inviteRole, setInviteRole] = useState("member");

  const [inviteLinkDialog, setInviteLinkDialog] = useState(false);
  const [inviteLinkRole, setInviteLinkRole] = useState("member");
  const [inviteLinkExpiresIn, setInviteLinkExpiresIn] = useState(604800);
  const [inviteLinkMaxUses, setInviteLinkMaxUses] = useState<number | null>(null);

  // Server-synced data
  const [serverMembers, setServerMembers] = useState<any[]>([]);
  const [serverInviteLinks, setServerInviteLinks] = useState<any[]>([]);
  const [serverError, setServerError] = useState("");

  const [editingStrategy, setEditingStrategy] = useState<Strategy | null>(null);
  const [strategyDialog, setStrategyDialog] = useState(false);
  const [strategyName, setStrategyName] = useState("");
  const [strategyType, setStrategyType] = useState("fallback");
  const [strategyConfig, setStrategyConfig] = useState("{}");

  const [editingTemplate, setEditingTemplate] = useState<ComboTemplate | null>(null);
  const [templateDialog, setTemplateDialog] = useState(false);
  const [templateName, setTemplateName] = useState("");
  const [templateModels, setTemplateModels] = useState("[]");
  const [templateRoutingMode, setTemplateRoutingMode] = useState("fallback");

  const [editingWebhook, setEditingWebhook] = useState<WebhookItem | null>(null);
  const [webhookDialog, setWebhookDialog] = useState(false);
  const [webhookName, setWebhookName] = useState("");
  const [webhookUrl, setWebhookUrl] = useState("");
  const [webhookEvents, setWebhookEvents] = useState("[]");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [webhookEnabled, setWebhookEnabled] = useState(true);

  const [editingSeats, setEditingSeats] = useState(false);
  const [newSeatCount, setNewSeatCount] = useState(5);

  const isTeamsOrAbove = cloudBilling?.plan_id === "teams" || cloudBilling?.plan_id === "enterprise";

  const fetchMembers = useCallback(async () => {
    if (!auth.is_logged_in || !isTeamsOrAbove) return;
    try {
      const data = await apiFetch("/tenant-admin/members");
      setServerMembers(data.data || []);
    } catch {}
  }, [auth.is_logged_in, isTeamsOrAbove]);

  const fetchInviteLinks = useCallback(async () => {
    if (!auth.is_logged_in || !isTeamsOrAbove) return;
    try {
      const data = await apiFetch("/tenant-admin/invite-links");
      setServerInviteLinks(data.data || []);
    } catch {}
  }, [auth.is_logged_in, isTeamsOrAbove]);

  useEffect(() => {
    if (!auth.is_logged_in) return;
    if (!isTeamsOrAbove) { setLoading(false); return; }
    Promise.all([fetchMembers(), fetchInviteLinks()]).finally(() => setLoading(false));
  }, [auth.is_logged_in, isTeamsOrAbove, fetchMembers, fetchInviteLinks]);

  /* --- Member handlers --- */
  const handleMemberInvite = async () => {
    const emails = inviteEmails.split("\n").map((e) => e.trim()).filter(Boolean);
    if (emails.length === 0) return;
    setServerError("");
    try {
      await apiFetch("/tenant-admin/members/invite", {
        method: "POST",
        body: JSON.stringify({ emails, role: inviteRole }),
      });
      store.addMembersBulk(emails, inviteRole);
      setInviteDialog(false);
      setInviteEmails("");
      await fetchMembers();
    } catch (e: any) {
      setServerError(t("team.inviteFailed") + e.message);
    }
  };

  const handleCreateInviteLink = async () => {
    setServerError("");
    try {
      await apiFetch("/tenant-admin/invite-links", {
        method: "POST",
        body: JSON.stringify({ role: inviteLinkRole, expires_in: inviteLinkExpiresIn, max_uses: inviteLinkMaxUses }),
      });
      store.createInviteLink(inviteLinkRole, inviteLinkExpiresIn, inviteLinkMaxUses);
      setInviteLinkDialog(false);
      await fetchInviteLinks();
    } catch (e: any) {
      setServerError(t("team.createInviteLinkFailed") + e.message);
    }
  };

  const handleCopyInviteLink = (token: string) => {
    navigator.clipboard.writeText(window.location.origin + "/invite/" + token);
  };

  const handleDeleteInviteLink = async (linkId: string) => {
    if (!confirm(t("team.confirmDeleteInviteLink"))) return;
    setServerError("");
    try {
      await apiFetch(`/tenant-admin/invite-links/${linkId}`, { method: "DELETE" });
      store.deleteInviteLink(linkId);
      await fetchInviteLinks();
    } catch (e: any) {
      setServerError(t("team.deleteInviteLinkFailed") + e.message);
    }
  };

  const handleRoleChange = async (memberId: string, role: string) => {
    try {
      await apiFetch(`/tenant-admin/members/${memberId}/role`, { method: "PUT", body: JSON.stringify({ role }) });
      store.updateMemberRole(memberId, role);
    } catch (e: any) {
      alert(t("team.updateRoleFailed") + e.message);
    }
  };

  const handleDisableMember = async (memberId: string) => {
    try {
      await apiFetch(`/tenant-admin/members/${memberId}/disable`, { method: "POST" });
      store.setMemberStatus(memberId, "disabled");
    } catch (e: any) {
      alert(t("team.disableMemberFailed") + e.message);
    }
  };

  const handleEnableMember = async (memberId: string) => {
    try {
      await apiFetch(`/tenant-admin/members/${memberId}/enable`, { method: "POST" });
      store.setMemberStatus(memberId, "active");
    } catch (e: any) {
      alert(t("team.enableMemberFailed") + e.message);
    }
  };

  const handleRemoveMember = async (memberId: string) => {
    if (!confirm(t("team.confirmRemoveMember"))) return;
    try {
      await apiFetch(`/tenant-admin/members/${memberId}`, { method: "DELETE" });
      store.removeMember(memberId);
    } catch (e: any) {
      alert(t("team.removeMemberFailed") + e.message);
    }
  };

  /* --- Profile handler --- */
  const handleUpdateProfile = () => {
    store.updateProfile({
      name: profileName || undefined,
      billing_email: profileBillingEmail || undefined,
    } as any);
    setEditingProfile(false);
  };

  /* --- Strategy handlers --- */
  const handleSaveStrategy = () => {
    if (editingStrategy) {
      store.updateStrategy(editingStrategy.strategy_id, { name: strategyName, strategy_type: strategyType, config: strategyConfig });
    } else {
      store.addStrategy(strategyName, strategyType, strategyConfig);
    }
    resetStrategyDialog();
  };

  const handleDeleteStrategy = (id: string) => {
    if (!confirm(t("team.confirmDeleteStrategy"))) return;
    store.deleteStrategy(id);
  };

  /* --- Template handlers --- */
  const handleSaveTemplate = () => {
    if (editingTemplate) {
      store.updateTemplate(editingTemplate.template_id, { name: templateName, models: templateModels, routing_mode: templateRoutingMode });
    } else {
      store.addTemplate(templateName, templateModels, templateRoutingMode);
    }
    resetTemplateDialog();
  };

  const handleDeleteTemplate = (id: string) => {
    if (!confirm(t("team.confirmDeleteTemplate"))) return;
    store.deleteTemplate(id);
  };

  /* --- Webhook handlers --- */
  const handleSaveWebhook = () => {
    if (editingWebhook) {
      store.updateWebhook(editingWebhook.webhook_id, { name: webhookName, url: webhookUrl, events: webhookEvents, secret: webhookSecret, enabled: webhookEnabled });
    } else {
      store.addWebhook({ name: webhookName, url: webhookUrl, events: webhookEvents, secret: webhookSecret, enabled: webhookEnabled });
    }
    resetWebhookDialog();
  };

  const handleDeleteWebhook = (id: string) => {
    if (!confirm(t("team.confirmDeleteWebhook"))) return;
    store.deleteWebhook(id);
  };

  const resetStrategyDialog = () => {
    setEditingStrategy(null);
    setStrategyDialog(false);
    setStrategyName("");
    setStrategyType("fallback");
    setStrategyConfig("{}");
  };

  const resetTemplateDialog = () => {
    setEditingTemplate(null);
    setTemplateDialog(false);
    setTemplateName("");
    setTemplateModels("[]");
    setTemplateRoutingMode("fallback");
  };

  const resetWebhookDialog = () => {
    setEditingWebhook(null);
    setWebhookDialog(false);
    setWebhookName("");
    setWebhookUrl("");
    setWebhookEvents("[]");
    setWebhookSecret("");
    setWebhookEnabled(true);
  };

  if (!auth.is_logged_in) {
    return (
      <div className="page team-page">
        <div className="empty-state">
          <h2>{t("team.pleaseLogin")}</h2>
          <p>{t("team.loginToViewWorkspace")}</p>
        </div>
      </div>
    );
  }

  if (!isTeamsOrAbove) {
    return (
      <div className="page team-page">
        <div className="empty-state">
          <div className="empty-icon">👥</div>
          <h2>{t("team.workspace")}</h2>
          <p>{t("team.upgradeToTeams")}</p>
          <button className="btn-primary" onClick={() => navigate("/subscription")}>{t("team.viewPlans")}</button>
        </div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="page team-page">
        <div className="loading-state">
          <div className="spinner" />
          <p>{t("team.loading")}</p>
        </div>
      </div>
    );
  }

  /* ======== RENDER HELPERS ======== */

  const renderOverview = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{profile.name || t("team.defaultName")}</h2>
          <div style={{ display: "flex", gap: 4 }}>
            <button className="btn-secondary btn-sm" onClick={() => { setProfileName(profile.name); setProfileBillingEmail(profile.billing_email || ""); setEditingProfile(true); }}>{t("team.edit")}</button>
          </div>
        </div>
        <div className="hero-metrics" style={{ marginTop: 8 }}>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("team.members")}</span>
            <strong className="hero-metric-value">{serverMembers.filter((m) => m.status === "active").length}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("team.seats")}</span>
            <strong className="hero-metric-value">{seatInfo.seats}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("team.strategies")}</span>
            <strong className="hero-metric-value">{strategies.length}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("team.templates")}</span>
            <strong className="hero-metric-value">{templates.length}</strong>
          </div>
        </div>
        {profile.billing_email && (
          <div style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 8 }}>
            {t("team.billingEmail")}: {profile.billing_email}
          </div>
        )}
        <div className="snapshot-footer">
          <button className="btn-secondary btn-sm" onClick={() => setActiveTab("members")}>{t("team.manageMembers")}</button>
          <button className="btn-secondary btn-sm" onClick={() => setActiveTab("strategy")}>{t("team.manageStrategies")}</button>
          <button className="btn-secondary btn-sm" onClick={() => setActiveTab("usage")}>{t("team.viewUsage")}</button>
        </div>
      </div>

      {costAttribution && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("team.monthlyCost")}</h2>
            <p className="hint">{costAttribution.month} · {t("team.vsLastMonth")} {costAttribution.total_cost > costAttribution.previous_cost ? "↑" : "↓"} ${Math.abs(costAttribution.total_cost - costAttribution.previous_cost).toFixed(2)}</p>
          </div>
          <div className="hero-metrics" style={{ marginTop: 8 }}>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.monthlyTotalCost")}</span>
              <strong className="hero-metric-value">${(costAttribution.total_cost || 0).toFixed(2)}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.previousMonthCost")}</span>
              <strong className="hero-metric-value">${(costAttribution.previous_cost || 0).toFixed(2)}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{costAttribution.members?.length || 0} {t("team.members")}</span>
              <strong className="hero-metric-value">{costAttribution.model_breakdown?.length || 0} {t("team.models")}</strong>
            </div>
          </div>
          {costAttribution.members?.slice(0, 5).map((m: any) => (
            <div key={m.member_id} className="member-card" style={{ borderBottom: "1px solid var(--border)", padding: "10px 4px", borderRadius: 0 }}>
              <div className="member-avatar">{m.email[0]?.toUpperCase()}</div>
              <div className="member-info">
                <div className="member-email">{m.email}</div>
                <div className="member-meta">
                  <span className={`member-role role-${m.role}`}>{m.role}</span>
                </div>
              </div>
              <div style={{ textAlign: "right", fontSize: 13 }}>
                <div style={{ fontWeight: 600 }}>${(m.cost || 0).toFixed(2)}</div>
                <div style={{ color: "var(--text-muted)", fontSize: 12 }}>{m.pct?.toFixed(1)}%</div>
              </div>
            </div>
          ))}
        </div>
      )}

      {billingSummary && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("team.billingOverview")}</h2>
          </div>
          <div className="hero-metrics" style={{ marginTop: 12 }}>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.totalOrders")}</span>
              <strong className="hero-metric-value">{billingSummary.order_count || 0}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.paidOrders")}</span>
              <strong className="hero-metric-value">{billingSummary.paid_orders || 0}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.totalRevenue")}</span>
              <strong className="hero-metric-value">${(billingSummary.gross_revenue || 0).toFixed(2)}</strong>
            </div>
          </div>
        </div>
      )}

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("team.seatManagement")}</h2>
          <button className="btn-primary btn-sm" onClick={() => { setNewSeatCount(seatInfo.seats); setEditingSeats(true); }}>{t("team.adjustSeats")}</button>
        </div>
        <div className="hero-metrics" style={{ marginTop: 8 }}>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("team.totalSeats")}</span>
            <strong className="hero-metric-value">{seatInfo.seats}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("team.used")}</span>
            <strong className="hero-metric-value">{seatInfo.active_members}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("team.available")}</span>
            <strong className="hero-metric-value">{seatInfo.available}</strong>
          </div>
        </div>
      </div>

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("team.quickActions")}</h2>
        </div>
        <div className="ent-governance-grid">
          <div className="governance-card" onClick={() => setActiveTab("members")} style={{ cursor: "pointer" }}>
            <div className="gov-icon">👥</div>
            <div className="gov-title">{t("team.memberCount", { count: serverMembers.length })}</div>
            <div className="gov-desc">{t("team.manageMemberAccess")}</div>
          </div>
          <div className="governance-card" onClick={() => setActiveTab("strategy")} style={{ cursor: "pointer" }}>
            <div className="gov-icon">🔀</div>
            <div className="gov-title">{t("team.strategyCount", { count: strategies.length })}</div>
            <div className="gov-desc">{t("team.configureRouting")}</div>
          </div>
          <div className="governance-card" onClick={() => setActiveTab("usage")} style={{ cursor: "pointer" }}>
            <div className="gov-icon">📊</div>
            <div className="gov-title">{t("team.usageRanking")}</div>
            <div className="gov-desc">{t("team.viewMemberRequests")}</div>
          </div>
        </div>
      </div>
    </>
  );

  const renderMembers = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("team.memberManagement")}</h2>
          <div style={{ display: "flex", gap: 4 }}>
            <button className="btn-primary btn-sm" onClick={() => setInviteDialog(true)}>{t("team.inviteMembers")}</button>
            <button className="btn-secondary btn-sm" onClick={() => setInviteLinkDialog(true)}>{t("team.createInviteLink")}</button>
          </div>
        </div>
        {serverError && <div className="error-msg" style={{ marginBottom: 8 }}>{serverError}</div>}
        <div className="member-list">
          {serverMembers.map((m: any) => (
            <div key={m.member_id} className="member-card">
              <div className="member-avatar">{m.email[0]?.toUpperCase() || "?"}</div>
              <div className="member-info">
                <div className="member-email">{m.email}</div>
                <div className="member-meta">
                  <span className={`member-role role-${m.role}`}>{m.role}</span>
                  <span className={m.status === "active" ? "badge-active" : "badge-inactive"}>{m.status}</span>
                </div>
              </div>
              <div className="member-actions">
                {m.role !== "owner" && (
                  <>
                    <select
                      value={m.role}
                      onChange={(e) => handleRoleChange(m.member_id, e.target.value)}
                      className="form-input"
                      style={{ width: 90, fontSize: 12, padding: "4px 6px" }}
                    >
                      <option value="admin">admin</option>
                      <option value="member">member</option>
                    </select>
                    {m.status === "active" ? (
                      <button className="btn-secondary btn-sm" onClick={() => handleDisableMember(m.member_id)}>{t("team.disable")}</button>
                    ) : (
                      <button className="btn-primary btn-sm" onClick={() => handleEnableMember(m.member_id)}>{t("team.enable")}</button>
                    )}
                    <button className="btn-text" style={{ color: "var(--red)" }} onClick={() => handleRemoveMember(m.member_id)}>{t("team.remove")}</button>
                  </>
                )}
                {m.role === "owner" && <span style={{ fontSize: 12, color: "var(--text-muted)" }}>{t("team.owner")}</span>}
              </div>
            </div>
          ))}
        </div>
      </div>

      {serverInviteLinks.length > 0 && (
        <div className="section section-card">
          <div className="section-heading">
            <h2>{t("team.inviteLinks")}</h2>
          </div>
          {serverInviteLinks.map((link: any) => (
            <div key={link.link_id} className="member-card">
              <div style={{ flex: 1, fontSize: 13 }}>
                <code style={{ fontSize: 12 }}>{link.token.substring(0, 16)}...</code>
                <span className="member-meta" style={{ marginLeft: 8 }}>
                  <span className={`member-role role-${link.role}`}>{link.role}</span>
                  {link.max_uses && <span>· {t("team.maxUses", { count: link.max_uses })}</span>}
                  <span>· {t("team.usedCount", { count: link.used_count })}</span>
                  {link.expires_at && <span>· {t("team.expiresAt")}: {new Date(link.expires_at).toLocaleDateString()}</span>}
                </span>
              </div>
              <div style={{ display: "flex", gap: 4 }}>
                <button className="btn-secondary btn-sm" onClick={() => handleCopyInviteLink(link.token)}>{t("team.copy")}</button>
                <button className="btn-text" style={{ color: "var(--red)" }} onClick={() => handleDeleteInviteLink(link.link_id)}>{t("team.delete")}</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  );

  const renderUsage = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("team.teamUsageRanking")}</h2>
          <p className="hint">{t("team.usageByMemberAndModel")}</p>
        </div>
        {memberUsage.length === 0 ? (
          <div className="empty-hint">{t("team.noUsageData")}</div>
        ) : (
          <div className="member-list">
            {memberUsage.map((m) => (
              <div key={m.member_id} className="member-card">
                <div className="member-avatar">{m.email[0]?.toUpperCase() || "?"}</div>
                <div className="member-info">
                  <div className="member-email">{m.email}</div>
                  <div className="member-meta">
                    <span className={`member-role role-${m.role}`}>{m.role}</span>
                  </div>
                </div>
                <div style={{ display: "flex", gap: 16, flexShrink: 0, fontSize: 13 }}>
                  <span style={{ color: "var(--text-primary)", fontWeight: 600 }}>{t("team.requestsCount", { count: m.requests.toLocaleString() })}</span>
                  <span style={{ color: "var(--text-secondary)" }}>↑{m.input_tokens.toLocaleString()}</span>
                  <span style={{ color: "var(--text-secondary)" }}>↓{m.output_tokens.toLocaleString()}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {costAttribution && (
        <>
          <div className="section section-card">
            <div className="section-heading">
              <h2>{t("team.costAttribution")}</h2>
              <p className="hint">{costAttribution.month} · {t("team.memberCostDistribution")}</p>
            </div>
            <div className="hero-metrics" style={{ marginTop: 8 }}>
              <div className="hero-metric-card">
                <span className="hero-metric-label">{t("team.totalCost")}</span>
                <strong className="hero-metric-value">${(costAttribution.total_cost || 0).toFixed(2)}</strong>
              </div>
              <div className="hero-metric-card">
                <span className="hero-metric-label">{t("team.vsLastMonth")}</span>
                <strong className="hero-metric-value" style={{ color: costAttribution.total_cost > costAttribution.previous_cost ? "var(--red)" : "var(--green)" }}>
                  {costAttribution.total_cost > costAttribution.previous_cost ? "↑" : "↓"} ${Math.abs(costAttribution.total_cost - costAttribution.previous_cost).toFixed(2)}
                </strong>
              </div>
            </div>
            {(costAttribution.members || []).length > 0 && (
              <div className="member-list">
                <div className="member-card" style={{ fontWeight: 700, fontSize: 12, color: "var(--text-muted)", padding: "8px 4px", borderBottom: "2px solid var(--border)" }}>
                  <div style={{ flex: 1 }}>{t("team.member")}</div>
                  <div style={{ width: 80, textAlign: "right" }}>{t("team.requests")}</div>
                  <div style={{ width: 80, textAlign: "right" }}>{t("team.cost")}</div>
                  <div style={{ width: 60, textAlign: "right" }}>{t("team.percentage")}</div>
                </div>
                {costAttribution.members.map((m: any) => (
                  <div key={m.member_id} className="member-card" style={{ padding: "10px 4px" }}>
                    <div className="member-avatar">{m.email[0]?.toUpperCase()}</div>
                    <div className="member-info">
                      <div className="member-email">{m.email}</div>
                    </div>
                    <div style={{ width: 80, textAlign: "right", fontSize: 13, color: "var(--text-secondary)" }}>{m.requests.toLocaleString()}</div>
                    <div style={{ width: 80, textAlign: "right", fontSize: 13, fontWeight: 600 }}>${(m.cost || 0).toFixed(2)}</div>
                    <div style={{ width: 60, textAlign: "right", fontSize: 12, color: "var(--text-muted)" }}>{(m.pct || 0).toFixed(1)}%</div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {(costAttribution.model_breakdown || []).length > 0 && (
            <div className="section section-card">
              <div className="section-heading">
                <h2>{t("team.modelDistribution")}</h2>
                <p className="hint">{t("team.usageAndCostByModel")}</p>
              </div>
              <div className="member-list">
                <div className="member-card" style={{ fontWeight: 700, fontSize: 12, color: "var(--text-muted)", padding: "8px 4px", borderBottom: "2px solid var(--border)" }}>
                  <div style={{ flex: 1 }}>{t("team.model")}</div>
                  <div style={{ width: 80, textAlign: "right" }}>{t("team.requests")}</div>
                  <div style={{ width: 100, textAlign: "right" }}>{t("team.inputTokens")}</div>
                  <div style={{ width: 100, textAlign: "right" }}>{t("team.outputTokens")}</div>
                  <div style={{ width: 70, textAlign: "right" }}>{t("team.cost")}</div>
                </div>
                {costAttribution.model_breakdown.map((m: any, i: number) => (
                  <div key={i} className="member-card" style={{ padding: "10px 4px" }}>
                    <div className="member-avatar" style={{ fontSize: 11, minWidth: 28 }}>🧠</div>
                    <div className="member-info">
                      <div className="member-email" style={{ maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis" }}>{m.model}</div>
                    </div>
                    <div style={{ width: 80, textAlign: "right", fontSize: 13, color: "var(--text-secondary)" }}>{m.requests.toLocaleString()}</div>
                    <div style={{ width: 100, textAlign: "right", fontSize: 13, color: "var(--text-secondary)" }}>{m.input_tokens.toLocaleString()}</div>
                    <div style={{ width: 100, textAlign: "right", fontSize: 13, color: "var(--text-secondary)" }}>{m.output_tokens.toLocaleString()}</div>
                    <div style={{ width: 70, textAlign: "right", fontSize: 13, fontWeight: 600 }}>${(m.cost || 0).toFixed(2)}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </>
  );

  const renderStrategy = () => (
    <>
      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("team.routingStrategy")}</h2>
          <button className="btn-primary btn-sm" onClick={() => { setEditingStrategy(null); setStrategyName(""); setStrategyType("fallback"); setStrategyConfig("{}"); setStrategyDialog(true); }}>{t("team.addStrategy")}</button>
        </div>
        {strategies.length === 0 ? (
          <div className="empty-hint">{t("team.noStrategy")}</div>
        ) : (
          <div className="member-list">
            {strategies.map((s) => (
              <div key={s.strategy_id} className="member-card">
                <div className="member-avatar">⚙️</div>
                <div className="member-info">
                  <div className="member-email">{s.name}</div>
                  <div className="member-meta">
                    <span className="member-role role-{s.strategy_type}">{s.strategy_type}</span>
                    <span style={{ fontSize: 12, color: "var(--text-muted)" }}>{s.updated_at?.substring(0, 10)}</span>
                  </div>
                </div>
                <div className="member-actions">
                  <button className="btn-secondary btn-sm" onClick={() => { setEditingStrategy(s); setStrategyName(s.name); setStrategyType(s.strategy_type); setStrategyConfig(s.config); setStrategyDialog(true); }}>{t("team.edit")}</button>
                  <button className="btn-text" style={{ color: "var(--red)" }} onClick={() => handleDeleteStrategy(s.strategy_id)}>{t("team.delete")}</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section section-card">
        <div className="section-heading">
          <h2>{t("team.comboTemplates")}</h2>
          <button className="btn-primary btn-sm" onClick={() => { setEditingTemplate(null); setTemplateName(""); setTemplateModels("[]"); setTemplateRoutingMode("fallback"); setTemplateDialog(true); }}>{t("team.addTemplate")}</button>
        </div>
        {templates.length === 0 ? (
          <div className="empty-hint">{t("team.noTemplates")}</div>
        ) : (
          <div className="member-list">
            {templates.map((tpl) => (
              <div key={tpl.template_id} className="member-card">
                <div className="member-avatar">📋</div>
                <div className="member-info">
                  <div className="member-email">{tpl.name}</div>
                  <div className="member-meta">
                    <span className={`member-role role-${tpl.routing_mode}`}>{tpl.routing_mode}</span>
                    <span style={{ fontSize: 12, color: "var(--text-muted)" }}>{tpl.updated_at?.substring(0, 10)}</span>
                  </div>
                </div>
                <div className="member-actions">
                  <button className="btn-secondary btn-sm" onClick={() => { setEditingTemplate(tpl); setTemplateName(tpl.name); setTemplateModels(tpl.models); setTemplateRoutingMode(tpl.routing_mode); setTemplateDialog(true); }}>{t("team.edit")}</button>
                  <button className="btn-text" style={{ color: "var(--red)" }} onClick={() => handleDeleteTemplate(tpl.template_id)}>{t("team.delete")}</button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );

  const renderBilling = () => (
    <>
      {billingSummary && (
        <div className="section section-card">
          <div className="section-heading"><h2>{t("team.billingOverview")}</h2></div>
          <div className="hero-metrics" style={{ marginTop: 12 }}>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.totalOrders")}</span>
              <strong className="hero-metric-value">{billingSummary.order_count || 0}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.paidOrders")}</span>
              <strong className="hero-metric-value">{billingSummary.paid_orders || 0}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("team.totalRevenue")}</span>
              <strong className="hero-metric-value">${(billingSummary.gross_revenue || 0).toFixed(2)}</strong>
            </div>
          </div>
        </div>
      )}

      <div className="section section-card">
        <div className="section-heading"><h2>{t("team.invoiceRecords")}</h2></div>
        {invoices.length === 0 ? (
          <div className="empty-hint">{t("team.noInvoices")}</div>
        ) : (
          <div className="member-list">
            {invoices.map((inv) => (
              <div key={inv.invoice_id} className="member-card">
                <div className="member-avatar">📄</div>
                <div className="member-info">
                  <div className="member-email">{inv.period_start?.substring(0, 10)} ~ {inv.period_end?.substring(0, 10)}</div>
                  <div className="member-meta">
                    <span>{inv.currency} ${inv.amount.toFixed(2)}</span>
                    <span className={inv.status === "paid" ? "badge-active" : "badge-inactive"}>{inv.status}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="section section-card">
        <div className="section-heading"><h2>{t("team.overageRecords")}</h2></div>
        {overages.length === 0 ? (
          <div className="empty-hint">{t("team.noOverage")}</div>
        ) : (
          <div className="member-list">
            {overages.map((o) => (
              <div key={o.overage_id} className="member-card">
                <div className="member-avatar">⚠️</div>
                <div className="member-info">
                  <div className="member-email">{o.billing_period}</div>
                  <div className="member-meta">
                    <span>{t("team.overageRequests", { count: o.overage_requests.toLocaleString() })}</span>
                    <span>${o.overage_cost.toFixed(2)}</span>
                    <span className={o.billed ? "badge-active" : "badge-inactive"}>{o.billed ? t("team.settled") : t("team.unsettled")}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );

  const renderWebhook = () => (
    <div className="section section-card">
      <div className="section-heading">
        <h2>{t("team.webhooks")}</h2>
        <button className="btn-primary btn-sm" onClick={() => { setEditingWebhook(null); setWebhookName(""); setWebhookUrl(""); setWebhookEvents("[]"); setWebhookSecret(""); setWebhookEnabled(true); setWebhookDialog(true); }}>{t("team.addWebhook")}</button>
      </div>
      {webhooks.length === 0 ? (
        <div className="empty-hint">{t("team.noWebhooks")}</div>
      ) : (
        <div className="member-list">
          {webhooks.map((w) => (
            <div key={w.webhook_id} className="member-card">
              <div className="member-avatar">🔗</div>
              <div className="member-info">
                <div className="member-email">{w.name}</div>
                <div className="member-meta">
                  <span style={{ fontSize: 12, color: "var(--text-muted)" }}>{w.url.substring(0, 40)}</span>
                  <span className={w.enabled ? "badge-active" : "badge-inactive"}>{w.enabled ? t("team.enabled") : t("team.disabled")}</span>
                </div>
              </div>
              <div className="member-actions">
                <button className="btn-secondary btn-sm" onClick={() => { setEditingWebhook(w); setWebhookName(w.name); setWebhookUrl(w.url); setWebhookEvents(w.events); setWebhookSecret(w.secret); setWebhookEnabled(w.enabled); setWebhookDialog(true); }}>{t("team.edit")}</button>
                <button className="btn-text" style={{ color: "var(--red)" }} onClick={() => handleDeleteWebhook(w.webhook_id)}>{t("team.delete")}</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );

  /* ======== DIALOGS ======== */
  const renderDialogs = () => (
    <>
      {/* Profile Edit */}
      {editingProfile && (
        <div className="dialog-overlay" onClick={() => setEditingProfile(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header">
              <h3>{t("team.editTeamInfo")}</h3>
              <button className="btn-text" onClick={() => setEditingProfile(false)}>✕</button>
            </div>
            <div className="dialog-body">
              <label className="form-label">{t("team.teamName")}</label>
              <input type="text" value={profileName} onChange={(e) => setProfileName(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.billingEmail")}</label>
              <input type="email" value={profileBillingEmail} onChange={(e) => setProfileBillingEmail(e.target.value)} className="form-input" />
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setEditingProfile(false)}>{t("team.cancel")}</button>
              <button className="btn-primary" onClick={handleUpdateProfile}>{t("team.save")}</button>
            </div>
          </div>
        </div>
      )}

      {/* Invite Dialog */}
      {inviteDialog && (
        <div className="dialog-overlay" onClick={() => setInviteDialog(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header">
              <h3>{t("team.inviteMember")}</h3>
              <button className="btn-text" onClick={() => setInviteDialog(false)}>✕</button>
            </div>
            <div className="dialog-body">
              <label className="form-label">{t("team.emailAddressPerLine")}</label>
              <textarea value={inviteEmails} onChange={(e) => setInviteEmails(e.target.value)} className="form-input" rows={4} placeholder="user1@example.com&#10;user2@example.com" />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.role")}</label>
              <select value={inviteRole} onChange={(e) => setInviteRole(e.target.value)} className="form-input">
                <option value="admin">{t("team.admin")}</option>
                <option value="member">{t("team.member")}</option>
              </select>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setInviteDialog(false)}>{t("team.cancel")}</button>
              <button className="btn-primary" onClick={handleMemberInvite}>{t("team.invite")}</button>
            </div>
          </div>
        </div>
      )}

      {/* Invite Link Dialog */}
      {inviteLinkDialog && (
        <div className="dialog-overlay" onClick={() => setInviteLinkDialog(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header">
              <h3>{t("team.createInviteLinkTitle")}</h3>
              <button className="btn-text" onClick={() => setInviteLinkDialog(false)}>✕</button>
            </div>
            <div className="dialog-body">
              <label className="form-label">{t("team.role")}</label>
              <select value={inviteLinkRole} onChange={(e) => setInviteLinkRole(e.target.value)} className="form-input">
                <option value="admin">{t("team.admin")}</option>
                <option value="member">{t("team.member")}</option>
              </select>
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.expiresIn")}</label>
              <select value={inviteLinkExpiresIn} onChange={(e) => setInviteLinkExpiresIn(Number(e.target.value))} className="form-input">
                <option value={3600}>{t("team.oneHour")}</option>
                <option value={86400}>{t("team.twentyFourHours")}</option>
                <option value={604800}>{t("team.sevenDays")}</option>
                <option value={2592000}>{t("team.thirtyDays")}</option>
                <option value={0}>{t("team.neverExpire")}</option>
              </select>
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.maxUsageCount")}</label>
              <input type="number" value={inviteLinkMaxUses ?? ""} onChange={(e) => setInviteLinkMaxUses(e.target.value ? Number(e.target.value) : null)} className="form-input" placeholder={t("team.unlimitedPlaceholder")} />
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setInviteLinkDialog(false)}>{t("team.cancel")}</button>
              <button className="btn-primary" onClick={handleCreateInviteLink}>{t("team.create")}</button>
            </div>
          </div>
        </div>
      )}

      {/* Strategy Dialog */}
      {strategyDialog && (
        <div className="dialog-overlay" onClick={resetStrategyDialog}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header">
              <h3>{editingStrategy ? t("team.editStrategy") : t("team.addStrategyTitle")}</h3>
              <button className="btn-text" onClick={resetStrategyDialog}>✕</button>
            </div>
            <div className="dialog-body">
              <label className="form-label">{t("team.name")}</label>
              <input type="text" value={strategyName} onChange={(e) => setStrategyName(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.type")}</label>
              <select value={strategyType} onChange={(e) => setStrategyType(e.target.value)} className="form-input">
                <option value="fallback">Fallback</option>
                <option value="round_robin">Round Robin</option>
                <option value="priority">Priority</option>
                <option value="latency">Latency-based</option>
              </select>
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.configJSON")}</label>
              <textarea value={strategyConfig} onChange={(e) => setStrategyConfig(e.target.value)} className="form-input" rows={4} />
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={resetStrategyDialog}>{t("team.cancel")}</button>
              <button className="btn-primary" onClick={handleSaveStrategy}>{t("team.save")}</button>
            </div>
          </div>
        </div>
      )}

      {/* Template Dialog */}
      {templateDialog && (
        <div className="dialog-overlay" onClick={resetTemplateDialog}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header">
              <h3>{editingTemplate ? t("team.editTemplate") : t("team.addTemplateTitle")}</h3>
              <button className="btn-text" onClick={resetTemplateDialog}>✕</button>
            </div>
            <div className="dialog-body">
              <label className="form-label">{t("team.name")}</label>
              <input type="text" value={templateName} onChange={(e) => setTemplateName(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.modelListJSON")}</label>
              <textarea value={templateModels} onChange={(e) => setTemplateModels(e.target.value)} className="form-input" rows={3} placeholder='["gpt-4","claude-3"]' />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.routingMode")}</label>
              <select value={templateRoutingMode} onChange={(e) => setTemplateRoutingMode(e.target.value)} className="form-input">
                <option value="fallback">Fallback</option>
                <option value="round_robin">Round Robin</option>
              </select>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={resetTemplateDialog}>{t("team.cancel")}</button>
              <button className="btn-primary" onClick={handleSaveTemplate}>{t("team.save")}</button>
            </div>
          </div>
        </div>
      )}

      {/* Webhook Dialog */}
      {webhookDialog && (
        <div className="dialog-overlay" onClick={resetWebhookDialog}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header">
              <h3>{editingWebhook ? t("team.editWebhook") : t("team.addWebhookTitle")}</h3>
              <button className="btn-text" onClick={resetWebhookDialog}>✕</button>
            </div>
            <div className="dialog-body">
              <label className="form-label">{t("team.name")}</label>
              <input type="text" value={webhookName} onChange={(e) => setWebhookName(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.url")}</label>
              <input type="url" value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.eventsJSON")}</label>
              <input type="text" value={webhookEvents} onChange={(e) => setWebhookEvents(e.target.value)} className="form-input" placeholder='["member.invited","strategy.updated"]' />
              <label className="form-label" style={{ marginTop: 12 }}>{t("team.secret")}</label>
              <input type="text" value={webhookSecret} onChange={(e) => setWebhookSecret(e.target.value)} className="form-input" />
              <label className="form-label" style={{ marginTop: 12 }}>
                <input type="checkbox" checked={webhookEnabled} onChange={(e) => setWebhookEnabled(e.target.checked)} /> {t("team.enabled")}
              </label>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={resetWebhookDialog}>{t("team.cancel")}</button>
              <button className="btn-primary" onClick={handleSaveWebhook}>{t("team.save")}</button>
            </div>
          </div>
        </div>
      )}

      {/* Seats Dialog */}
      {editingSeats && (
        <div className="dialog-overlay" onClick={() => setEditingSeats(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-header">
              <h3>{t("team.adjustSeatsTitle")}</h3>
              <button className="btn-text" onClick={() => setEditingSeats(false)}>✕</button>
            </div>
            <div className="dialog-body">
              <label className="form-label">{t("team.seatCount")}</label>
              <div className="seat-selector">
                <button className="seat-btn" onClick={() => setNewSeatCount(Math.max(1, newSeatCount - 1))} disabled={newSeatCount <= 1}>−</button>
                <span className="seat-count">{t("team.seatCountValue", { count: newSeatCount })}</span>
                <button className="seat-btn" onClick={() => setNewSeatCount(Math.min(1000, newSeatCount + 1))} disabled={newSeatCount >= 1000}>+</button>
              </div>
            </div>
            <div className="dialog-footer">
              <button className="btn-secondary" onClick={() => setEditingSeats(false)}>{t("team.cancel")}</button>
              <button className="btn-primary" onClick={() => { store.updateSeats(newSeatCount); setEditingSeats(false); }}>{t("team.save")}</button>
            </div>
          </div>
        </div>
      )}
    </>
  );

  /* ======== MAIN RENDER ======== */
  const tabs: { id: TabId; label: string; icon: string }[] = [
    { id: "overview", label: t("team.overview"), icon: "📊" },
    { id: "members", label: t("team.members"), icon: "👥" },
    { id: "usage", label: t("team.usage"), icon: "📈" },
    { id: "strategy", label: t("team.strategy"), icon: "🔀" },
    { id: "billing", label: t("team.billing"), icon: "💰" },
  ];

  return (
    <div className="page team-page">
      <div className="page-header">
        <h1>{profile.name || t("team.workspaceTitle")}</h1>
        <p className="hint">{t("team.manageTeamSettings")}</p>
      </div>

      <div className="tab-bar">
        {tabs.map((tab) => (
          <button key={tab.id} className={activeTab === tab.id ? "active" : ""} onClick={() => setActiveTab(tab.id)}>
            {tab.icon} {tab.label}
          </button>
        ))}
      </div>

      {activeTab === "overview" && renderOverview()}
      {activeTab === "members" && renderMembers()}
      {activeTab === "usage" && renderUsage()}
      {activeTab === "strategy" && renderStrategy()}
      {activeTab === "billing" && renderBilling()}
      {activeTab === "webhook" && renderWebhook()}

      {renderDialogs()}
    </div>
  );
}
