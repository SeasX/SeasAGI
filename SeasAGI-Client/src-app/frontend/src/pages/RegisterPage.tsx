import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import { useTranslation } from "../i18n";
import { validateForm, loginFormSchema, registerFormSchema } from "../utils/validation";

type AuthMode = "login" | "register";

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
      const message =
        err instanceof Error ? err.message :
        typeof err === "string" ? err :
        err && typeof err === "object" && "message" in err ? String((err as {message: string}).message) :
        err && typeof err === "object" && "error" in err ? String((err as {error: string}).error) :
        t("auth.operationFailed");
      setError(message);
    } finally {
      setLoading(false);
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
