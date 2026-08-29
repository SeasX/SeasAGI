import { useState, useRef, useEffect } from "react";
import { useTranslation } from "../i18n";
import {
  chatCompletion, chatCompletionForChannel,
  imageGeneration, imageGenerationForChannel,
  videoGeneration, videoGenerationForChannel,
  listChannels, getLocalAccessToken, listModelCombos,
} from "../utils/commands";
import type { Channel, ModelCombo } from "../utils/types";

type PlaygroundMode = "text" | "image" | "video";

interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  model?: string;
  latency?: number;
  channel?: string;
  curlCommand?: string;
  comboSteps?: { step: number; role: string; model: string; status: "ok" | "fallback" | "error"; latency_ms?: number }[];
}

interface ImageResult {
  url?: string;
  b64_json?: string;
  revised_prompt?: string;
}

interface VideoResult {
  url?: string;
  status?: string;
}

export function PlaygroundPage() {
  const { t } = useTranslation();

  const [mode, setMode] = useState<PlaygroundMode>("text");

  // Shared state
  const [channels, setChannels] = useState<Channel[]>([]);
  const [combos, setCombos] = useState<ModelCombo[]>([]);
  const [gatewayRunning, setGatewayRunning] = useState(false);
  const [selectedChannel, setSelectedChannel] = useState("auto");
  const [selectedModel, setSelectedModel] = useState("auto");
  const [selectedCombo, setSelectedCombo] = useState("");

  // Text mode state
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [showCurl, setShowCurl] = useState<string | null>(null);
  const [showComboTrace, setShowComboTrace] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // Image mode state
  const [imagePrompt, setImagePrompt] = useState("");
  const [imageSize, setImageSize] = useState("1024x1024");
  const [imageCount, setImageCount] = useState(1);
  const [imageResults, setImageResults] = useState<ImageResult[]>([]);
  const [imageLoading, setImageLoading] = useState(false);
  const [imageError, setImageError] = useState("");
  const imagePromptRef = useRef<HTMLTextAreaElement>(null);

  // Video mode state
  const [videoPrompt, setVideoPrompt] = useState("");
  const [videoResults, setVideoResults] = useState<VideoResult[]>([]);
  const [videoLoading, setVideoLoading] = useState(false);
  const [videoError, setVideoError] = useState("");
  const videoPromptRef = useRef<HTMLTextAreaElement>(null);

  const allModels: string[] = [];
  for (const ch of channels) {
    if (ch.enabled && ch.models) {
      for (const m of ch.models) allModels.push(m);
    }
  }
  const modelSet = Array.from(new Set(allModels)).sort();

  useEffect(() => {
    (async () => {
      try {
        const token = await getLocalAccessToken();
        setGatewayRunning(!!token);
      } catch {}
    })();
    (async () => {
      try {
        const channelList = await listChannels();
        setChannels(channelList);
      } catch {}
    })();
    (async () => {
      try {
        const comboList = await listModelCombos();
        setCombos(comboList);
      } catch {}
    })();
  }, []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const availableModels = selectedChannel === "auto"
    ? modelSet
    : (() => {
        const ch = channels.find(c => c.channel_id === selectedChannel);
        return ch?.models?.sort() || [];
      })();

  // ── Text mode handlers ──
  const handleSend = async () => {
    const text = input.trim();
    if (!text || loading) return;

    const userMsg: ChatMessage = { role: "user", content: text };
    const newMessages = [...messages, userMsg];
    setMessages(newMessages);
    setInput("");
    setLoading(true);

    try {
      const payload = newMessages.map((m) => ({ role: m.role, content: m.content }));
      const useCombo = selectedCombo !== "" ? combos.find(c => c.name === selectedCombo) : null;
      const model = useCombo ? (useCombo.steps?.[0]?.model || "gpt-4o") : (selectedModel === "auto" ? "gpt-4o" : selectedModel);
      const useChannel = selectedChannel !== "auto" ? selectedChannel : null;
      const result = useCombo
        ? await chatCompletion(payload, useCombo.name)
        : useChannel
          ? await chatCompletionForChannel(useChannel, payload, model)
          : await chatCompletion(payload, model);

      const curlCommand = result._curl_command as string | undefined;
      const channelLabel = useChannel ? channels.find(c => c.channel_id === useChannel)?.display_name || useChannel : "";
      const gatewayStatus = result._gateway_status || result._status_code;
      const isError = gatewayStatus >= 400;
      const steps = result._combo_steps
        ? (result._combo_steps as any[]).map((s: any, idx: number) => ({
            step: idx + 1,
            role: s.step_role || s.role || "",
            model: s.model || "",
            status: (s.status === "success" ? "ok" : s.status === "fallback" ? "fallback" : "error") as "ok" | "fallback" | "error",
            latency_ms: s.latency_ms,
          }))
        : undefined;

      if (isError) {
        const errMsg = result.error?.message || `HTTP ${gatewayStatus}`;
        setMessages([...newMessages, { role: "assistant", content: `Error: ${errMsg}`, curlCommand, comboSteps: steps }]);
      } else {
        const content = result.choices?.[0]?.message?.content ?? JSON.stringify(result, null, 2);
        setMessages([...newMessages, { role: "assistant", content, model: result.model || model, channel: channelLabel || undefined, curlCommand, comboSteps: steps }]);
      }
    } catch (err: any) {
      setMessages([...newMessages, { role: "assistant", content: `Error: ${err.message}` }]);
    } finally {
      setLoading(false);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleSend(); }
  };

  // ── Image mode handlers ──
  const handleImageGenerate = async () => {
    const prompt = imagePrompt.trim();
    if (!prompt || imageLoading) return;
    setImageLoading(true);
    setImageError("");
    try {
      const params: Record<string, any> = { size: imageSize, n: imageCount };
      const useChannel = selectedChannel !== "auto" ? selectedChannel : null;
      const model = selectedModel === "auto" ? "dall-e-3" : selectedModel;
      const result = useChannel
        ? await imageGenerationForChannel(useChannel, prompt, model, params)
        : await imageGeneration(prompt, model, params);

      const status = result._gateway_status || result._status_code;
      if (status >= 400) {
        setImageError(result.error?.message || `HTTP ${status}`);
      } else if (result.data && Array.isArray(result.data)) {
        const imgs: ImageResult[] = result.data.map((d: any) => ({ url: d.url, b64_json: d.b64_json, revised_prompt: d.revised_prompt }));
        setImageResults(prev => [...imgs, ...prev]);
      } else {
        setImageError(t("playground.image.noResults"));
      }
    } catch (err: any) {
      setImageError(err.message);
    } finally {
      setImageLoading(false);
    }
  };

  // ── Video mode handlers ──
  const handleVideoGenerate = async () => {
    const prompt = videoPrompt.trim();
    if (!prompt || videoLoading) return;
    setVideoLoading(true);
    setVideoError("");
    try {
      const useChannel = selectedChannel !== "auto" ? selectedChannel : null;
      const model = selectedModel === "auto" ? "sora" : selectedModel;
      const result = useChannel
        ? await videoGenerationForChannel(useChannel, prompt, model, {})
        : await videoGeneration(prompt, model, {});

      const status = result._gateway_status || result._status_code;
      if (status >= 400) {
        setVideoError(result.error?.message || `HTTP ${status}`);
      } else if (result.data && Array.isArray(result.data)) {
        const vids: VideoResult[] = result.data.map((d: any) => ({ url: d.url, status: d.status }));
        setVideoResults(prev => [...vids, ...prev]);
      } else if (result.url) {
        setVideoResults(prev => [{ url: result.url, status: result.status }, ...prev]);
      } else {
        setVideoError(t("playground.video.noResults"));
      }
    } catch (err: any) {
      setVideoError(err.message);
    } finally {
      setVideoLoading(false);
    }
  };

  const modeTabs: { key: PlaygroundMode; label: string; icon: string }[] = [
    { key: "text", label: t("playground.mode.text"), icon: "💬" },
    { key: "image", label: t("playground.mode.image"), icon: "🖼" },
    { key: "video", label: t("playground.mode.video"), icon: "🎬" },
  ];

  return (
    <div className="page playground-page">
      {/* Header */}
      <div className="page-header-row page-hero">
        <div>
          <h1>{t("nav.playground")}</h1>
          <p className="page-subtitle">{t("playground.description")}</p>
        </div>
        <div className="hero-metrics">
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("playground.currentModel")}</span>
            <strong className="hero-metric-value">{selectedModel === "auto" ? t("playground.auto") : selectedModel}</strong>
          </div>
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("playground.channel")}</span>
            <strong className="hero-metric-value">
              {selectedChannel === "auto" ? t("playground.auto") : (channels.find(c => c.channel_id === selectedChannel)?.display_name || selectedChannel)}
            </strong>
          </div>
        </div>
      </div>

      {!gatewayRunning && (
        <div className="alert alert-warning playground-warning">{t("playground.gatewayNotRunning")}</div>
      )}

      {/* Mode tabs */}
      <div className="playground-mode-tabs">
        {modeTabs.map(tab => (
          <button
            key={tab.key}
            className={`playground-mode-tab ${mode === tab.key ? "active" : ""}`}
            onClick={() => setMode(tab.key)}
          >
            <span className="playground-mode-icon">{tab.icon}</span>
            <span>{tab.label}</span>
          </button>
        ))}
      </div>

      {/* Shared toolbar */}
      <div className="playground-toolbar section-card">
        <div className="playground-toolbar-copy">
          <h2 style={{ fontSize: "16px", marginBottom: "4px" }}>{t("playground.testConfigTitle")}</h2>
          <p className="hint" style={{ fontSize: "12px" }}>{t("playground.testConfigHint")}</p>
        </div>
        <div className="playground-toolbar-actions">
          {mode === "text" && (
            <select
              value={selectedCombo}
              onChange={(e) => {
                setSelectedCombo(e.target.value);
                if (e.target.value) { setSelectedChannel("auto"); setSelectedModel("auto"); }
              }}
              className="select-input"
              style={{ minWidth: 160, height: "32px", fontSize: "13px" }}
            >
              <option value="">{t("playground.noCombo")}</option>
              {combos.map((c) => (
                <option key={c.name} value={c.name}>{t("playground.comboWithSteps", { name: c.name, count: c.steps?.length || 0 })}</option>
              ))}
            </select>
          )}
          <select
            value={selectedChannel}
            onChange={(e) => { setSelectedChannel(e.target.value); setSelectedModel("auto"); setSelectedCombo(""); }}
            className="select-input"
            style={{ minWidth: 160, height: "32px", fontSize: "13px" }}
            disabled={!!(mode === "text" && selectedCombo)}
          >
            <option value="auto">{t("playground.autoGatewayRouting")}</option>
            {channels.filter(c => c.enabled).map((ch) => (
              <option key={ch.channel_id} value={ch.channel_id}>{ch.display_name || ch.channel_id}</option>
            ))}
          </select>
          <select
            value={selectedModel}
            onChange={(e) => setSelectedModel(e.target.value)}
            className="select-input"
            style={{ height: "32px", fontSize: "13px" }}
            disabled={!!(mode === "text" && selectedCombo)}
          >
            <option value="auto">{t("playground.autoDefaultModel")}</option>
            {availableModels.map((m) => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
          {mode === "text" && messages.length > 0 && (
            <button className="btn-outline btn-sm" onClick={() => setMessages([])}>{t("playground.clear")}</button>
          )}
        </div>
      </div>

      {/* ── Text mode ── */}
      {mode === "text" && (
        <div className="playground-chat-container">
          <div className="playground-messages">
            {messages.length === 0 ? (
              <div className="empty-state playground-empty-state">
                <p className="playground-empty-title">{t("playground.emptyTitle")}</p>
                <p className="text-dim">{t("playground.emptyHint", { model: selectedModel === "auto" ? t("playground.defaultModel") : selectedModel })}</p>
              </div>
            ) : (
              <>
                {messages.map((msg, i) => (
                  <div key={i} className={`playground-message ${msg.role === "user" ? "message-user" : "message-assistant"}`}>
                    <div className="message-avatar">
                      {msg.role === "user" ? "U" : "AI"}
                    </div>
                    <div className="message-content-wrapper">
                      <div className="message-header">
                        <span className="message-author">{msg.role === "user" ? "User" : "SeasAGI"}</span>
                        <span className="message-time">{new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                      </div>
                      <div className={`message-bubble ${msg.role === "user" ? "bubble-user" : "bubble-assistant"}`}>
                        <div className="message-content">{msg.content}</div>
                        {msg.role === "assistant" && (msg.model || msg.channel || msg.curlCommand || msg.comboSteps) && (
                          <div className="message-meta">
                            {msg.channel && <span className="meta-item">{msg.channel}</span>}
                            {msg.model && <span className="meta-item">{msg.model}</span>}
                            {msg.comboSteps && (
                              <button className="btn-link btn-curl-toggle" onClick={() => setShowComboTrace(showComboTrace === `${i}` ? null : `${i}`)}>
                                {showComboTrace === `${i}` ? t("playground.hideComboTrace") : t("playground.showComboTrace")}
                              </button>
                            )}
                            {msg.curlCommand && (
                              <button className="btn-link btn-curl-toggle" onClick={() => setShowCurl(showCurl === `${i}` ? null : `${i}`)}>
                                {showCurl === `${i}` ? t("playground.hideCurl") : t("playground.showCurl")}
                              </button>
                            )}
                          </div>
                        )}
                        {msg.role === "assistant" && msg.comboSteps && showComboTrace === `${i}` && (
                          <div className="combo-trace-block">
                            <div className="combo-trace-title">{t("playground.comboTraceTitle")}</div>
                            {msg.comboSteps.map((step, si) => (
                              <div key={si} className={`combo-trace-step combo-trace-${step.status}`}>
                                <span className="combo-trace-step-num">#{step.step}</span>
                                <span className="combo-trace-step-role">{step.role}</span>
                                <span className="combo-trace-step-model">{step.model}</span>
                                <span className="combo-trace-step-status">
                                  {step.status === "ok" ? t("playground.traceOk") : step.status === "fallback" ? t("playground.traceFallback") : t("playground.traceFail")}
                                </span>
                                {step.latency_ms != null && <span className="combo-trace-step-latency">{step.latency_ms}ms</span>}
                              </div>
                            ))}
                          </div>
                        )}
                        {msg.role === "assistant" && msg.curlCommand && showCurl === `${i}` && (
                          <pre className="curl-command-block"><code>{msg.curlCommand}</code></pre>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
                {loading && (
                  <div className="playground-message message-assistant">
                    <div className="message-avatar">AI</div>
                    <div className="message-content-wrapper">
                      <div className="message-header">
                        <span className="message-author">SeasAGI</span>
                        <span className="message-time">{t("playground.sending")}</span>
                      </div>
                      <div className="loading-dots"><span className="dot" /><span className="dot" /><span className="dot" /></div>
                    </div>
                  </div>
                )}
                <div ref={messagesEndRef} />
              </>
            )}
          </div>
          <div className="playground-input-bar">
            <div className="input-wrapper" style={{ flex: 1, position: "relative", display: "flex", alignItems: "center" }}>
              <button className="input-plus-btn" style={{ position: "absolute", left: "12px", background: "none", border: "none", color: "var(--text-muted)" }}>
                <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="16" /><line x1="8" y1="12" x2="16" y2="12" /></svg>
              </button>
              <textarea
                ref={inputRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder={t("playground.inputPlaceholder") || "给 #发现 发送消息"}
                rows={1}
                className="playground-textarea"
                style={{ paddingLeft: "44px", borderRadius: "8px", background: "#383a40", border: "none" }}
                disabled={loading}
                onInput={(e) => { const el = e.target as HTMLTextAreaElement; el.style.height = "auto"; el.style.height = Math.min(el.scrollHeight, 160) + "px"; }}
              />
            </div>
          </div>
        </div>
      )}

      {/* ── Image mode ── */}
      {mode === "image" && (
        <div className="playground-gen-container section-card">
          <div className="playground-gen-input-area">
            <textarea
              ref={imagePromptRef}
              value={imagePrompt}
              onChange={(e) => setImagePrompt(e.target.value)}
              placeholder={t("playground.image.promptPlaceholder")}
              rows={3}
              className="playground-textarea"
              disabled={imageLoading}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleImageGenerate(); } }}
              onInput={(e) => { const el = e.target as HTMLTextAreaElement; el.style.height = "auto"; el.style.height = Math.min(el.scrollHeight, 200) + "px"; }}
            />
            <div className="playground-gen-controls">
              <div className="playground-gen-param">
                <label className="playground-param-label">{t("playground.image.size")}</label>
                <select value={imageSize} onChange={(e) => setImageSize(e.target.value)} className="select-input" disabled={imageLoading}>
                  <option value="1024x1024">1024×1024</option>
                  <option value="1024x1792">1024×1792</option>
                  <option value="1792x1024">1792×1024</option>
                  <option value="512x512">512×512</option>
                  <option value="256x256">256×256</option>
                </select>
              </div>
              <div className="playground-gen-param">
                <label className="playground-param-label">{t("playground.image.count")}</label>
                <select value={imageCount} onChange={(e) => setImageCount(Number(e.target.value))} className="select-input" disabled={imageLoading}>
                  <option value={1}>1</option>
                  <option value={2}>2</option>
                  <option value={3}>3</option>
                  <option value={4}>4</option>
                </select>
              </div>
              <button className="btn-primary" onClick={handleImageGenerate} disabled={imageLoading || !imagePrompt.trim()}>
                {imageLoading ? t("playground.image.generating") : t("playground.image.generate")}
              </button>
            </div>
          </div>

          {imageError && <div className="error-msg">{imageError}</div>}

          <div className="playground-gen-results">
            {imageResults.length === 0 && !imageLoading ? (
              <div className="empty-state playground-empty-state">
                <p className="playground-empty-title">{t("playground.image.empty")}</p>
              </div>
            ) : (
              <div className="playground-image-grid">
                {imageLoading && (
                  <div className="playground-image-card playground-image-loading">
                    <div className="loading-dots"><span className="dot" /><span className="dot" /><span className="dot" /></div>
                  </div>
                )}
                {imageResults.map((img, i) => (
                  <div key={i} className="playground-image-card">
                    {img.url ? (
                      <img src={img.url} alt={img.revised_prompt || `result-${i}`} />
                    ) : img.b64_json ? (
                      <img src={`data:image/png;base64,${img.b64_json}`} alt={img.revised_prompt || `result-${i}`} />
                    ) : null}
                    {img.revised_prompt && <p className="playground-image-caption">{img.revised_prompt}</p>}
                    {img.url && (
                      <a href={img.url} target="_blank" rel="noopener noreferrer" className="btn-link playground-image-download">
                        {t("playground.image.download")}
                      </a>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── Video mode ── */}
      {mode === "video" && (
        <div className="playground-gen-container section-card">
          <div className="playground-gen-input-area">
            <textarea
              ref={videoPromptRef}
              value={videoPrompt}
              onChange={(e) => setVideoPrompt(e.target.value)}
              placeholder={t("playground.video.promptPlaceholder")}
              rows={3}
              className="playground-textarea"
              disabled={videoLoading}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleVideoGenerate(); } }}
              onInput={(e) => { const el = e.target as HTMLTextAreaElement; el.style.height = "auto"; el.style.height = Math.min(el.scrollHeight, 200) + "px"; }}
            />
            <div className="playground-gen-controls">
              <button className="btn-primary" onClick={handleVideoGenerate} disabled={videoLoading || !videoPrompt.trim()}>
                {videoLoading ? t("playground.video.generating") : t("playground.video.generate")}
              </button>
            </div>
          </div>

          {videoError && <div className="error-msg">{videoError}</div>}

          <div className="playground-gen-results">
            {videoResults.length === 0 && !videoLoading ? (
              <div className="empty-state playground-empty-state">
                <p className="playground-empty-title">{t("playground.video.empty")}</p>
              </div>
            ) : (
              <div className="playground-video-list">
                {videoLoading && (
                  <div className="playground-video-card playground-video-loading">
                    <div className="loading-dots"><span className="dot" /><span className="dot" /><span className="dot" /></div>
                  </div>
                )}
                {videoResults.map((vid, i) => (
                  <div key={i} className="playground-video-card">
                    {vid.url ? (
                      <video src={vid.url} controls loop className="playground-video-player" />
                    ) : (
                      <p className="text-dim">{vid.status || t("playground.video.processing")}</p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
