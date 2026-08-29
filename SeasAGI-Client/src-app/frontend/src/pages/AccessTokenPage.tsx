import { useEffect, useState } from "react";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import { useTranslation } from "../i18n";
import { getErrorMessage } from "../utils/errors";

export function AccessTokenPage() {
  const { t } = useTranslation();
  const runtime = useAppStore((s) => s.runtime);
  const [token, setToken] = useState("");
  const [loading, setLoading] = useState(true);
  const [resetting, setResetting] = useState(false);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");
  const baseUrl = `http://127.0.0.1:${runtime?.listen_port || 4318}/v1`;

  const loadToken = async () => {
    setLoading(true);
    setError("");
    try {
      setToken(await cmd.getLocalAccessToken());
    } catch (err) {
      setError(getErrorMessage(err, t("token.loadFailed")));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadToken();
  }, []);

  const handleReset = async () => {
    if (resetting) return;
    setResetting(true);
    setError("");
    try {
      const nextToken = await cmd.resetLocalAccessToken();
      setToken(nextToken);
      setCopied(false);
    } catch (err) {
      setError(getErrorMessage(err, t("token.resetFailed")));
    } finally {
      setResetting(false);
    }
  };

  const handleCopy = async () => {
    if (!token) return;
    await navigator.clipboard.writeText(token);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  if (loading) {
    return <div className="loading">{t("token.loading")}</div>;
  }

  return (
    <div className="page token-page">
      <div className="page-header page-hero">
        <div>
          <h1>{t("token.title")}</h1>
          <p className="page-subtitle">{t("token.subtitle")}</p>
        </div>
        <div className="hero-metrics">
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("token.gatewayAddress")}</span>
            <strong className="hero-metric-value token-metric-value">{runtime?.listen_port || 4318}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("token.tokenStatus")}</span>
            <strong className="hero-metric-value">{token ? t("token.generated") : t("token.notGenerated")}</strong>
          </div>
        </div>
      </div>

      {error && <div className="error-msg">{error}</div>}

      <div className="tab-content section-card">
        <div className="section-heading">
          <h2>{t("token.currentToken")}</h2>
          <p className="hint">{t("token.localOnlyHint")}</p>
        </div>

        <div className="token-card">
          <code className="token-value">{token || "-"}</code>
          <div className="token-actions">
            <button onClick={handleCopy} className="btn-secondary" disabled={!token || resetting}>
              {copied ? t("token.copied") : t("token.copy")}
            </button>
            <button onClick={handleReset} className="btn-danger" disabled={resetting}>
              {t("token.reset")}
            </button>
          </div>
        </div>

        <div className="token-help">
          <div className="token-help-item token-help-card">
            <span className="token-help-label">{t("token.baseUrlLabel")}</span>
            <code>{baseUrl}</code>
          </div>
          <div className="token-help-item token-help-card">
            <span className="token-help-label">{t("token.storageLabel")}</span>
            <span>{t("token.storageValue")}</span>
          </div>
          <div className="token-help-item token-help-card">
            <span className="token-help-label">{t("token.usageLabel")}</span>
            <span>{t("token.usageValue")}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
