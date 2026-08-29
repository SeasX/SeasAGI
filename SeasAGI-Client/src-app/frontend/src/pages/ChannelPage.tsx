import { useState, useEffect } from "react";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import type { Channel } from "../utils/types";
import { useTranslation } from "../i18n";
import { validateForm, channelFormSchema } from "../utils/validation";

const modalityIcon = (m: string) => {
  switch (m) {
    case "chat": return "Chat";
    case "embeddings": return "Embedding";
    case "image": return "Image";
    case "tts": return "TTS";
    case "stt": return "STT";
    default: return "API";
  }
};

export function ChannelPage() {
  const auth = useAppStore((s) => s.auth);
  const channels = useAppStore((s) => s.channels);
  const setChannels = useAppStore((s) => s.setChannels);
  const { t } = useTranslation();

  const [editing, setEditing] = useState<Channel | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [expandedError, setExpandedError] = useState<string | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ channelId: string; success: boolean; message: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [syncingCustom, setSyncingCustom] = useState(false);

  const [showAPIKey, setShowAPIKey] = useState(false);
  const [modelCatalog, setModelCatalog] = useState<Record<string, any>[]>([]);

  // Fetch model catalog from enterprise server for auto-complete (7.2.4)
  useEffect(() => {
    if (!auth.is_logged_in) return;
    let cancelled = false;
    (async () => {
      try {
        const catalog = await cmd.fetchModelCatalog();
        if (!cancelled && catalog && !catalog.some((m) => m.error)) {
          setModelCatalog(catalog);
        }
      } catch {
        // silent fail - catalog not available
      }
    })();
    return () => { cancelled = true; };
  }, [auth.is_logged_in]);

  // Auto-sync platform channels from cloud on mount when logged in
  useEffect(() => {
    if (!auth.is_logged_in) return;
    let cancelled = false;
    (async () => {
      try {
        await cmd.syncPlatformChannels();
        if (!cancelled) {
          const refreshed = await cmd.listChannels();
          setChannels(refreshed);
        }
      } catch {
        // silent fail - channels already loaded from local config
      }
    })();
    return () => { cancelled = true; };
  }, [auth.is_logged_in]);

  const platformChannels = channels.filter((ch) => ch.channel_type === "platform");
  const customChannels = channels.filter((ch) => ch.channel_type !== "platform");

  const handleTest = async (ch: Channel) => {
    setTesting(ch.channel_id);
    setTestResult(null);
    try {
      const result = await cmd.testChannelDirect(ch.channel_id);
      if (result && result.success) {
        setTestResult({ channelId: ch.channel_id, success: true, message: result.message || "connected" });
      } else {
        setTestResult({ channelId: ch.channel_id, success: false, message: result?.error || "test failed" });
      }
    } catch (e) {
      setTestResult({ channelId: ch.channel_id, success: false, message: String(e) });
    }
    setTesting(null);
  };

  const handleSync = async () => {
    if (!auth.is_logged_in) return;
    setSyncing(true);
    try {
      await cmd.syncPlatformChannels();
      const refreshed = await cmd.listChannels();
      setChannels(refreshed);
    } catch (e) {
      console.error(e);
    }
    setSyncing(false);
  };

  const handleSyncCustomToCloud = async () => {
    if (!auth.is_logged_in) return;
    setSyncingCustom(true);
    try {
      const customChs = channels.filter((ch: Channel) => ch.channel_type !== "platform");
      const result = await cmd.syncCustomChannelsToCloud(customChs.map((ch: Channel) => ({
        channel_id: ch.channel_id,
        provider_type: ch.provider_type,
        display_name: ch.display_name,
        base_url: ch.base_url,
        models: JSON.stringify(ch.models || []),
        enabled: ch.enabled !== false,
      })));
      console.log("Custom channels synced to cloud:", result);
    } catch (e) {
      console.error("Failed to sync custom channels to cloud:", e);
    }
    setSyncingCustom(false);
  };

  const handleDelete = async (ch: Channel) => {
    try {
      await cmd.deleteCustomChannel(ch.channel_id);
      setChannels(channels.filter((c) => c.channel_id !== ch.channel_id));
    } catch (e) {
      console.error(e);
    }
  };

  const handleSaveEdit = async () => {
    if (!editing) return;
    const v = validateForm(channelFormSchema, editing);
    if (!v.success) {
      setValidationError(Object.values(v.errors).join("; "));
      return;
    }
    setSaving(true);
    setValidationError(null);
    try {
      if (isNew) {
        await cmd.saveCustomChannel(editing);
      } else {
        await cmd.saveCustomChannel(editing);
      }
      setEditing(null);
      const refreshed = await cmd.listChannels();
      setChannels(refreshed);
    } catch (e: any) {
      setValidationError(e?.message || String(e));
    }
    setSaving(false);
  };

  const handleCopyErrorText = (_ch: Channel, err: string) => {
    navigator.clipboard.writeText(err);
  };

  const handleCopyErrorJSON = (_ch: Channel, err: string) => {
    navigator.clipboard.writeText(JSON.stringify({ error: err, timestamp: new Date().toISOString() }, null, 2));
  };

  const handleCopyErrorMarkdown = (_ch: Channel, err: string) => {
    const md = `### ${t("channel.validationFailedMarkdown")}\n\n\`\`\`json\n${JSON.stringify({ error: err, timestamp: new Date().toISOString() }, null, 2)}\n\`\`\``;
    navigator.clipboard.writeText(md);
  };

  const renderChannelItem = (ch: Channel) => (
    <div key={ch.channel_id} className="channel-item channel-card">
      <div className="channel-card-main">
        <div className="channel-card-content">
          <div className="channel-header">
            <div className="channel-title-row">
              <span className="channel-name">{ch.display_name}</span>
              <span className={`channel-status ${ch.health_status === "healthy" ? "ok" : "off"}`}>
                {ch.health_status === "healthy" ? t("channel.enabled") : t("channel.disabled")}
              </span>
            </div>
            <div className="channel-id-chip">{ch.channel_id}</div>
          </div>
          <div className="channel-meta">
            <span className="channel-provider">{ch.provider_type}</span>
            <span className="channel-models">{t("home.modelCount", { count: ch.models?.length || 0 })}</span>
            {ch.supported_modalities && ch.supported_modalities.length > 0 && (
              <span className="channel-modalities">
                {ch.supported_modalities.map((m) => modalityIcon(m)).join(" ")}
              </span>
            )}
          </div>
          {ch.health_status !== "healthy" && (
            <div className="error-bar">
              <span>{t("channel.unavailable")}</span>
              <button onClick={() => setExpandedError(expandedError === ch.channel_id ? null : ch.channel_id)} className="btn-link">
                {expandedError === ch.channel_id ? t("channel.collapse") : t("channel.detail")}
              </button>
            </div>
          )}
          {expandedError === ch.channel_id && ch.last_error && (
            <div className="error-detail">
              <pre>{ch.last_error}</pre>
              <div className="error-actions">
                <button onClick={() => handleCopyErrorText(ch, ch.last_error!)} className="btn-link">{t("channel.copy")}</button>
                <button onClick={() => handleCopyErrorJSON(ch, ch.last_error!)} className="btn-link">JSON</button>
                <button onClick={() => handleCopyErrorMarkdown(ch, ch.last_error!)} className="btn-link">Markdown</button>
              </div>
            </div>
          )}
        </div>
        <div className="channel-card-side">
          <div className="channel-actions">
            <button onClick={() => handleTest(ch)} disabled={testing === ch.channel_id} className="btn-secondary">
              {testing === ch.channel_id ? t("channel.testing") : t("channel.testConnection")}
            </button>
            {ch.channel_type !== "platform" && (
              <>
                <button onClick={() => { setEditing(ch); setIsNew(false); }} className="btn-secondary">{t("channel.edit")}</button>
                <button onClick={() => handleDelete(ch)} className="btn-danger">{t("channel.delete")}</button>
              </>
            )}
          </div>
          {testResult && testResult.channelId === ch.channel_id && (
            <span className={`test-result ${testResult.success ? "test-ok" : "test-fail"}`}>
              {testResult.success ? "✓" : "✗"} {testResult.message}
            </span>
          )}
        </div>
      </div>
    </div>
  );

  return (
    <div className="page channel-page">
      <div className="page-hero">
        <div className="page-hero-head">
          <div className="page-hero-copy">
            <h1>{t("channel.title")}</h1>
            <div className="page-hero-content-row">
              <p className="page-subtitle">{t("channel.pageSubtitle")}</p>
              <div className="hero-metrics">
                <div className="hero-metric-card">
                  <span className="hero-metric-label">{t("channel.platformChannels")}</span>
                  <strong className="hero-metric-value">{platformChannels.length}</strong>
                </div>
                <div className="hero-metric-card">
                  <span className="hero-metric-label">{t("channel.customChannels")}</span>
                  <strong className="hero-metric-value">{customChannels.length}</strong>
                </div>
              </div>
            </div>
          </div>
          <div className="page-hero-actions">
            {auth.is_logged_in && (
              <button onClick={handleSync} disabled={syncing} className="btn-secondary">
                {syncing ? t("channel.syncing") : t("channel.syncPlatform")}
              </button>
            )}
            {auth.is_logged_in && (
              <button onClick={handleSyncCustomToCloud} disabled={syncingCustom} className="btn-secondary">
                {syncingCustom ? t("channel.syncing") : t("channel.syncCustomToCloud")}
              </button>
            )}
            <button
              onClick={() => {
                setEditing({ channel_id: "", channel_type: "custom", provider_type: "openai", display_name: "", base_url: "", api_key: "", enabled: true, models: [], health_status: "unknown", provider_specific_config: {} });
                setIsNew(true);
              }}
              className="btn-primary"
            >
              {t("channel.addChannel")}
            </button>
          </div>
        </div>
      </div>

      {platformChannels.length > 0 && (
        <div className="channel-group section-card">
          <div className="section-heading">
            <h2>{t("channel.platformChannels")}</h2>
            <p className="hint">{t("channel.platformHint")}</p>
          </div>
          {platformChannels.map(renderChannelItem)}
        </div>
      )}

      <div className="channel-group section-card">
        <div className="section-heading">
          <h2>{t("channel.customChannels")}</h2>
          <p className="hint">{t("channel.customHint")}</p>
        </div>
        {customChannels.length === 0 && <p className="empty-hint">{t("channel.noCustom")}</p>}
        {customChannels.map(renderChannelItem)}
      </div>

      {editing && (
        <div className="modal-overlay" onClick={() => setEditing(null)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>{isNew ? t("channel.addCustomChannel") : t("channel.editChannel")}</h2>
            
            <div className="form-group">
              <label>{t("channel.channelName")}</label>
              <input value={editing.display_name} onChange={(e) => setEditing({ ...editing, display_name: e.target.value })} placeholder={t("channel.channelNamePlaceholder")} />
            </div>

            <div className="form-group">
              <label>{t("channel.providerType")}</label>
              <select value={editing.provider_type} onChange={(e) => setEditing({ ...editing, provider_type: e.target.value })}>
                <option value="openai">OpenAI</option>
                <option value="azure_openai">Azure OpenAI</option>
                <option value="anthropic">Anthropic</option>
                <option value="gemini">Google Gemini</option>
                <option value="openrouter">OpenRouter</option>
                <option value="deepseek">DeepSeek</option>
                <option value="mistral">Mistral AI</option>
                <option value="qwen">Qwen (Alibaba Cloud)</option>
                <option value="bailian">{t("channel.providerBailian")}</option>
                <option value="minimax">MiniMax</option>
                <option value="moonshot">Moonshot AI (Kimi)</option>
                <option value="zhipu">Zhipu AI (GLM)</option>
                <option value="xiaomi">Xiaomi MiMo</option>
                <option value="custom">Custom</option>
              </select>
            </div>

            <div className="form-group">
              <label>{t("channel.baseUrl")}</label>
              <input value={editing.base_url} onChange={(e) => setEditing({ ...editing, base_url: e.target.value })} />
            </div>

            <div className="form-group">
              <label>{t("channel.apiKey")}</label>
              <div className="api-key-row">
                <input type={showAPIKey ? "text" : "password"} value={editing.api_key || ""} onChange={(e) => setEditing({ ...editing, api_key: e.target.value })} placeholder={t("channel.apiKeyPlaceholder")} />
                <button type="button" className="btn-text" onClick={() => setShowAPIKey(!showAPIKey)} style={{ fontSize: 13, padding: "6px 10px", whiteSpace: "nowrap" }}>{showAPIKey ? t("channel.hide") : t("channel.show")}</button>
              </div>
            </div>

            <div className="form-group">
              <label>{t("channel.apiKeys")}</label>
              <textarea
                value={(editing.api_keys || []).join("\n")}
                onChange={(e) => setEditing({ ...editing, api_keys: e.target.value.split("\n") })}
                placeholder={t("channel.apiKeysPlaceholder")}
                rows={4}
              />
            </div>

            <div className="form-group">
              <label>{t("channel.models")}</label>
              <textarea
                value={(editing.models || []).join("\n")}
                onChange={(e) => setEditing({ ...editing, models: e.target.value.split("\n").map(s => s.trim()).filter(Boolean) })}
                placeholder={t("channel.modelsPlaceholder")}
                rows={5}
              />
              <p className="hint" style={{ marginTop: 4, fontSize: 12 }}>
                {t("channel.modelsHint")}
              </p>
              {modelCatalog.length > 0 && editing.provider_type && editing.provider_type !== "custom" && (() => {
                const providerModels = modelCatalog.filter((m) => {
                  const mp = (m.provider || m.category || "").toLowerCase();
                  const pt = editing.provider_type.toLowerCase();
                  return mp === pt || (pt === "qwen" && (mp === "alibaba" || mp === "qwen")) ||
                         (pt === "bailian" && (mp === "alibaba" || mp === "bailian")) ||
                         (pt === "gemini" && mp === "google") ||
                         (pt === "azure_openai" && mp === "openai");
                });
                if (providerModels.length === 0) return null;
                const currentModels = new Set(editing.models || []);
                const unselected = providerModels.filter((m) => !currentModels.has(m.model_id));
                return (
                  <div style={{ marginTop: 6, padding: "8px 10px", background: "var(--bg-surface, #f5f5f5)", borderRadius: 6, fontSize: 12 }}>
                    <div style={{ fontWeight: 600, marginBottom: 4 }}>
                      {t("channel.catalogSuggestion") || "Catalog models for this provider:"} ({providerModels.length})
                    </div>
                    <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
                      {providerModels.slice(0, 12).map((m) => {
                        const selected = currentModels.has(m.model_id);
                        return (
                          <button
                            key={m.model_id}
                            type="button"
                            onClick={() => {
                              const models = editing.models || [];
                              if (selected) {
                                setEditing({ ...editing, models: models.filter((id) => id !== m.model_id) });
                              } else {
                                setEditing({ ...editing, models: [...models, m.model_id] });
                              }
                            }}
                            style={{
                              padding: "2px 8px", borderRadius: 4, fontSize: 11, cursor: "pointer",
                              border: selected ? "1px solid var(--accent, #22C55E)" : "1px solid var(--border-subtle, #ccc)",
                              background: selected ? "rgba(34,197,94,0.1)" : "transparent",
                              color: selected ? "var(--accent, #22C55E)" : "inherit",
                            }}
                          >
                            {m.display_name || m.model_id}
                          </button>
                        );
                      })}
                    </div>
                    {unselected.length > 0 && (
                      <button
                        type="button"
                        onClick={() => {
                          const models = editing.models || [];
                          const newModels = [...new Set([...models, ...unselected.map((m) => m.model_id)])];
                          setEditing({ ...editing, models: newModels });
                        }}
                        style={{ marginTop: 6, fontSize: 11, padding: "3px 10px", cursor: "pointer" }}
                        className="btn-text"
                      >
                        + {t("channel.addAllCatalog") || "Add all catalog models"} ({unselected.length})
                      </button>
                    )}
                  </div>
                );
              })()}
            </div>

            <div className="form-group">
              <label>{t("channel.modelsApi")}</label>
              <div className="api-key-row">
                <input
                  value={editing.provider_specific_config?.models_api_url || ""}
                  onChange={(e) => setEditing({ ...editing, provider_specific_config: { ...editing.provider_specific_config, models_api_url: e.target.value } })}
                  placeholder={"https://api.openai.com/v1/models"}
                />
                <button
                  type="button"
                  className="btn-secondary btn-sm"
                  onClick={async () => {
                    const modelsUrl = editing.provider_specific_config?.models_api_url || "";
                    const baseURL = editing.base_url || "";
                    const apiKey = editing.api_key || (editing.api_keys && editing.api_keys[0]) || "";
                    if (!modelsUrl && !baseURL) return;
                    try {
                      const result = await cmd.discoverModelsFromURL(
                        modelsUrl || baseURL + "/models",
                        apiKey
                      );
                      if (result.models && result.models.length > 0) {
                        setEditing({ ...editing, models: result.models });
                      }
                    } catch (e) {
                      console.error("Failed to fetch models:", e);
                    }
                  }}
                  style={{ whiteSpace: "nowrap" }}
                >
                  {t("channel.fetchModels")}
                </button>
              </div>
            </div>

            <div className="form-group">
              <label>{t("channel.maxRetries")}</label>
              <input type="number" min={0} max={10} value={editing.retry_config?.max_retries ?? 3} onChange={(e) => setEditing({ ...editing, retry_config: { ...editing.retry_config, max_retries: parseInt(e.target.value) || 3, initial_backoff_ms: editing.retry_config?.initial_backoff_ms ?? 1000, max_backoff_ms: editing.retry_config?.max_backoff_ms ?? 30000, jitter: editing.retry_config?.jitter ?? true } })} />
            </div>

            {editing.provider_type === "azure_openai" && (
              <div className="form-row">
                <div className="form-group">
                  <label>deployment_name</label>
                  <input value={editing.provider_specific_config?.deployment_name || ""} onChange={(e) => setEditing({ ...editing, provider_specific_config: { ...editing.provider_specific_config, deployment_name: e.target.value } })} />
                </div>
                <div className="form-group">
                  <label>api_version</label>
                  <input value={editing.provider_specific_config?.api_version || ""} onChange={(e) => setEditing({ ...editing, provider_specific_config: { ...editing.provider_specific_config, api_version: e.target.value } })} />
                </div>
              </div>
            )}

            {editing.provider_type === "anthropic" && (
              <div className="form-group">
                <label>anthropic_version</label>
                <input value={editing.provider_specific_config?.anthropic_version || ""} onChange={(e) => setEditing({ ...editing, provider_specific_config: { ...editing.provider_specific_config, anthropic_version: e.target.value } })} />
              </div>
            )}

            <div className="modal-actions">
              <button onClick={() => setEditing(null)} className="btn-secondary">{t("channel.cancel")}</button>
              <button onClick={handleSaveEdit} disabled={saving} className="btn-primary">
                {saving ? (isNew ? t("channel.addingValidating") : t("channel.savingValidating")) : (isNew ? t("channel.addAndValidate") : t("channel.saveAndValidate"))}
              </button>
            </div>
            {validationError && (
              <div className="error-detail">
                <pre>{validationError}</pre>
                <div className="error-actions">
                  <button onClick={() => handleCopyErrorText(editing, validationError)} className="btn-link">{t("channel.copy")}</button>
                  <button onClick={() => handleCopyErrorJSON(editing, validationError)} className="btn-link">JSON</button>
                  <button onClick={() => handleCopyErrorMarkdown(editing, validationError)} className="btn-link">Markdown</button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
