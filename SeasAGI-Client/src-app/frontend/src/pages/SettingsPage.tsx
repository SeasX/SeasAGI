import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import { useTranslation } from "../i18n";
import { SUPPORTED_LOCALES, type Locale } from "../i18n/index";
import { getErrorMessage } from "../utils/errors";
import type { MITMStatus } from "../utils/types";

export function SettingsPage() {
  const auth = useAppStore((s) => s.auth);
  const appConfig = useAppStore((s) => s.appConfig);
  const setAuth = useAppStore((s) => s.setAuth);
  const setAppConfig = useAppStore((s) => s.setAppConfig);
  const { t, locale, setLocale } = useTranslation();

  const [activeTab, setActiveTab] = useState<"account" | "routing" | "rtk" | "caveman" | "mitm" | "rate-limit" | "performance" | "mcp-gateway" | "advanced">("account");
  const [routingStrategy, setRoutingStrategy] = useState(appConfig?.routing_strategy || "fallback");
  const [stickyUses, setStickyUses] = useState(appConfig?.sticky_channel_use || 3);
  const [platformApiURL, setPlatformApiURL] = useState("");
  const [platformApiError, setPlatformApiError] = useState("");
  const [mitmStatus, setMitmStatus] = useState<MITMStatus | null>(null);
  const [mitmRules, setMitmRules] = useState<string[]>([]);
  const [mitmNewRule, setMitmNewRule] = useState("");
  const [mitmError, setMitmError] = useState("");
  const [mitmToggling, setMitmToggling] = useState(false);
  const [mitmDomainTests, setMitmDomainTests] = useState<Record<string, { loading: boolean; result: Record<string, any> | null }>>({});

  const handleLogout = async () => {
    try {
      await cmd.logout();
      setAuth({ is_logged_in: false, user_id: null, email: null });
    } catch (e) {
      console.error(e);
    }
  };

  const loadMITMState = async () => {
    try {
      const [status, rules] = await Promise.all([
        cmd.getMITMStatus(),
        cmd.getMITMRules(),
      ]);
      setMitmStatus(status);
      setMitmRules(rules);
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => {
    void loadMITMState();
  }, []);

  useEffect(() => {
    if (activeTab !== "mitm") return undefined;
    const timer = window.setInterval(() => {
      void loadMITMState();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [activeTab]);

  useEffect(() => {
    void (async () => {
      try {
        const baseUrl = await cmd.getPlatformAPIBaseURL();
        setPlatformApiURL(baseUrl);
      } catch {
        // state unavailable, keep defaults
      }
    })();
  }, []);

  const handleSaveRouting = async () => {
    try {
      await cmd.updateRoutingSettings(routingStrategy, stickyUses);
      if (appConfig) {
        setAppConfig({ ...appConfig, routing_strategy: routingStrategy, sticky_channel_use: stickyUses });
      }
    } catch (e) {
      console.error(e);
    }
  };

  const rtkEnabled = appConfig?.rtk_enabled ?? true;
  const cavemanEnabled = appConfig?.caveman_enabled ?? false;

  // 保存 RTK/Caveman 设置到后端（持久化 + 热更新网关），成功后再更新前端状态。
  const saveRTKSettings = async (next: { rtk_enabled?: boolean; caveman_enabled?: boolean; caveman_style?: string }) => {
    if (!appConfig) return;
    const merged = {
      rtk_enabled: appConfig.rtk_enabled,
      caveman_enabled: appConfig.caveman_enabled,
      caveman_style: appConfig.caveman_style,
      ...next,
    };
    try {
      await cmd.setRTKSettings(merged.rtk_enabled, appConfig.rtk_max_output_chars, merged.caveman_enabled, merged.caveman_style);
      setAppConfig({ ...appConfig, ...merged });
    } catch (e) {
      console.error(e);
    }
  };

  const toggleRtkEnabled = () => {
    void saveRTKSettings({ rtk_enabled: !rtkEnabled });
  };

  const toggleCavemanEnabled = () => {
    void saveRTKSettings({ caveman_enabled: !cavemanEnabled });
  };

  const handleQuitApp = async () => {
    try {
      await cmd.quitApp();
    } catch (e) {
      console.error(e);
    }
  };

  const handleToggleCardKeyDown = (event: React.KeyboardEvent<HTMLDivElement>, toggle: () => void) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    toggle();
  };

  return (
    <div className="page settings-page">
      <div className="page-header page-hero">
        <div>
          <h1>{t("settings.title")}</h1>
          <p className="page-subtitle">{t("settings.subtitle")}</p>
        </div>
        <div className="hero-metrics">
          <div className="hero-metric-card">
            <span className="hero-metric-label">{t("settings.metricLoginStatus")}</span>
            <strong className="hero-metric-value">{auth.is_logged_in ? t("settings.loggedIn") : t("settings.notLoggedIn")}</strong>
          </div>
        </div>
      </div>

      <div className="tab-bar settings-tab-bar">
        <button className={activeTab === "account" ? "active" : ""} onClick={() => setActiveTab("account")}>{t("settings.account")}</button>
        <button className={activeTab === "routing" ? "active" : ""} onClick={() => setActiveTab("routing")}>{t("settings.routing")}</button>
        <button className={activeTab === "rtk" ? "active" : ""} onClick={() => setActiveTab("rtk")}>{t("settings.rtkTab")}</button>
        <button className={activeTab === "caveman" ? "active" : ""} onClick={() => setActiveTab("caveman")}>{t("settings.cavemanTab")}</button>
        <button className={activeTab === "mitm" ? "active" : ""} onClick={() => setActiveTab("mitm")}>MITM</button>
        <button className={activeTab === "rate-limit" ? "active" : ""} onClick={() => setActiveTab("rate-limit")}>{t("settings.rateLimit")}</button>
        <button className={activeTab === "performance" ? "active" : ""} onClick={() => setActiveTab("performance")}>{t("settings.performance")}</button>
        <button className={activeTab === "mcp-gateway" ? "active" : ""} onClick={() => setActiveTab("mcp-gateway")}>{t("settings.mcpGateway")}</button>
        <button className={activeTab === "advanced" ? "active" : ""} onClick={() => setActiveTab("advanced")}>{t("settings.advanced")}</button>
      </div>

      {activeTab === "account" && (
        <div className="settings-stack">
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.sectionBasic")}</h2>
              <p className="hint">{t("settings.sectionBasicHint")}</p>
            </div>
            <div className="form-group">
              <label>{t("settings.language")}</label>
              <select value={locale} onChange={(e) => setLocale(e.target.value as Locale)}>
                {SUPPORTED_LOCALES.map((l) => (
                  <option key={l.code} value={l.code}>{l.label}</option>
                ))}
              </select>
            </div>

          </div>

          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.platformApiUrl")}</h2>
              <p className="hint">{t("settings.platformApiUrlHint")}</p>
            </div>
            <div className="form-row">
              <div className="form-group" style={{ marginBottom: 0, flex: 1 }}>
                <input
                  value={platformApiURL}
                  onChange={(e) => setPlatformApiURL(e.target.value)}
                  placeholder="https://seasagi.seasx.ai/api/v1"
                />
              </div>
              <button
                onClick={async () => {
                  try {
                    setPlatformApiError("");
                    await cmd.setPlatformAPIBaseURL(platformApiURL);
                    if (appConfig) {
                      setAppConfig({ ...appConfig, platform_api_base_url: platformApiURL });
                    }
                  } catch (e) {
                    setPlatformApiError(getErrorMessage(e, t("settings.saveFailed")));
                  }
                }}
                className="btn-primary"
                style={{ height: 40, alignSelf: "flex-end" }}
              >
                {t("settings.platformApiUrlSave")}
              </button>
            </div>
            {platformApiError && <p className="error-msg" style={{ marginTop: 8 }}>{platformApiError}</p>}
          </div>

          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.accountStatus")}</h2>
              <p className="hint">{t("settings.accountStatusHint")}</p>
            </div>
            {auth.is_logged_in ? (
              <div className="settings-inline-actions">
                <span className="auth-email-label" style={{ marginBottom: 0 }}>{t("settings.loggedInAs", { email: auth.email || "" })}</span>
                <button onClick={handleLogout} className="btn-secondary btn-sm">{t("settings.logout")}</button>
              </div>
            ) : (
              <p className="auth-email-label" style={{ marginBottom: 0 }}>
                {t("settings.notLoggedInDash")} <NavLink to="/auth" className="link-btn">{t("settings.goLoginRegister")}</NavLink>
              </p>
            )}
          </div>

          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.quitApp")}</h2>
              <p className="hint">{t("settings.quitAppHint")}</p>
            </div>
            <div className="settings-inline-actions">
              <span className="hint" style={{ marginBottom: 0 }}>{t("settings.quitAppDescription")}</span>
              <button onClick={handleQuitApp} className="btn-danger btn-sm">{t("settings.quitAppButton")}</button>
            </div>
          </div>
        </div>
      )}

      {activeTab === "rtk" && (
        <div className="settings-stack">
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.rtkEnabled")}</h2>
              <p className="hint">{t("settings.rtkSectionHint")}</p>
            </div>
            <div
              className={`settings-toggle-card settings-toggle-card-selectable${rtkEnabled ? " is-active" : ""}`}
              role="checkbox"
              aria-checked={rtkEnabled}
              tabIndex={0}
              onClick={toggleRtkEnabled}
              onKeyDown={(event) => handleToggleCardKeyDown(event, toggleRtkEnabled)}
            >
              <div>
                <div className="settings-toggle-title">{t("settings.rtkEnabled")}</div>
                <div className="hint">{t("settings.rtkHint")}</div>
                <div style={{ marginTop: 8, display: "flex", gap: 6, flexWrap: "wrap" }}>
                  <span className="badge badge-blue">{t("settings.rtkBadgeCompression")}</span>
                  <span className="badge badge-green">{t("settings.rtkBadgeRuntime")}</span>
                </div>
              </div>
              <div className="form-checkbox" style={{ marginBottom: 0 }}>
                <input
                  type="checkbox"
                  id="rtkEnabled"
                  checked={rtkEnabled}
                  onClick={(event) => event.stopPropagation()}
                  onChange={(e) => {
                    void saveRTKSettings({ rtk_enabled: e.target.checked });
                  }}
                />
                <label htmlFor="rtkEnabled" onClick={(event) => event.stopPropagation()}>{t("settings.enable")}</label>
              </div>
            </div>
          </div>
        </div>
      )}

      {activeTab === "caveman" && (
        <div className="settings-stack">
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.cavemanEnabled")}</h2>
              <p className="hint">{t("settings.cavemanSectionHint")}</p>
            </div>
            <div
              className={`settings-toggle-card settings-toggle-card-selectable${cavemanEnabled ? " is-active" : ""}`}
              role="checkbox"
              aria-checked={cavemanEnabled}
              tabIndex={0}
              onClick={toggleCavemanEnabled}
              onKeyDown={(event) => handleToggleCardKeyDown(event, toggleCavemanEnabled)}
            >
              <div>
                <div className="settings-toggle-title">{t("settings.cavemanEnabled")}</div>
                <div className="hint">{t("settings.cavemanHint")}</div>
                <div style={{ marginTop: 8, display: "flex", gap: 6, flexWrap: "wrap" }}>
                  <span className="badge badge-blue">{t("settings.cavemanBadgeOutput")}</span>
                  <span className="badge badge-yellow">{t("settings.cavemanBadgeStyle")}</span>
                </div>
              </div>
              <div className="form-row">
                <div className="form-checkbox" style={{ marginBottom: 0 }}>
                  <input
                    type="checkbox"
                    id="cavemanEnabled"
                    checked={cavemanEnabled}
                    onClick={(event) => event.stopPropagation()}
                    onChange={(e) => {
                      void saveRTKSettings({ caveman_enabled: e.target.checked });
                    }}
                  />
                  <label htmlFor="cavemanEnabled" onClick={(event) => event.stopPropagation()}>{t("settings.enable")}</label>
                </div>
                {cavemanEnabled && (
                  <div className="form-group settings-inline-select" style={{ marginBottom: 0 }}>
                    <select
                      value={appConfig?.caveman_style ?? "concise"}
                      onClick={(event) => event.stopPropagation()}
                      onChange={(e) => {
                        void saveRTKSettings({ caveman_style: e.target.value });
                      }}
                    >
                      <option value="concise">{t("settings.cavemanStyleConcise")}</option>
                      <option value="brief">{t("settings.cavemanStyleBrief")}</option>
                      <option value="minimal">{t("settings.cavemanStyleMinimal")}</option>
                      <option value="terse">{t("settings.cavemanStyleTerse")}</option>
                    </select>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {activeTab === "routing" && (
        <div className="settings-stack">
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.routingStrategy")}</h2>
              <p className="hint">{t("settings.routingHint")}</p>
            </div>
            <div className="routing-form-grid">
              <div className="form-group">
                <label>{t("settings.defaultStrategy")}</label>
                <select value={routingStrategy} onChange={(e) => setRoutingStrategy(e.target.value)}>
                  <option value="fallback">{t("settings.fallback")}</option>
                  <option value="round_robin">{t("settings.roundRobin")}</option>
                  <option value="weighted_round_robin">{t("settings.weightedRoundRobin")}</option>
                  <option value="fill_first">{t("settings.fillFirst")}</option>
                  <option value="priority">{t("settings.priority")}</option>
                </select>
              </div>
              {routingStrategy === "round_robin" && (
                <div className="form-group">
                  <label>{t("settings.stickyCount")}</label>
                  <input type="number" value={stickyUses} onChange={(e) => setStickyUses(Number(e.target.value))} />
                </div>
              )}
            </div>
            <p className="hint">
              {routingStrategy === "fallback" ? t("settings.fallbackHint") :
               routingStrategy === "round_robin" ? t("settings.roundRobinHint") :
               routingStrategy === "weighted_round_robin" ? t("settings.weightedRRHint") :
               routingStrategy === "fill_first" ? t("settings.fillFirstHint") :
               routingStrategy === "priority" ? t("settings.priorityHint") :
               t("settings.routingHint")}
            </p>
          </div>

          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.byokTitle")}</h2>
              <p className="hint">{t("settings.byokHint")}</p>
            </div>
            <div className="byok-status-card">
              <div className="byok-title">{t("settings.byokInfoTitle")}</div>
              <div className="byok-desc">
                <p><strong>{t("settings.byokScenarioLabel")}</strong>{t("settings.byokScenarioText")}</p>
                <p><strong>{t("settings.byokCostLabel")}</strong>{t("settings.byokCostText")}</p>
                <p><strong>{t("settings.byokProtectionLabel")}</strong>{t("settings.byokProtectionText")}</p>
                <p><strong>{t("settings.byokUsageLabel")}</strong>{t("settings.byokUsageText")}</p>
              </div>
            </div>
            <label className="toggle-row settings-toggle-block">
              <span>{t("settings.byokToggle")}</span>
              <input type="checkbox" checked={true} onChange={() => {}} />
            </label>
          </div>

          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>{t("settings.autoLaunch")}</h2>
              <p className="hint">{t("settings.autoLaunchHint")}</p>
            </div>
            <div className="settings-inline-actions">
              <button onClick={() => cmd.setAutoLaunch(true)} className="btn-secondary">{t("settings.turnOn")}</button>
              <button onClick={() => cmd.setAutoLaunch(false)} className="btn-secondary">{t("settings.turnOff")}</button>
            </div>
          </div>
          <div className="settings-save-row">
            <button onClick={handleSaveRouting} className="btn-primary">{t("settings.saveRouting")}</button>
          </div>
        </div>
      )}

      {activeTab === "mitm" && (
        <div className="settings-stack">
          {/* 一键开关 */}
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>MITM Proxy</h2>
              <p className="hint">一键接管 LLM API 流量，自动路由到本地网关。</p>
            </div>
            {mitmError && <div className="error-msg" style={{ marginTop: 12 }}>{mitmError}</div>}
            <div
              className={`settings-toggle-card settings-toggle-card-selectable${mitmStatus?.state === "running" ? " is-active" : ""}`}
              role="switch"
              aria-checked={mitmStatus?.state === "running"}
              tabIndex={0}
              onClick={async () => {
                if (mitmToggling) return;
                const isRunning = mitmStatus?.state === "running";
                setMitmToggling(true);
                try {
                  setMitmError("");
                  if (isRunning) {
                    await cmd.stopMITM();
                  } else {
                    await cmd.startMITM();
                  }
                  await loadMITMState();
                } catch (e) {
                  setMitmError(getErrorMessage(e, isRunning ? "停止失败" : "启动失败"));
                } finally {
                  setMitmToggling(false);
                }
              }}
              onKeyDown={(event) => {
                if (event.key !== "Enter" && event.key !== " ") return;
                event.preventDefault();
                (event.currentTarget as HTMLElement).click();
              }}
            >
              <div>
                <div className="settings-toggle-title">
                  {mitmToggling ? "切换中..." : mitmStatus?.state === "running" ? "已启用" : "未启用"}
                </div>
                <div className="hint">
                  {mitmStatus?.state === "running"
                    ? `代理端口 ${mitmStatus.proxy_port} · CA ${mitmStatus.ca_installed ? "已安装" : "未安装"} · 系统代理 ${mitmStatus.system_proxy ? "已设置" : "未设置"}`
                    : "点击开启一键接管 AI API 流量"}
                </div>
                <div style={{ marginTop: 8, display: "flex", gap: 6, flexWrap: "wrap" }}>
                  <span className={`badge ${mitmStatus?.system_proxy_active ? "badge-green" : "badge-red"}`}>
                    {mitmStatus?.system_proxy_active ? t("settings.mitmProxyActive") : t("settings.mitmProxyInactive")}
                  </span>
                  {mitmStatus?.residual_system_proxy && (
                    <span className="badge badge-yellow">{t("settings.mitmResidualProxy")}</span>
                  )}
                </div>
                {mitmStatus?.last_error && (
                  <div className="error-msg" style={{ marginTop: 8 }}>{mitmStatus.last_error}</div>
                )}
              </div>
              <div className="form-checkbox" style={{ marginBottom: 0 }}>
                <input
                  type="checkbox"
                  checked={mitmStatus?.state === "running"}
                  readOnly
                  style={{ width: 20, height: 20 }}
                />
              </div>
            </div>
          </div>

          {/* 域名配置 + 连通性检查 */}
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>接管域名</h2>
              <p className="hint">被接管的域名流量将通过 MITM 代理转发到本地网关，点击「测试」检查连通性。</p>
            </div>
            <div className="form-row">
              <div className="form-group" style={{ marginBottom: 0, flex: 1 }}>
                <input
                  value={mitmNewRule}
                  onChange={(e) => setMitmNewRule(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && mitmNewRule.trim()) {
                      (e.currentTarget as HTMLElement).blur();
                    }
                  }}
                  placeholder="api.example.com"
                />
              </div>
              <button
                onClick={async () => {
                  if (!mitmNewRule.trim()) return;
                  try {
                    setMitmError("");
                    await cmd.addMITMRule(mitmNewRule.trim());
                    setMitmNewRule("");
                    await loadMITMState();
                  } catch (e) {
                    setMitmError(getErrorMessage(e, "添加域名失败"));
                  }
                }}
                className="btn-primary btn-sm"
                style={{ height: 40, alignSelf: "flex-end" }}
              >
                添加
              </button>
            </div>
            <div style={{ marginTop: 12 }}>
              {mitmRules.length === 0 && (
                <p className="hint">暂无接管域名，请添加需要拦截的 API 域名。</p>
              )}
              {mitmRules.map((domain) => {
                const testState = mitmDomainTests[domain];
                const testResult = testState?.result;
                return (
                  <div key={domain} style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8, flexWrap: "wrap" }}>
                    <span className="hint" style={{ marginBottom: 0, minWidth: 180, fontFamily: "monospace" }}>{domain}</span>
                    {testState?.loading && <span className="badge badge-yellow">测试中...</span>}
                    {testResult && !testState?.loading && (
                      <>
                        <span className={`badge ${testResult.reachable ? "badge-green" : "badge-red"}`}>
                          {testResult.reachable ? `可达 ${testResult.status_code}` : "不可达"}
                        </span>
                        {testResult.intercepted && <span className="badge badge-blue">已接管</span>}
                        {testResult.latency_ms > 0 && <span className="hint" style={{ marginBottom: 0 }}>{testResult.latency_ms}ms</span>}
                        {testResult.error && <span className="hint" style={{ marginBottom: 0, color: "var(--error, #e53e3e)" }}>{String(testResult.error).slice(0, 60)}</span>}
                      </>
                    )}
                    <div style={{ marginLeft: "auto", display: "flex", gap: 4 }}>
                      <button
                        onClick={async () => {
                          setMitmDomainTests((prev) => ({ ...prev, [domain]: { loading: true, result: null } }));
                          try {
                            const result = await cmd.testMITMDomain(domain);
                            setMitmDomainTests((prev) => ({ ...prev, [domain]: { loading: false, result } }));
                          } catch (e) {
                            setMitmDomainTests((prev) => ({ ...prev, [domain]: { loading: false, result: { reachable: false, error: String(e) } } }));
                          }
                        }}
                        className="btn-secondary btn-sm"
                        disabled={mitmStatus?.state !== "running" || testState?.loading}
                      >
                        测试
                      </button>
                      <button
                        onClick={async () => {
                          try {
                            setMitmError("");
                            await cmd.removeMITMRule(domain);
                            setMitmDomainTests((prev) => {
                              const next = { ...prev };
                              delete next[domain];
                              return next;
                            });
                            await loadMITMState();
                          } catch (e) {
                            setMitmError(getErrorMessage(e, "移除域名失败"));
                          }
                        }}
                        className="btn-danger btn-sm"
                      >
                        移除
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* 拦截日志 */}
          {mitmStatus?.state === "running" && (
            <MITMInterceptLog />
          )}

          {/* CLI 兼容提示 */}
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>CLI 兼容</h2>
              <p className="hint">终端/CLI 工具如不读取系统代理，可手动设置环境变量。</p>
            </div>
            <MITMEnvHint running={mitmStatus?.state === "running"} />
          </div>

          {/* 已知局限（不接管） */}
          <div className="tab-content section-card">
            <div className="section-heading">
              <h2>已知局限（不接管）</h2>
              <p className="hint">一键接管基于系统代理（HTTP/HTTPS over TCP）与本地 CA 实现，以下流量无法被监控或治理，属于客观盲区：</p>
            </div>
            <ul className="hint" style={{ margin: 0, paddingLeft: 20, lineHeight: 1.9 }}>
              <li><strong>HTTP/3（QUIC / UDP 443）</strong>：系统代理仅接管 TCP，基于 UDP 的流量不会被拦截。</li>
              <li><strong>h2 / gRPC 长连接</strong>：仅对标准 HTTP/HTTPS 请求做解析与治理，gRPC 等二进制长连接不被改写。</li>
              <li><strong>证书固定（Certificate Pinning）客户端</strong>：内置证书校验的应用会拒绝本地 CA，从而绕过 MITM。</li>
              <li><strong>不读取系统代理的进程</strong>：自行实现网络栈或显式指定代理的 CLI/工具需手动设置环境变量（见上）。</li>
              <li><strong>未纳入接管域名清单的流量</strong>：仅清单内的域名会被转发到本地网关。</li>
            </ul>
            <p className="hint" style={{ marginTop: 8 }}>WebSocket（ws/wss）流量已复用同一网关主链路，可正常接管与记账。</p>
          </div>
        </div>
      )}

      {activeTab === "rate-limit" && (
        <div className="settings-stack">
          <RateLimitPanel />
        </div>
      )}

      {activeTab === "performance" && (
        <div className="settings-stack">
          <PerfAuditPanel />
        </div>
      )}

      {activeTab === "mcp-gateway" && (
        <div className="settings-stack">
          <MCPGatewayPanel />
        </div>
      )}

      {activeTab === "advanced" && (
        <div className="settings-stack">
          <LogRotationPanel />
          <CloudSyncPanel />
        </div>
      )}
    </div>
  );
}

function RateLimitPanel() {
  const { t } = useTranslation();
  const [config, setConfig] = useState<Record<string, any> | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState(false);

  const loadConfig = async () => {
    setLoading(true);
    try {
      const cfg = await cmd.getRateLimitConfig();
      setConfig(cfg);
    } catch {
      setError("Failed to load rate limit config");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadConfig(); }, []);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    setError("");
    setSuccess(false);
    try {
      await cmd.setRateLimitConfig(config);
      setSuccess(true);
      setTimeout(() => setSuccess(false), 3000);
    } catch (err) {
      setError(getErrorMessage(err, "Failed to save rate limit config"));
    } finally {
      setSaving(false);
    }
  };

  const update = (key: string, value: any) => {
    setConfig(prev => prev ? { ...prev, [key]: value } : prev);
  };

  if (loading) return <div className="loading-state">{t("common.loading")}</div>;

  return (
    <div className="tab-content section-card">
      <div className="section-heading">
        <h2>{t("settings.rateLimit")}</h2>
        <p className="hint">{t("rateLimit.subtitle")}</p>
      </div>

      {error && <div className="form-error">{error}</div>}
      {success && <div className="form-success">{t("rateLimit.saved")}</div>}

      <div className="settings-toggle-card" style={{ marginBottom: 16 }}>
        <div>
          <strong>{t("rateLimit.enable")}</strong>
          <p className="hint" style={{ marginTop: 4 }}>{t("rateLimit.enableHint")}</p>
        </div>
        <label className="toggle-switch">
          <input type="checkbox" checked={!!config?.enabled} onChange={(e) => update("enabled", e.target.checked)} />
          <span className="toggle-slider" />
        </label>
      </div>

      <div className="form-grid" style={{ gridTemplateColumns: "1fr 1fr", gap: 16 }}>
        <div className="form-group">
          <label className="form-label">{t("rateLimit.defaultRPM")}</label>
          <input
            type="number"
            className="form-input"
            value={config?.default_rpm ?? 0}
            onChange={(e) => update("default_rpm", parseInt(e.target.value) || 0)}
            placeholder="0 = unlimited"
          />
          <p className="hint">{t("rateLimit.defaultRPMHint")}</p>
        </div>
        <div className="form-group">
          <label className="form-label">{t("rateLimit.defaultTPM")}</label>
          <input
            type="number"
            className="form-input"
            value={config?.default_tpm ?? 0}
            onChange={(e) => update("default_tpm", parseInt(e.target.value) || 0)}
            placeholder="0 = unlimited"
          />
          <p className="hint">{t("rateLimit.defaultTPMHint")}</p>
        </div>
        <div className="form-group">
          <label className="form-label">{t("rateLimit.minInterval")}</label>
          <input
            type="number"
            className="form-input"
            value={config?.min_interval_ms ?? 0}
            onChange={(e) => update("min_interval_ms", parseInt(e.target.value) || 0)}
            placeholder="0 = no minimum"
          />
          <p className="hint">{t("rateLimit.minIntervalHint")}</p>
        </div>
        <div className="form-group">
          <label className="form-label">{t("rateLimit.maxConcurrent")}</label>
          <input
            type="number"
            className="form-input"
            value={config?.max_concurrent ?? 0}
            onChange={(e) => update("max_concurrent", parseInt(e.target.value) || 0)}
            placeholder="0 = unlimited"
          />
          <p className="hint">{t("rateLimit.maxConcurrentHint")}</p>
        </div>
        <div className="form-group" style={{ gridColumn: "span 2" }}>
          <label className="form-label">{t("rateLimit.maxWait")}</label>
          <input
            type="number"
            className="form-input"
            value={config?.max_wait_ms ?? 15000}
            onChange={(e) => update("max_wait_ms", parseInt(e.target.value) || 0)}
            placeholder="15000"
          />
          <p className="hint">{t("rateLimit.maxWaitHint")}</p>
        </div>
        <div className="form-group" style={{ gridColumn: "span 2" }}>
          <label className="form-label">{t("rateLimit.monthlyCostLimit")}</label>
          <input
            type="number"
            step="0.01"
            className="form-input"
            value={config?.monthly_cost_limit_usd ?? 0}
            onChange={(e) => update("monthly_cost_limit_usd", parseFloat(e.target.value) || 0)}
            placeholder="0 = unlimited"
          />
          <p className="hint">{t("rateLimit.monthlyCostLimitHint")}</p>
        </div>
      </div>

      <div style={{ marginTop: 16, display: "flex", gap: 12 }}>
        <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
          {saving ? t("common.submitting") : t("common.save")}
        </button>
        <button className="btn btn-secondary" onClick={loadConfig}>
          {t("common.reset")}
        </button>
      </div>
    </div>
  );
}

function PerfAuditPanel() {
  const { t } = useTranslation();
  const [report, setReport] = useState<Record<string, any> | null>(null);
  const [loading, setLoading] = useState(false);

  const loadReport = async () => {
    setLoading(true);
    try {
      const r = await cmd.getPerfAuditReport();
      setReport(r);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { loadReport(); }, []);

  return (
    <div className="tab-content section-card">
      <div className="section-heading">
        <h2>{t('settings.performance')}</h2>
        <p className="hint">{t('perf.subtitle')}</p>
        <button className="btn btn-primary btn-sm" onClick={loadReport} disabled={loading} style={{ marginTop: 8 }}>
          {loading ? t('diagnostics.running') : t('perf.refresh')}
        </button>
      </div>
      {report && (
        <div className="diag-grid">
          <div className="diag-card">
            <span className="diag-label">{t('perf.totalFindings')}</span>
            <span className="diag-value">{report.total_findings || 0}</span>
          </div>
          <div className="diag-card">
            <span className="diag-label">{t('perf.slowQueries')}</span>
            <span className="badge-red">{report.slow_queries || 0}</span>
          </div>
          <div className="diag-card">
            <span className="diag-label">{t('perf.totalDbQueries')}</span>
            <span className="diag-value">{report.total_db_queries || 0}</span>
          </div>
        </div>
      )}
      {report?.findings && Array.isArray(report.findings) && report.findings.length > 0 && (
        <table className="diag-table" style={{ marginTop: 16 }}>
          <thead>
            <tr>
              <th>{t('perf.category')}</th>
              <th>{t('perf.title')}</th>
              <th>{t('perf.effort')}</th>
              <th>{t('perf.impact')}</th>
              <th>{t('perf.score')}</th>
            </tr>
          </thead>
          <tbody>
            {(report.findings as any[]).map((f, i) => (
              <tr key={i}>
                <td>{f.category}</td>
                <td>{f.title}</td>
                <td>{f.effort}</td>
                <td>{f.impact}</td>
                <td><span className="badge-blue">{f.score?.toFixed(1)}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function MCPGatewayPanel() {
  const { t } = useTranslation();
  const [tools, setTools] = useState<Record<string, any>[]>([]);
  const [auditLog, setAuditLog] = useState<Record<string, any>[]>([]);

  useEffect(() => {
    (async () => {
      try {
        const tl = await cmd.getMCPGatewayTools();
        setTools(tl || []);
        const al = await cmd.getMCPAuditLog();
        setAuditLog(al || []);
      } catch {
        // ignore
      }
    })();
  }, []);

  return (
    <div className="tab-content section-card">
      <div className="section-heading">
        <h2>{t('settings.mcpGateway')}</h2>
        <p className="hint">{t('mcp.subtitle')}</p>
      </div>
      <h4>{t('mcp.tools')} ({tools.length})</h4>
      <table className="diag-table" style={{ marginTop: 8 }}>
        <thead>
          <tr><th>{t('mcp.toolName')}</th><th>{t('mcp.description')}</th></tr>
        </thead>
        <tbody>
          {tools.map((tool, i) => (
            <tr key={i}>
              <td><span className="badge-blue">{tool.name}</span></td>
              <td>{tool.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {auditLog.length > 0 && (
        <>
          <h4 style={{ marginTop: 16 }}>{t('mcp.auditLog')}</h4>
          <table className="diag-table" style={{ marginTop: 8 }}>
            <thead>
              <tr><th>{t('mcp.toolName')}</th><th>{t('mcp.success')}</th><th>{t('mcp.timestamp')}</th></tr>
            </thead>
            <tbody>
              {auditLog.map((log, i) => (
                <tr key={i}>
                  <td>{log.tool_name}</td>
                  <td><span className={log.success ? "badge-green" : "badge-red"}>{log.success ? "OK" : "FAIL"}</span></td>
                  <td>{log.timestamp}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  );
}

function LogRotationPanel() {
  const { t } = useTranslation();
  const [config, setConfig] = useState<Record<string, any> | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const c = await cmd.getLogRotationConfig();
        setConfig(c);
      } catch {
        // ignore
      }
    })();
  }, []);

  return (
    <div className="tab-content section-card">
      <div className="section-heading">
        <h2>{t('logRotation.title')}</h2>
        <p className="hint">{t('logRotation.subtitle')}</p>
      </div>
      {config && (
        <div className="diag-grid">
          <div className="diag-card">
            <span className="diag-label">{t('logRotation.maxFileSize')}</span>
            <span className="diag-value">{config.max_file_size_mb} MB</span>
          </div>
          <div className="diag-card">
            <span className="diag-label">{t('logRotation.retentionDays')}</span>
            <span className="diag-value">{config.retention_days} {t('logRotation.days')}</span>
          </div>
          <div className="diag-card">
            <span className="diag-label">{t('logRotation.maxFiles')}</span>
            <span className="diag-value">{config.max_files}</span>
          </div>
          <div className="diag-card">
            <span className="diag-label">{t('logRotation.currentSize')}</span>
            <span className="diag-value">{config.current_size_mb} MB</span>
          </div>
        </div>
      )}
    </div>
  );
}

function CloudSyncPanel() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<Record<string, any> | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const s = await cmd.getCloudSyncStatus();
        setStatus(s);
      } catch {
        // ignore
      }
    })();
  }, []);

  return (
    <div className="tab-content section-card">
      <div className="section-heading">
        <h2>{t('cloudSync.title')}</h2>
        <p className="hint">{t('cloudSync.subtitle')}</p>
      </div>
      {status && (
        <>
          <div className="diag-grid">
            <div className="diag-card">
              <span className="diag-label">{t('cloudSync.hmacEnabled')}</span>
              <span className={status.hmac_enabled ? "badge-green" : "badge-red"}>{status.hmac_enabled ? 'ON' : 'OFF'}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('cloudSync.versionHash')}</span>
              <span className="diag-value" style={{ fontFamily: 'monospace', fontSize: 11 }}>{(status.version_hash || '').slice(0, 16) || '-'}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('cloudSync.lastSync')}</span>
              <span className="diag-value">{status.last_sync || '-'}</span>
            </div>
            <div className="diag-card">
              <span className="diag-label">{t('cloudSync.conflicts')}</span>
              <span className={status.conflict_count > 0 ? "badge-red" : "badge-green"}>{status.conflict_count || 0}</span>
            </div>
          </div>
          {status.conflicts && Array.isArray(status.conflicts) && status.conflicts.length > 0 && (
            <table className="diag-table" style={{ marginTop: 16 }}>
              <thead>
                <tr><th>{t('cloudSync.conflictType')}</th><th>{t('cloudSync.conflictDetail')}</th></tr>
              </thead>
              <tbody>
                {(status.conflicts as any[]).map((c, i) => (
                  <tr key={i}>
                    <td>{c.type}</td>
                    <td>{c.detail}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  );
}

function MITMInterceptLog() {
  const [intercepts, setIntercepts] = useState<Record<string, any>[]>([]);

  useEffect(() => {
    const timer = window.setInterval(async () => {
      try {
        const data = await cmd.getMITMRecentIntercepts(20);
        setIntercepts(data || []);
      } catch {
        // ignore
      }
    }, 3000);
    return () => window.clearInterval(timer);
  }, []);

  if (intercepts.length === 0) {
    return null;
  }

  return (
    <div className="tab-content section-card">
      <div className="section-heading">
        <h2>拦截日志</h2>
        <p className="hint">最近 20 条被接管的请求（每 3 秒刷新）</p>
      </div>
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", fontSize: 13, borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ textAlign: "left", borderBottom: "1px solid var(--border, #333)" }}>
              <th style={{ padding: "4px 8px" }}>时间</th>
              <th style={{ padding: "4px 8px" }}>方法</th>
              <th style={{ padding: "4px 8px" }}>域名</th>
              <th style={{ padding: "4px 8px" }}>路径</th>
              <th style={{ padding: "4px 8px" }}>状态</th>
              <th style={{ padding: "4px 8px" }}>延迟</th>
            </tr>
          </thead>
          <tbody>
            {intercepts.slice().reverse().map((entry, i) => (
              <tr key={i} style={{ borderBottom: "1px solid var(--border-light, #222)" }}>
                <td style={{ padding: "4px 8px", color: "var(--text-secondary, #888)" }}>
                  {entry.time ? new Date(entry.time).toLocaleTimeString() : "-"}
                </td>
                <td style={{ padding: "4px 8px" }}>{entry.method || "-"}</td>
                <td style={{ padding: "4px 8px", fontFamily: "monospace" }}>{entry.host || "-"}</td>
                <td style={{ padding: "4px 8px", fontFamily: "monospace", maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis" }}>{entry.path || "-"}</td>
                <td style={{ padding: "4px 8px" }}>
                  <span className={`badge ${entry.status >= 200 && entry.status < 400 ? "badge-green" : "badge-red"}`}>
                    {entry.status || "-"}
                  </span>
                </td>
                <td style={{ padding: "4px 8px" }}>{entry.duration_ms ? `${entry.duration_ms.toFixed(1)}ms` : "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function MITMEnvHint({ running }: { running: boolean }) {
  const [hint, setHint] = useState<Record<string, string> | null>(null);

  useEffect(() => {
    if (!running) return;
    void (async () => {
      try {
        const result = await cmd.getMITMEnvHint();
        setHint(result);
      } catch (e) {
        console.error(e);
      }
    })();
  }, [running]);

  if (!running || !hint) {
    return <p className="hint">启动 MITM 代理后显示 CLI 环境变量设置命令。</p>;
  }

  return (
    <div>
      <p className="hint">Shell: <strong>{hint.shell}</strong></p>
      <div className="form-row" style={{ marginTop: 8 }}>
        <code style={{ flex: 1, padding: 8, background: "var(--bg-code, #1e1e1e)", borderRadius: 4, fontSize: 13, overflowX: "auto", whiteSpace: "nowrap" }}>
          {hint.export_cmds}
        </code>
        <button
          onClick={() => navigator.clipboard?.writeText(hint.export_cmds)}
          className="btn-secondary btn-sm"
          style={{ height: 40 }}
        >
          复制
        </button>
      </div>
      <p className="hint" style={{ marginTop: 8 }}>停止后取消设置：</p>
      <div className="form-row" style={{ marginTop: 4 }}>
        <code style={{ flex: 1, padding: 8, background: "var(--bg-code, #1e1e1e)", borderRadius: 4, fontSize: 13, overflowX: "auto", whiteSpace: "nowrap" }}>
          {hint.unset_cmds}
        </code>
        <button
          onClick={() => navigator.clipboard?.writeText(hint.unset_cmds)}
          className="btn-secondary btn-sm"
          style={{ height: 40 }}
        >
          复制
        </button>
      </div>
    </div>
  );
}
