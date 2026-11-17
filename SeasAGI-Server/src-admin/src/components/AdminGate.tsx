import React, { useEffect, useState } from "react";
import { useTranslation } from "../i18n";
import {
  ADMIN_UNAUTHORIZED_EVENT,
  isAuthenticated,
  loginAdmin,
  logoutAdmin,
  verifySession,
} from "../utils/api";

/**
 * 社区版内嵌后台的登录门。ADMIN_SECRET 只在登录请求的 body 中提交一次，
 * 服务端据此签发 HttpOnly 会话 Cookie；前端 JS 不再持有任何长期凭据。
 * 服务端返回 401/403 时 apiFetch 广播事件，这里回到登录态。
 */
export function AdminGate({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation();
  // null = 正在向服务端确认已有会话；true = 已登录；false = 需要登录。
  const [authed, setAuthed] = useState<boolean | null>(
    isAuthenticated() ? null : false,
  );
  const [input, setInput] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  useEffect(() => {
    // 只在挂载时确认一次已有会话；依赖不能含 authed，否则状态切换会
    // 触发 cleanup 把 active 置 false，导致验证结果丢失、页面卡在检查中。
    if (!isAuthenticated()) return;
    let active = true;
    verifySession().then((ok) => {
      if (active) setAuthed(ok);
    });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    const onUnauthorized = () => {
      setAuthed(false);
      setError(t("admin.loginExpired"));
    };
    window.addEventListener(ADMIN_UNAUTHORIZED_EVENT, onUnauthorized);
    return () => window.removeEventListener(ADMIN_UNAUTHORIZED_EVENT, onUnauthorized);
  }, [t]);

  const logout = async () => {
    await logoutAdmin();
    setAuthed(false);
    setError("");
    setInput("");
  };

  if (authed === true) {
    return (
      <>
        {children}
        <button
          type="button"
          onClick={logout}
          style={{
            position: "fixed",
            right: 14,
            bottom: 14,
            zIndex: 1000,
            padding: "5px 10px",
            fontSize: 12,
            borderRadius: "var(--radius)",
            border: "1px solid var(--border)",
            background: "var(--bg-card)",
            color: "var(--text-dim)",
            cursor: "pointer",
            opacity: 0.75,
          }}
        >
          {t("admin.logout")}
        </button>
      </>
    );
  }

  if (authed === null) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center" }}>
        <div style={{ fontSize: 13, color: "var(--text-dim)" }}>{t("admin.loginVerifying")}</div>
      </div>
    );
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    if (!input.trim()) {
      setError(t("admin.loginMissing"));
      return;
    }
    setPending(true);
    try {
      if (!(await loginAdmin(input))) {
        setError(t("admin.loginInvalid"));
        return;
      }
      setInput("");
      setAuthed(true);
    } catch {
      setError(t("admin.loginInvalid"));
    } finally {
      setPending(false);
    }
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 24,
      }}
    >
      <form
        onSubmit={submit}
        style={{
          width: 360,
          display: "flex",
          flexDirection: "column",
          gap: 12,
          padding: 28,
          borderRadius: "var(--radius)",
          border: "1px solid var(--border)",
          background: "var(--bg-card)",
        }}
      >
        <div style={{ fontSize: 20, fontWeight: 600 }}>⚡ SeasAGI Admin</div>
        <div style={{ fontSize: 13, color: "var(--text-dim)" }}>{t("admin.loginHint")}</div>

        <label htmlFor="admin-secret" style={{ fontSize: 13 }}>
          {t("admin.loginSecretLabel")}
        </label>
        <input
          id="admin-secret"
          type="password"
          autoComplete="off"
          autoFocus
          value={input}
          placeholder={t("admin.loginPlaceholder")}
          onChange={(e) => setInput(e.target.value)}
          style={{
            padding: "8px 10px",
            fontSize: 14,
            borderRadius: "var(--radius)",
            border: "1px solid var(--border)",
            background: "var(--bg)",
            color: "var(--text)",
          }}
        />

        {error && <div style={{ fontSize: 13, color: "var(--red)" }}>{error}</div>}

        <button
          type="submit"
          disabled={pending}
          style={{
            padding: "8px 12px",
            fontSize: 14,
            borderRadius: "var(--radius)",
            border: "none",
            background: "var(--accent)",
            color: "#fff",
            cursor: pending ? "not-allowed" : "pointer",
            opacity: pending ? 0.7 : 1,
          }}
        >
          {pending ? t("admin.loginVerifying") : t("admin.loginSubmit")}
        </button>
      </form>
    </div>
  );
}
