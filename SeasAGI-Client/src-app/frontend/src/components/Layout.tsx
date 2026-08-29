import { useEffect, useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import { useTranslation } from "../i18n";
import { getCloudBilling, getRuntimeStatus } from "../utils/commands";
import { useConfigSync } from "../hooks/useConfigSync";
import seasagiIcon from "../assets/seasagi-icon.png";

type AppIconName =
  | "home"
  | "channels"
  | "combo"
  | "optimization"
  | "playground"
  | "usage"
  | "key"
  | "subscription"
  | "logs"
  | "settings"
  | "sun"
  | "lock"
  | "layers"
  | "store"
  | "diagnostics"
  | "plugin"
  | "chevron";

type NavEntry =
  | { kind: "item"; to: string; label: string; icon: AppIconName; cloudOnly?: boolean }
  | { kind: "group"; id: string; label: string; icon: AppIconName; children: Array<{ to: string; label: string; icon: AppIconName; cloudOnly?: boolean }> };

const navEntries: NavEntry[] = [
  { kind: "item", to: "/", label: "nav.home", icon: "home" },
  { kind: "item", to: "/channels", label: "nav.channels", icon: "channels" },
  { kind: "item", to: "/combo-workbench", label: "nav.comboWorkbench", icon: "layers" },
  { kind: "item", to: "/playground", label: "nav.playground", icon: "playground" },
  { kind: "group", id: "data-security", label: "nav.dataSecurity", icon: "key", children: [
    { to: "/access-token", label: "nav.accessToken", icon: "key" },
    { to: "/diagnostics", label: "nav.diagnostics", icon: "diagnostics" },
  ]},
  { kind: "group", id: "data-compliance", label: "nav.dataCompliance", icon: "combo", children: [
    { to: "/enterprise", label: "nav.enterprise", icon: "combo" },
  ]},
  { kind: "group", id: "budget-control", label: "nav.budgetControl", icon: "subscription", children: [
    { to: "/subscription", label: "nav.subscription", icon: "subscription" },
    { to: "/usage", label: "nav.usage", icon: "usage" },
    { to: "/token-market", label: "nav.tokenMarket", icon: "store", cloudOnly: true },
  ]},
  { kind: "group", id: "audit-governance", label: "nav.auditGovernance", icon: "logs", children: [
    { to: "/logs", label: "nav.logs", icon: "logs" },
    { to: "/plugins", label: "nav.plugins", icon: "plugin" },
  ]},
  { kind: "item", to: "/settings", label: "nav.settings", icon: "settings" },
];

type ThemeMode = "light" | "dark";

function getInitialTheme(): ThemeMode {
  if (typeof window === "undefined") {
    return "dark";
  }
  const saved = window.localStorage.getItem("seasagi-theme");
  if (saved === "light" || saved === "dark") {
    return saved;
  }
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function AppIcon({ name, className = "" }: { name: AppIconName; className?: string }) {
  const props = {
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.8,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    className,
    "aria-hidden": true,
  };

  switch (name) {
    case "home":
      return <svg {...props}><path d="M3 10.5L12 3l9 7.5" /><path d="M5.5 9.5V20h13V9.5" /><path d="M9.5 20v-5.5h5V20" /></svg>;
    case "channels":
      return <svg {...props}><path d="M8.5 7.5h8a3 3 0 010 6h-8a3 3 0 010-6z" /><path d="M7.5 10.5h-2" /><path d="M18.5 10.5h2" /><path d="M8.5 13.5h8a3 3 0 010 6h-8a3 3 0 010-6z" /><path d="M7.5 16.5h-2" /><path d="M18.5 16.5h2" /></svg>;
    case "combo":
      return <svg {...props}><rect x="4" y="5" width="6" height="6" rx="2" /><rect x="14" y="5" width="6" height="6" rx="2" /><rect x="9" y="13" width="6" height="6" rx="2" /><path d="M10 8h4" /><path d="M12 11v2" /></svg>;
    case "optimization":
      return <svg {...props}><path d="M13 2L6 13h5l-1 9 8-12h-5l0-8z" /></svg>;
    case "playground":
      return <svg {...props}><path d="M7 6.5h10" /><path d="M7 12h10" /><path d="M7 17.5h6" /><rect x="3.5" y="3.5" width="17" height="17" rx="4" /></svg>;
    case "usage":
      return <svg {...props}><path d="M5 19V10" /><path d="M12 19V5" /><path d="M19 19v-7" /></svg>;
    case "key":
      return <svg {...props}><circle cx="8" cy="12" r="3" /><path d="M11 12h9" /><path d="M17 12v3" /><path d="M20 12v2" /></svg>;
    case "subscription":
      return <svg {...props}><rect x="3.5" y="6" width="17" height="12" rx="3" /><path d="M3.5 10h17" /><path d="M7 15h4" /></svg>;
    case "logs":
      return <svg {...props}><path d="M8 7.5h8" /><path d="M8 12h8" /><path d="M8 16.5h5" /><rect x="4" y="4" width="16" height="16" rx="3" /></svg>;
    case "settings":
      return <svg {...props}><circle cx="12" cy="12" r="3.2" /><path d="M19.4 15a1 1 0 00.2 1.1l.1.1a2 2 0 01-2.8 2.8l-.1-.1a1 1 0 00-1.1-.2 1 1 0 00-.6.9V20a2 2 0 01-4 0v-.2a1 1 0 00-.7-.9 1 1 0 00-1.1.2l-.1.1a2 2 0 01-2.8-2.8l.1-.1a1 1 0 00.2-1.1 1 1 0 00-.9-.6H4a2 2 0 010-4h.2a1 1 0 00.9-.7 1 1 0 00-.2-1.1l-.1-.1a2 2 0 012.8-2.8l.1.1a1 1 0 001.1.2 1 1 0 00.6-.9V4a2 2 0 014 0v.2a1 1 0 00.7.9 1 1 0 001.1-.2l.1-.1a2 2 0 012.8 2.8l-.1.1a1 1 0 00-.2 1.1 1 1 0 00.9.6H20a2 2 0 010 4h-.2a1 1 0 00-.9.7z" /></svg>;
    case "sun":
      return <svg {...props}><circle cx="12" cy="12" r="3.5" /><path d="M12 2.5v2.2" /><path d="M12 19.3v2.2" /><path d="M4.7 4.7l1.6 1.6" /><path d="M17.7 17.7l1.6 1.6" /><path d="M2.5 12h2.2" /><path d="M19.3 12h2.2" /><path d="M4.7 19.3l1.6-1.6" /><path d="M17.7 6.3l1.6-1.6" /></svg>;
    case "lock":
      return <svg {...props}><rect x="5" y="10" width="14" height="10" rx="3" /><path d="M8 10V8a4 4 0 018 0v2" /></svg>;
    case "layers":
      return <svg {...props}><polygon points="12 2 22 7 12 12 2 7 12 2" /><polyline points="2 12 12 17 22 12" /><polyline points="2 17 12 22 22 17" /></svg>;
    case "store":
      return <svg {...props}><path d="M3 9l1.5-5h15L21 9" /><path d="M3 9v11h18V9" /><path d="M9 20v-6h6v6" /></svg>;
    case "diagnostics":
      return <svg {...props}><path d="M12 2a10 10 0 100 20 10 10 0 000-20z" /><path d="M12 6v6l4 2" /></svg>;
    case "plugin":
      return <svg {...props}><path d="M9 3v4M15 3v4M7 7h10v4a3 3 0 01-3 3h-4a3 3 0 01-3-3V7z" /><path d="M12 14v4M10 21h4" /></svg>;
    case "chevron":
      return <svg {...props}><path d="M6 9l6 6 6-6" /></svg>;
    default:
      return null;
  }
}

export function Layout({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation();
  const auth = useAppStore((s) => s.auth);
  const cloudBilling = useAppStore((s) => s.cloudBilling);
  const setRuntime = useAppStore((s) => s.setRuntime);
  const setCloudBilling = useAppStore((s) => s.setCloudBilling);
  const location = useLocation();
  const [theme, setTheme] = useState<ThemeMode>(getInitialTheme);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());

  const toggleGroup = (id: string) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  // Auto-expand group containing current route
  useEffect(() => {
    for (const entry of navEntries) {
      if (entry.kind === "group") {
        for (const child of entry.children) {
          if (location.pathname === child.to || (child.to !== "/" && location.pathname.startsWith(child.to))) {
            setExpandedGroups((prev) => new Set(prev).add(entry.id));
            return;
          }
        }
      }
    }
    // Also check enterprise route
    if (location.pathname === "/enterprise") {
      setExpandedGroups((prev) => new Set(prev).add("data-compliance"));
    }
  }, [location.pathname]);

  const isAuthPage = location.pathname === "/auth";
  useEffect(() => {
    (async () => {
      try {
        const status = await getRuntimeStatus();
        setRuntime(status);
      } catch {}
    })();
  }, []);

  useEffect(() => {
    if (!auth.is_logged_in) {
      setCloudBilling(null);
      return;
    }
    (async () => {
      try {
        const billing = await getCloudBilling();
        setCloudBilling(billing);
      } catch {
        setCloudBilling(null);
      }
    })();
  }, [auth.is_logged_in, setCloudBilling]);

  useConfigSync();

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    document.body.setAttribute("data-theme", theme);
    window.localStorage.setItem("seasagi-theme", theme);
  }, [theme]);

  return (
    <div className="app-frame">
      <div className="app-chrome">
        <button
          type="button"
          className="theme-toggle"
          onClick={() => setTheme((prev) => (prev === "dark" ? "light" : "dark"))}
          aria-label={theme === "dark" ? t("layout.themeSwitchToLight") : t("layout.themeSwitchToDark")}
          title={theme === "dark" ? t("layout.themeSwitchToLight") : t("layout.themeSwitchToDark")}
        >
          <AppIcon name="sun" />
        </button>
      </div>
      <div className="app-layout">
        <aside className="sidebar">
          <div className="sidebar-header">
            <div className="sidebar-title-row">
              <div className="sidebar-logo-container">
                <img src={seasagiIcon} alt="SeasAGI" className="sidebar-logo-img" />
                <div className="sidebar-logo-info">
                  <span className="logo-text">SeasAGI</span>
                  <span className="logo-version">v0.1.0</span>
                </div>
              </div>
              <AppIcon name="chevron" className="title-chevron" />
            </div>
          </div>
          <nav className="sidebar-nav">
            {navEntries.map((entry) => {
              if (entry.kind === "item") {
                const disabled = !!entry.cloudOnly && !auth.is_logged_in;
                if (disabled) {
                  return (
                    <div key={entry.to} className="nav-item nav-item-disabled" aria-disabled="true" title={t("tokenMarket.loginToUse")} onClick={() => window.location.assign("#/auth")}>
                      <span className="nav-icon"><AppIcon name={entry.icon} /></span>
                      <span className="nav-label">{t(entry.label)}</span>
                      <span className="nav-cloud-lock" title={t("tokenMarket.loginToUse")}><AppIcon name="lock" /></span>
                    </div>
                  );
                }
                return (
                  <NavLink
                    key={entry.to}
                    to={entry.to}
                    end={entry.to === "/"}
                    className={({ isActive }) => `nav-item ${isActive ? "active" : ""}`}
                  >
                    <span className="nav-icon"><AppIcon name={entry.icon} /></span>
                    <span className="nav-label">{t(entry.label)}</span>
                  </NavLink>
                );
              }
              // Group: skip data-compliance if not enterprise
              if (entry.id === "data-compliance" && !(auth.is_logged_in && cloudBilling?.plan_id === "enterprise")) {
                return null;
              }
              const isExpanded = expandedGroups.has(entry.id);
              return (
                <div key={entry.id} className="nav-group">
                  <button type="button" className="nav-group-header" onClick={() => toggleGroup(entry.id)}>
                    <span className="nav-icon"><AppIcon name={entry.icon} /></span>
                    <span className="nav-label">{t(entry.label)}</span>
                    <span className={`nav-chevron ${isExpanded ? "expanded" : ""}`}><AppIcon name="chevron" /></span>
                  </button>
                  {isExpanded && (
                    <div className="nav-group-children">
                      {entry.children.map((child) => {
                        const childDisabled = !!child.cloudOnly && !auth.is_logged_in;
                        if (childDisabled) {
                          return (
                            <div key={child.to} className="nav-item nav-item-disabled nav-sub-item" aria-disabled="true" title={t("tokenMarket.loginToUse")} onClick={() => window.location.assign("#/auth")}>
                              <span className="nav-icon"><AppIcon name={child.icon} /></span>
                              <span className="nav-label">{t(child.label)}</span>
                              <span className="nav-cloud-lock" title={t("tokenMarket.loginToUse")}><AppIcon name="lock" /></span>
                            </div>
                          );
                        }
                        return (
                          <NavLink
                            key={child.to}
                            to={child.to}
                            end={child.to === "/"}
                            className={({ isActive }) => `nav-item nav-sub-item ${isActive ? "active" : ""}`}
                          >
                            <span className="nav-icon"><AppIcon name={child.icon} /></span>
                            <span className="nav-label">{t(child.label)}</span>
                          </NavLink>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
            {auth.is_logged_in && (cloudBilling?.plan_id === "teams" || cloudBilling?.plan_id === "enterprise") && (
              <NavLink
                to="/team"
                className={({ isActive }) => `nav-item ${isActive ? "active" : ""}`}
              >
                <span className="nav-icon"><AppIcon name="combo" /></span>
                <span className="nav-label">{t("nav.team")}</span>
              </NavLink>
            )}
          </nav>
          
          <div className="sidebar-footer">
            <div className="user-profile-bar">
              {auth.is_logged_in ? (
                <div className="user-avatar-section">
                  <div className="user-avatar-container">
                    <div className="user-avatar">{auth.email?.[0]?.toUpperCase() || "U"}</div>
                    <div className="status-indicator online" />
                  </div>
                  <div className="user-info-text">
                    <div className="user-name">{auth.email?.split("@")[0] || t("layout.loggedIn")}</div>
                    <div className="user-status-text">在线</div>
                  </div>
                </div>
              ) : (
                <NavLink to="/auth" className="user-avatar-section guest">
                  <div className="user-avatar-container">
                    <div className="user-avatar guest"><AppIcon name="home" /></div>
                    <div className="status-indicator offline" />
                  </div>
                  <div className="user-info-text">
                    <div className="user-name">未登录</div>
                    <div className="user-status-text">点击登录云端</div>
                  </div>
                </NavLink>
              )}
              <div className="user-control-icons">
                <NavLink to="/settings" className="user-control-btn" title="设置"><AppIcon name="settings" /></NavLink>
              </div>
            </div>
          </div>
        </aside>

        <div className="main-shell">
          {!auth.is_logged_in && !isAuthPage && (
            <div className="readonly-banner">
              <span className="readonly-banner-copy">
                <span className="readonly-banner-icon"><AppIcon name="lock" /></span>
                <span>{t("home.localModeBanner")}</span>
              </span>
              <NavLink to="/auth" className="readonly-login-link">{t("home.localModeLoginLink")}</NavLink>
            </div>
          )}
          <main className="main-content">
            <div className="content-inner">
              {children}
            </div>
          </main>
        </div>
      </div>
    </div>
  );
}
