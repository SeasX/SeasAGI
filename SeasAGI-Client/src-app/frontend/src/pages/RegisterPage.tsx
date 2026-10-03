import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import { useTranslation } from "../i18n";
import { validateForm, loginFormSchema, registerFormSchema } from "../utils/validation";
import type { OAuthProvider } from "../utils/types";

type AuthMode = "login" | "register";

// GoogleIcon 渲染 Google 品牌图标。
function GoogleIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 18 18" aria-hidden="true">
      <path fill="#4285F4" d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.92c1.7-1.57 2.68-3.88 2.68-6.62z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.32-1.58-5.03-3.7H.96v2.33A9 9 0 0 0 9 18z" />
      <path fill="#FBBC05" d="M3.97 10.72a5.4 5.4 0 0 1 0-3.44V4.95H.96a9 9 0 0 0 0 8.1l3.01-2.33z" />
      <path fill="#EA4335" d="M9 3.58c1.32 0 2.5.45 3.44 1.35l2.58-2.58C13.46.89 11.43 0 9 0A9 9 0 0 0 .96 4.95l3.01 2.33C4.68 5.16 6.66 3.58 9 3.58z" />
    </svg>
  );
}

// GitHubIcon 渲染 GitHub 品牌图标。
function GitHubIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
      <path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}

// oauthProviderIcon 按 provider 名称返回对应品牌图标；未知 provider 返回 null。
function oauthProviderIcon(name: string) {
  if (name === "google") return <GoogleIcon />;
  if (name === "github") return <GitHubIcon />;
  return null;
}

function extractErrorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message :
    typeof err === "string" ? err :
    err && typeof err === "object" && "message" in err ? String((err as {message: string}).message) :
    err && typeof err === "object" && "error" in err ? String((err as {error: string}).error) :
    fallback;
}

