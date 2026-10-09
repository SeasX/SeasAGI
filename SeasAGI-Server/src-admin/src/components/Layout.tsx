import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "../i18n";

const LANG_LABELS: Record<string, string> = {
  "zh-CN": "中文",
  en: "EN",
  ja: "日本語",
  ko: "한국어",
};

const navConfig = [
  { to: "/", key: "admin.navDashboard" },
  { to: "/users", key: "admin.navUsers" },
  { to: "/channels", key: "admin.navChannels" },
  { to: "/combos", key: "admin.navCombos" },
  { to: "/relay-gateways", key: "admin.navGateways" },
  { to: "/usage", key: "admin.navUsage" },
];

export function Layout() {
  const { t, locale, setLocale } = useTranslation();

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="sidebar-logo">
          <span style={{ fontSize: 20 }}>⚡</span>
          <div className="sidebar-logo-text">
            <span>SeasAGI</span> Admin
          </div>
        </div>
        <ul className="sidebar-nav">
          {navConfig.map((item) => (
            <li key={item.to}>
              <NavLink
                to={item.to}
                end={item.to === "/"}
                className={({ isActive }) => (isActive ? "active" : "")}
              >
                {t(item.key)}
              </NavLink>
            </li>
          ))}
        </ul>
        <div className="sidebar-footer">
          <select
            value={locale}
            onChange={(e) => setLocale(e.target.value as any)}
            style={{
              width: "100%",
              padding: "6px 8px",
              fontSize: 13,
              borderRadius: 6,
              border: "1px solid var(--border)",
              backgroundColor: "transparent",
              color: "var(--text)",
              cursor: "pointer",
            }}
          >
            {Object.entries(LANG_LABELS).map(([lang, label]) => (
              <option key={lang} value={lang}>
                {label}
              </option>
            ))}
          </select>
        </div>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}