export function RegisterPage() {
  const navigate = useNavigate();
  const setAuth = useAppStore((s) => s.setAuth);
  const { t } = useTranslation();
  const [mode, setMode] = useState<AuthMode>("register");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [oauthProviders, setOauthProviders] = useState<OAuthProvider[]>([]);
  const [oauthLoading, setOauthLoading] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    cmd.getOAuthProviders()
      .then((providers) => {
        if (!cancelled) setOauthProviders(providers);
      })
      .catch(() => {
        // 服务端未配置 OAuth 时静默隐藏第三方登录入口
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const planBenefits = [
    {
      tier: t("auth.planFreeTier"),
      price: t("auth.planFreePrice"),
      audience: t("auth.planFreeAudience"),
      features: [
        t("auth.planFreeFeature1"),
        t("auth.planFreeFeature2"),
        t("auth.planFreeFeature3"),
        t("auth.planFreeFeature4"),
      ],
    },
    {
      tier: t("auth.planProTier"),
      price: t("auth.planProPrice"),
      audience: t("auth.planProAudience"),
      features: [
        t("auth.planProFeature1"),
        t("auth.planProFeature2"),
        t("auth.planProFeature3"),
        t("auth.planProFeature4"),
      ],
      accent: true,
    },
    {
      tier: t("auth.planTeamsTier"),
      price: t("auth.planTeamsPrice"),
      audience: t("auth.planTeamsAudience"),
      features: [
        t("auth.planTeamsFeature1"),
        t("auth.planTeamsFeature2"),
        t("auth.planTeamsFeature3"),
        t("auth.planTeamsFeature4"),
      ],
    },
    {
      tier: t("auth.planEnterpriseTier"),
      price: t("auth.planEnterprisePrice"),
      audience: t("auth.planEnterpriseAudience"),
      features: [
        t("auth.planEnterpriseFeature1"),
        t("auth.planEnterpriseFeature2"),
        t("auth.planEnterpriseFeature3"),
        t("auth.planEnterpriseFeature4"),
      ],
    },
  ];

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);

    // Zod validation
    const schema = mode === "register" ? registerFormSchema : loginFormSchema;
    const data = mode === "register"
      ? { email, password, displayName }
      : { email, password };
    const result = validateForm(schema, data);
    if (!result.success) {
      const firstError = Object.values(result.errors)[0];
      setError(firstError || t("auth.validationFailed"));
      setLoading(false);
      return;
    }

    try {
      if (mode === "register") {
        await cmd.register(email, password, displayName || email.split("@")[0]);
      } else {
        await cmd.login(email, password);
      }
      const authState = await cmd.getAuthState();
      setAuth(authState);
      navigate("/");
    } catch (err: unknown) {
      setError(extractErrorMessage(err, t("auth.operationFailed")));
    } finally {
      setLoading(false);
    }
  };

  const handleOAuthLogin = async (provider: string) => {
    setError("");
    setOauthLoading(provider);
    try {
      await cmd.startOAuthLogin(provider);
      const authState = await cmd.getAuthState();
      setAuth(authState);
      navigate("/");
    } catch (err: unknown) {
      setError(extractErrorMessage(err, t("auth.operationFailed")));
    } finally {
      setOauthLoading(null);
    }
  };

  const switchMode = () => {
    setMode(mode === "login" ? "register" : "login");
    setError("");
  };

  return (
    <div className="auth-page-wide">
      <div className="auth-layout">
        <div className="auth-form-panel">
          <div className="auth-card">
            <div className="auth-brand">
              <div className="auth-brand-mark">S</div>
              <div>
                <div className="auth-brand-title">SeasAGI</div>
                <div className="auth-brand-subtitle">{t("auth.brandSubtitle")}</div>
              </div>
            </div>

            <div className="auth-tabs">
              <button
                className={`auth-tab ${mode === "register" ? "active" : ""}`}
                onClick={() => mode !== "register" && switchMode()}
              >
                {t("auth.register")}
              </button>
              <button
                className={`auth-tab ${mode === "login" ? "active" : ""}`}
                onClick={() => mode !== "login" && switchMode()}
              >
                {t("auth.login")}
              </button>
            </div>

            {mode === "register" ? (
              <p className="auth-desc">{t("auth.registerDesc")}</p>
            ) : (
              <p className="auth-desc">{t("auth.loginDesc")}</p>
            )}

            {error && <div className="error-msg">{error}</div>}

            <form onSubmit={handleSubmit}>
              {mode === "register" && (
                <div className="form-group">
                  <input
                    type="text"
                    placeholder={t("auth.displayNamePlaceholder")}
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                  />
                </div>
              )}
              <div className="form-group">
                <input
                  type="email"
                  placeholder={t("settings.emailPlaceholder")}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </div>
              <div className="form-group">
                <input
                  type="password"
                  placeholder={t("auth.passwordPlaceholder")}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  minLength={6}
                  required
                />
              </div>
              <button type="submit" className="btn-primary btn-full" disabled={loading}>
                {loading
                  ? (mode === "register" ? t("auth.registering") : t("auth.loggingIn"))
                  : (mode === "register" ? t("auth.register") : t("auth.login"))}
              </button>
            </form>

            {oauthProviders.length > 0 && (
              <>
                <div className="auth-divider"><span>{t("auth.oauthDivider")}</span></div>
                <div className="auth-oauth-buttons">
                  {oauthProviders.map((provider) => {
                    const label = provider.name.charAt(0).toUpperCase() + provider.name.slice(1);
                    return (
                      <button
                        key={provider.name}
                        type="button"
                        className="auth-oauth-btn"
                        disabled={loading || oauthLoading !== null}
                        onClick={() => handleOAuthLogin(provider.name)}
                      >
                        {oauthProviderIcon(provider.name)}
                        <span>
                          {oauthLoading === provider.name
                            ? t("auth.oauthInProgress")
                            : t("auth.oauthContinueWith", { provider: label })}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </>
            )}

            <div className="auth-switch">
              {mode === "register" ? (
                <>{t("auth.hasAccount")}<button type="button" className="link-btn" onClick={switchMode}>{t("auth.goLogin")}</button></>
              ) : (
                <>{t("auth.noAccount")}<button type="button" className="link-btn" onClick={switchMode}>{t("auth.goRegister")}</button></>
              )}
            </div>

            <div className="auth-skip">
              <div className="auth-divider"><span>{t("auth.enterLocalMode")}</span></div>
              <p className="auth-local-desc">{t("auth.localModeDesc")}</p>
              <button type="button" className="btn btn-primary" onClick={() => navigate("/")}>
                {t("auth.enterLocalMode")}
              </button>
            </div>
          </div>
        </div>

        <div className="auth-benefits-panel">
          <div className="auth-benefits-header">
            <h2>{t("auth.cloudBenefitsTitle")}</h2>
            <p>{t("auth.cloudBenefitsDesc")}</p>
          </div>
          <div className="auth-benefits-grid">
            {planBenefits.map((plan) => (
              <div key={plan.tier} className={`auth-benefit-card ${plan.accent ? "auth-benefit-accent" : ""}`}>
                <div className="auth-benefit-head">
                  <span className="auth-benefit-tier">{plan.tier}</span>
                  <span className="auth-benefit-price">{plan.price}</span>
                </div>
                <div className="auth-benefit-audience">{plan.audience}</div>
                <ul className="auth-benefit-features">
                  {plan.features.map((f) => (
                    <li key={f}>{f}</li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
