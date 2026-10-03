import { useState, useEffect, useMemo } from "react";
import { getOptimizationPlan, getOptimizationConfig, setOptimizationConfig, applyRecommendation, syncOptimizationConfigToCloud, syncOptimizationConfigFromCloud, getCloudBilling, saveModelCombo, setDefaultComboName, previewComboOptimization, applyComboOptimization } from "../utils/commands";
import type { OptimizationPlan, OptimizationConfig, CloudBilling, ModelCombo, TaskProfile } from "../utils/types";
import { useAppStore } from "../stores/appStore";
import { useTranslation } from "../i18n";

const MODE_LABEL_KEYS: Record<string, string> = {
  quality_first: "optimization.mode.quality_first",
  value_first: "optimization.mode.value_first",
  auto_strategy: "optimization.mode.auto_strategy",
};

const MODE_DESC_KEYS: Record<string, string> = {
  quality_first: "optimization.modeDesc.quality_first",
  value_first: "optimization.modeDesc.value_first",
  auto_strategy: "optimization.modeDesc.auto_strategy",
};

const TASK_TYPE_META_KEYS: Record<string, { title: string; subtitle: string; hint: string }> = {
  general_chat: {
    title: "optimization.task.general_chat.title",
    subtitle: "optimization.task.general_chat.subtitle",
    hint: "optimization.task.general_chat.hint",
  },
  tool_calling: {
    title: "optimization.task.tool_calling.title",
    subtitle: "optimization.task.tool_calling.subtitle",
    hint: "optimization.task.tool_calling.hint",
  },
  structured_output: {
    title: "optimization.task.structured_output.title",
    subtitle: "optimization.task.structured_output.subtitle",
    hint: "optimization.task.structured_output.hint",
  },
  long_context: {
    title: "optimization.task.long_context.title",
    subtitle: "optimization.task.long_context.subtitle",
    hint: "optimization.task.long_context.hint",
  },
  vision: {
    title: "optimization.task.vision.title",
    subtitle: "optimization.task.vision.subtitle",
    hint: "optimization.task.vision.hint",
  },
};

const HIGH_COST_MODELS = new Set([
  "gpt-4o", "gpt-4-turbo", "gpt-4.1", "gpt-4.1-mini",
  "claude-3-5-sonnet", "claude-3-opus", "claude-opus-4-8", "claude-fable-5", "claude-sonnet-5",
  "gemini-2.0-pro", "gemini-2.5-pro",
]);

function isHighCost(model: string): boolean {
  for (const prefix of HIGH_COST_MODELS) {
    if (model.startsWith(prefix)) return true;
  }
  return false;
}

function ModeIcon({ mode }: { mode: string }) {
  const props = {
    viewBox: "0 0 24 24",
    width: 16,
    height: 16,
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.8,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": true,
  };

  if (mode === "quality_first") {
    return <svg {...props}><path d="M12 4l2.1 4.3 4.7.7-3.4 3.3.8 4.7-4.2-2.2-4.2 2.2.8-4.7L5.2 9l4.7-.7L12 4z" /></svg>;
  }
  if (mode === "value_first") {
    return <svg {...props}><path d="M12 3v18" /><path d="M16.5 7.5c0-1.7-1.8-3-4.5-3s-4.5 1.3-4.5 3 1.8 3 4.5 3 4.5 1.3 4.5 3-1.8 3-4.5 3-4.5-1.3-4.5-3" /></svg>;
  }
  return <svg {...props}><rect x="4" y="4" width="16" height="16" rx="4" /><path d="M9 9h6v6H9z" /><path d="M9 2.5v3" /><path d="M15 2.5v3" /><path d="M9 18.5v3" /><path d="M15 18.5v3" /></svg>;
}

function BulbIcon() {
  return (
    <svg viewBox="0 0 24 24" width={14} height={14} fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M9 18h6" />
      <path d="M10 22h4" />
      <path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 18 8 6 6 0 0 0 6 8c0 1 .23 2.23 1.5 3.5A4.61 4.61 0 0 1 8.91 14" />
    </svg>
  );
}

function CheckCircleIcon() {
  return (
    <svg viewBox="0 0 24 24" width={34} height={34} fill="none" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <circle cx="12" cy="12" r="9" />
      <path d="m8.5 12.2 2.4 2.4 4.6-4.9" />
    </svg>
  );
}

export function OptimizationPage({ embedded, taskType = "general_chat" }: { embedded?: boolean; taskType?: TaskProfile["task_type"] }) {
  const { t } = useTranslation();
  const [plan, setPlan] = useState<OptimizationPlan | null>(null);
  const [config, setConfig] = useState<OptimizationConfig | null>(null);
  const [billing, setBilling] = useState<CloudBilling | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [activeMode, setActiveMode] = useState("value_first");
  const [applying, setApplying] = useState<string | null>(null);
  const [syncMsg, setSyncMsg] = useState<string | null>(null);
  const [appliedRecs, setAppliedRecs] = useState<Set<string>>(new Set());
  const [previewData, setPreviewData] = useState<any>(null);
  const [showPreview, setShowPreview] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [applyingPreview, setApplyingPreview] = useState<string | null>(null);
  const currentTaskMetaKeys = TASK_TYPE_META_KEYS[taskType] || TASK_TYPE_META_KEYS.general_chat;
  const currentTaskMeta = useMemo(
    () => ({
      title: t(currentTaskMetaKeys.title),
      subtitle: t(currentTaskMetaKeys.subtitle),
      hint: t(currentTaskMetaKeys.hint),
    }),
    [t, currentTaskMetaKeys],
  );
  const taskSections = useMemo(
    () =>
      Object.entries(TASK_TYPE_META_KEYS).map(([key, meta]) => ({
        key,
        title: t(meta.title),
        subtitle: t(meta.subtitle),
        hint: t(meta.hint),
        active: key === taskType,
      })),
    [t, taskType],
  );

  useEffect(() => {
    (async () => {
      setLoading(true);
      try {
        const [cfg, p, b] = await Promise.all([
          getOptimizationConfig(),
          getOptimizationPlan("value_first", taskType),
          getCloudBilling().catch(() => null),
        ]);
        setConfig(cfg);
        setPlan(p);
        setBilling(b);
        setActiveMode(cfg.mode || "value_first");
      } catch {
      } finally {
        setLoading(false);
      }
    })();
  }, [taskType]);

  const switchMode = async (mode: string) => {
    setLoading(true);
    setActiveMode(mode);
    try {
      const p = await getOptimizationPlan(mode, taskType);
      setPlan(p);
      if (config) {
        const updated = { ...config, mode };
        setConfig(updated);
        await setOptimizationConfig({ mode } as Record<string, any>);
      }
    } catch {
    } finally {
      setLoading(false);
    }
  };

  const updateConfig = async (patch: Partial<OptimizationConfig>) => {
    if (!config) return;
    setSaving(true);
    const updated = { ...config, ...patch };
    setConfig(updated);
    try {
      await setOptimizationConfig(updated as Record<string, any>);
    } catch {
    } finally {
      setSaving(false);
    }
  };

  const handleApply = async (toModel: string) => {
    setApplying(toModel);
    try {
      await applyRecommendation(toModel, config?.default_preset || "budget");
      setAppliedRecs((prev) => new Set(prev).add(toModel));
    } catch {
    } finally {
      setApplying(null);
    }
  };

  const handleSyncToCloud = async () => {
    setSyncMsg(t("optimization.syncToCloudLoading"));
    try {
      await syncOptimizationConfigToCloud();
      setSyncMsg(t("optimization.syncToCloudSuccess"));
    } catch {
      setSyncMsg(t("optimization.syncToCloudFailedLogin"));
    }
    setTimeout(() => setSyncMsg(null), 3000);
  };

  const handleSyncFromCloud = async () => {
    setSyncMsg(t("optimization.syncFromCloudLoading"));
    try {
      const result = await syncOptimizationConfigFromCloud();
      if (result) {
        setConfig(result);
        setActiveMode(result.mode || "value_first");
        const p = await getOptimizationPlan(result.mode || "value_first", taskType);
        setPlan(p);
        setSyncMsg(t("optimization.syncFromCloudSuccess"));
      } else {
        setSyncMsg(t("optimization.syncFromCloudNoConfig"));
      }
    } catch {
      setSyncMsg(t("optimization.syncFromCloudFailedLogin"));
    }
    setTimeout(() => setSyncMsg(null), 3000);
  };

  if (loading && !plan) return <div className="page"><div className="loading">{t("optimization.loadingAnalyzing")}</div></div>;

  const currentPlanID = billing?.plan_id || "free";
  const isFree = currentPlanID === "free";
  const isPro = currentPlanID === "pro";

  const planLabel: Record<string, string> = {
    free: t("plan.free"),
    pro: t("plan.pro"),
    teams: t("plan.teams"),
    enterprise: t("plan.enterprise"),
  };

  const content = (
    <>
      <div className="plan-context-banner">
        <span className="plan-context-icon"><BulbIcon /></span>
        <span className="plan-context-text">
          {t("optimization.currentScenario", { title: currentTaskMeta.title, hint: currentTaskMeta.hint })}
          <br />
          {isFree && t("optimization.planAdvice.free")}
          {isPro && t("optimization.planAdvice.pro")}
          {!isFree && !isPro && t("optimization.planAdvice.other", { plan: planLabel[currentPlanID] })}
        </span>
      </div>

      <div className="section section-card">
        <div className="section-heading-row">
          <h2 className="section-title">{t("optimization.taskSectionTitle")}</h2>
          <div className="section-meta-text">{t("optimization.taskSectionHint")}</div>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 12 }}>
          {taskSections.map((section) => (
            <div
              key={section.key}
              style={{
                border: section.active ? "1px solid var(--accent)" : "1px solid var(--border)",
                background: section.active ? "var(--primary-soft)" : "var(--bg-elevated)",
                borderRadius: 12,
                padding: 14,
              }}
            >
              <div style={{ fontWeight: 600, marginBottom: 6 }}>{section.title}</div>
              <div className="text-muted">{section.subtitle}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="section section-card">
        <h2 className="section-title">{t("optimization.modeTitle")}</h2>
        <div className="mode-picker">
          {["quality_first", "value_first", "auto_strategy"].map((mode) => (
            <button
              key={mode}
              className={`mode-btn ${activeMode === mode ? "active" : ""}`}
              onClick={() => switchMode(mode)}
              disabled={loading}
            >
              <span className="mode-icon"><ModeIcon mode={mode} /></span>
              <span className="mode-label">{t(MODE_LABEL_KEYS[mode] || mode)}</span>
              <span className="mode-desc">{t(MODE_DESC_KEYS[mode] || mode)}</span>
            </button>
          ))}
        </div>
      </div>

      {plan && plan.recommendations.length > 0 && (
        <div className="section section-card">
          <div className="section-heading-row">
            <h2 className="section-title">{currentTaskMeta.title}</h2>
            <div className="strategy-badge">
              {t("optimization.strategyLabel", {
                strategy:
                  plan.strategy === "cost_optimized"
                    ? t("optimization.strategy.cost_optimized")
                    : plan.strategy === "balanced"
                      ? t("optimization.strategy.balanced")
                      : (MODE_LABEL_KEYS[plan.mode] ? t(MODE_LABEL_KEYS[plan.mode]) : plan.mode),
              })}
            </div>
            <div className="section-meta-text">{currentTaskMeta.subtitle}</div>
          </div>
          <div className="rec-list">
            {plan.recommendations.map((rec, idx) => {
              const recIsHighCost = isHighCost(rec.to_model);
              const needsUpgrade = recIsHighCost && (isFree || isPro);
              const useBYOK = isFree && rec.channel_name && rec.channel_name !== "local";

              return (
                <div key={idx} className="rec-card">
                  <div className="rec-main">
                    <div className="rec-content">
                      <div className="rec-header">
                        <span className="rec-from">{rec.from_model}</span>
                        <span className="rec-arrow">→</span>
                        <span className="rec-to">{rec.to_model}</span>
                        {rec.model_tag === "open" && <span className="tag tag-open">{t("optimization.tagOpenSource")}</span>}
                        {rec.model_tag !== "open" && <span className="tag tag-closed">{t("optimization.tagClosedSource")}</span>}
                        {recIsHighCost && <span className="tag tag-highcost">{t("optimization.tagHighCost")}</span>}
                        {useBYOK && <span className="tag tag-byok">{t("optimization.tagByokSuggested")}</span>}
                        {needsUpgrade && (
                          <span className="tag tag-premium">
                            {isFree ? t("optimization.upgradeProPlus") : t("optimization.upgradeTeamsPlus")}
                          </span>
                        )}
                      </div>
                      <div className="rec-info">
                        <span className="rec-channel">
                          {t("optimization.channelLabel", { name: rec.channel_name || t("optimization.unspecified") })}
                        </span>
                        <span className="rec-sep">·</span>
                        <span className="rec-quality">
                          {t("optimization.qualityLabel")}{
                            rec.quality_diff === "equivalent_or_better"
                              ? t("optimization.qualityEquivalentOrBetter")
                              : rec.quality_diff === "better"
                                ? t("optimization.qualityBetter")
                                : t("optimization.qualityAcceptable")
                          }
                        </span>
                        {rec.avg_latency_ms > 0 && (
                          <>
                            <span className="rec-sep">·</span>
                            <span className="rec-latency">
                              {t("optimization.latencyLabel")}{rec.avg_latency_ms < 1000 ? `${Math.round(rec.avg_latency_ms)}ms` : `${(rec.avg_latency_ms / 1000).toFixed(1)}s`}
                            </span>
                          </>
                        )}
                        {rec.error_rate > 0 && (
                          <>
                            <span className="rec-sep">·</span>
                            <span className="rec-error">
                              {t("optimization.errorRateLabel")}{(rec.error_rate * 100).toFixed(1)}%
                            </span>
                          </>
                        )}
                      </div>
                      <div className="rec-reason">{rec.reason}</div>
                      {needsUpgrade && (
                        <div className="rec-plan-hint">
                          {isFree ? t("optimization.unavailableHintFree") : t("optimization.unavailableHintPro")}
                        </div>
                      )}
                      {useBYOK && (
                        <div className="rec-plan-hint rec-plan-hint-byok">
                          {t("optimization.byokHint")}
                        </div>
                      )}
                    </div>
                    <div className="rec-side">
                      {rec.savings_usd > 0 ? (
                        <span className="rec-save green">{t("optimization.savingsMonthly", { amount: rec.savings_usd.toFixed(2) })}</span>
                      ) : (
                        <span className="rec-save blue">{t("optimization.upgradeMonthly", { amount: (-rec.savings_usd).toFixed(2) })}</span>
                      )}
                      <div className="rec-actions">
                        <button
                          className="btn-rec-preview"
                          onClick={async () => {
                            setPreviewLoading(true);
                            try {
                              const data = await previewComboOptimization(activeMode, taskType);
                              setPreviewData(data);
                              setShowPreview(true);
                            } catch {}
                            setPreviewLoading(false);
                          }}
                          disabled={previewLoading}
                        >
                          {previewLoading ? t("optimization.loading") : t("optimization.previewDiff")}
                        </button>
                        <button
                          className={`btn-rec-apply ${appliedRecs.has(rec.to_model) ? "applied" : ""}`}
                          onClick={() => handleApply(rec.to_model)}
                          disabled={applying === rec.to_model}
                        >
                          {applying === rec.to_model ? t("optimization.applying") : appliedRecs.has(rec.to_model) ? t("optimization.applied") : t("optimization.applyRecommendation")}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {plan && plan.recommendations.length >= 2 && (
        <div className="section section-card">
          <div className="section-heading-row">
            <h2 className="section-title">{t("optimization.comboSuggestionTitle")}</h2>
            <span className="badge badge-new">{t("optimization.newFeature")}</span>
          </div>
          <p className="hint" style={{ marginBottom: 12 }}>
            {t("optimization.comboSuggestionHint")}
          </p>

          {(() => {
            const recs = plan.recommendations;
            const steps = recs.slice(0, 3).map((r, i) => ({
              role: i === 0 ? "primary" : i === 1 ? "backup" : "last_resort" as "primary" | "backup" | "last_resort",
              model: r.to_model,
              channelName: r.channel_name || "",
              quality: r.quality_diff,
              reason: r.reason,
            }));
            const stepLabels: Record<string, string> = {
              primary: t("combo.stepRolePrimary"),
              backup: t("combo.stepRoleBackup"),
              last_resort: t("combo.stepRoleLastResort"),
            };
            const comboName = t("optimization.comboSuggestionName", {
              task: currentTaskMeta.title,
              mode: MODE_LABEL_KEYS[activeMode] ? t(MODE_LABEL_KEYS[activeMode]) : activeMode,
            });
            // Check if combo exists in backend store, not just local appliedRecs
            const combos = useAppStore.getState().combos;
            const comboExists = combos && combos.some((c: ModelCombo) => c.name === comboName);
            const isApplied = comboExists || appliedRecs.has(comboName);

            return (
              <div className="combo-suggestion-card">
                <div className="combo-suggestion-steps">
                  {steps.map((s, si) => (
                    <div key={si} className={`combo-suggestion-step ${s.role}`}>
                      <div className="combo-suggestion-step-role">{stepLabels[s.role]}</div>
                      <div className="combo-suggestion-step-model">{s.model}</div>
                      {s.channelName && <div className="combo-suggestion-step-channel">{t("optimization.channelLabel", { name: s.channelName })}</div>}
                      <div className="combo-suggestion-step-reason">{s.reason}</div>
                      {si < steps.length - 1 && <div className="combo-suggestion-arrow">{t("optimization.fallbackArrow")}</div>}
                    </div>
                  ))}
                </div>
                <div className="combo-suggestion-actions">
                  <button
                    className="btn-primary"
                    disabled={isApplied || loading}
                    onClick={async () => {
                      setLoading(true);
                      try {
                        const newCombo: ModelCombo = {
                          name: comboName,
                          models: steps.map(s => s.model),
                          steps: steps.map(s => ({ model: s.model, step_role: s.role })),
                          strategy: "fallback",
                          sticky_uses: 1,
                          task_profile: { task_type: taskType, priority_providers: [], fallback_order: [] },
                        };
                        await saveModelCombo(newCombo);
                        await setDefaultComboName(newCombo.name);
                        setAppliedRecs(prev => new Set(prev).add(comboName));
                      } catch {}
                      setLoading(false);
                    }}
                  >
                    {isApplied ? t("optimization.applied") : t("optimization.applyAsCombo")}
                  </button>
                  <span className="hint" style={{ marginLeft: 8 }}>{t("optimization.fallbackStepsCount", { count: steps.length })}</span>
                </div>
              </div>
            );
          })()}
        </div>
      )}

      {plan && plan.recommendations.length === 0 && (
        <div className="empty-state">
          <div className="empty-icon empty-icon-check"><CheckCircleIcon /></div>
          <p>{t("optimization.noRecommendations", { task: currentTaskMeta.title })}</p>
        </div>
      )}

      {showPreview && previewData && (
        <div className="preview-modal-overlay" onClick={() => setShowPreview(false)}>
          <div className="preview-modal" onClick={(e) => e.stopPropagation()}>
            <div className="preview-modal-header">
              <h3>{t("optimization.previewTitle")}</h3>
              <button className="preview-modal-close" onClick={() => setShowPreview(false)}>✕</button>
            </div>
            <div className="preview-modal-body">
              {previewData.recommendations?.map((rec: any, idx: number) => (
                <div key={idx} className="preview-rec-card">
                  <div className="preview-rec-title">
                    <span className="preview-rec-from">{rec.from_model}</span>
                    <span className="preview-rec-arrow">→</span>
                    <span className="preview-rec-to">{rec.to_model}</span>
                  </div>
                  {rec.savings_usd != null && (
                    <div className="preview-rec-impact">
                      <span className={rec.savings_usd >= 0 ? "green" : "blue"}>
                        {t("optimization.previewMonthlyImpact", { action: rec.savings_usd >= 0 ? t("optimization.saving") : t("optimization.increase"), amount: Math.abs(rec.savings_usd).toFixed(2) })}
                      </span>
                      {rec.quality_diff && (
                        <span className="preview-rec-quality">
                          {t("optimization.previewQuality", {
                            value:
                              rec.quality_diff === "equivalent_or_better"
                                ? t("optimization.qualityEquivalentOrBetter")
                                : rec.quality_diff === "better"
                                  ? t("optimization.qualityBetter")
                                  : t("optimization.qualityAcceptable"),
                          })}
                        </span>
                      )}
                    </div>
                  )}
                  {rec.reason && <div className="preview-rec-reason">{rec.reason}</div>}
                  <div className="preview-steps-compare">
                    <div className="preview-steps-column">
                      <div className="preview-steps-column-title">{t("optimization.currentSteps")}</div>
                      {rec.current_steps?.length > 0 ? rec.current_steps.map((s: any, si: number) => (
                        <div key={si} className="preview-step-item">{s.model}</div>
                      )) : <div className="preview-step-item preview-step-empty">{t("optimization.noExistingCombo")}</div>}
                    </div>
                    <div className="preview-steps-arrow">→</div>
                    <div className="preview-steps-column">
                      <div className="preview-steps-column-title">{t("optimization.proposedSteps")}</div>
                      {rec.proposed_steps?.map((s: any, si: number) => (
                        <div key={si} className="preview-step-item preview-step-proposed">{s.model}</div>
                      ))}
                    </div>
                  </div>
                </div>
              ))}
            </div>
            <div className="preview-modal-footer">
              <button className="btn-outline" onClick={() => setShowPreview(false)}>{t("optimization.close")}</button>
              <button
                className="btn-primary"
                disabled={applyingPreview !== null}
                onClick={async () => {
                  const recs = previewData.recommendations || [];
                  const toModel = recs[0]?.to_model;
                  if (!toModel) return;
                  setApplyingPreview(toModel);
                  try {
                    await applyComboOptimization(toModel, activeMode, true);
                    setAppliedRecs(prev => new Set(prev).add(toModel));
                    setShowPreview(false);
                  } catch {}
                  setApplyingPreview(null);
                }}
              >
                {applyingPreview ? t("optimization.applying") : t("optimization.applyRecommendation")}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="section section-card">
        <div className="section-heading-row">
          <h2 className="section-title">{t("optimization.configTitle")}</h2>
          <div className="config-toolbar">
            <button className="btn-outline" onClick={() => setShowConfig(!showConfig)}>
              {showConfig ? t("optimization.collapseConfig") : t("optimization.expandConfig")}
            </button>
            <button className="btn-outline" onClick={handleSyncToCloud}>
              {t("optimization.syncToCloud")}
            </button>
            <button className="btn-outline" onClick={handleSyncFromCloud}>
              {t("optimization.restoreFromCloud")}
            </button>
            {syncMsg && <span className="sync-msg">{syncMsg}</span>}
          </div>
        </div>
        {showConfig && config && (
          <div className="opt-config-panel">
            <div className="config-group">
              <h4>{t("optimization.configPenaltyTitle")}</h4>
              <label className="toggle-row">
                <span>{t("optimization.configPenaltyEnabled")}</span>
                <input type="checkbox" checked={config.penalty_enabled} onChange={(e) => updateConfig({ penalty_enabled: e.target.checked })} />
              </label>
              <label className="input-row">
                <span>{t("optimization.configPenaltyDecay")}</span>
                <input type="number" value={config.penalty_decay_sec} min={30} max={600} onChange={(e) => updateConfig({ penalty_decay_sec: parseInt(e.target.value) || 120 })} disabled={!config.penalty_enabled} />
              </label>
            </div>

            <div className="config-group">
              <h4>{t("optimization.configHealthTitle")}</h4>
              <label className="toggle-row">
                <span>{t("optimization.configHealthEnabled")}</span>
                <input type="checkbox" checked={config.health_check_enabled} onChange={(e) => updateConfig({ health_check_enabled: e.target.checked })} />
              </label>
              <label className="input-row">
                <span>{t("optimization.configHealthInterval")}</span>
                <input type="number" value={config.health_check_sec} min={60} max={3600} onChange={(e) => updateConfig({ health_check_sec: parseInt(e.target.value) || 300 })} disabled={!config.health_check_enabled} />
              </label>
              <label className="input-row">
                <span>{t("optimization.configHealthMaxFailures")}</span>
                <input type="number" value={config.health_max_failures} min={1} max={20} onChange={(e) => updateConfig({ health_max_failures: parseInt(e.target.value) || 3 })} disabled={!config.health_check_enabled} />
              </label>
            </div>

            <div className="config-group">
              <h4>{t("optimization.configCooldownTitle")}</h4>
              <label className="toggle-row">
                <span>{t("optimization.configCooldownEnabled")}</span>
                <input type="checkbox" checked={config.cooldown_enabled} onChange={(e) => updateConfig({ cooldown_enabled: e.target.checked })} />
              </label>
              <label className="input-row">
                <span>{t("optimization.configCooldownDuration")}</span>
                <input type="number" value={config.cooldown_sec} min={10} max={600} onChange={(e) => updateConfig({ cooldown_sec: parseInt(e.target.value) || 120 })} disabled={!config.cooldown_enabled} />
              </label>
            </div>

            <div className="config-group">
              <h4>{t("optimization.configStickyTitle")}</h4>
              <label className="toggle-row">
                <span>{t("optimization.configStickyEnabled")}</span>
                <input type="checkbox" checked={config.sticky_enabled} onChange={(e) => updateConfig({ sticky_enabled: e.target.checked })} />
              </label>
              <label className="input-row">
                <span>{t("optimization.configStickyTtl")}</span>
                <input type="number" value={config.sticky_ttl_sec} min={60} max={86400} onChange={(e) => updateConfig({ sticky_ttl_sec: parseInt(e.target.value) || 1800 })} disabled={!config.sticky_enabled} />
              </label>
            </div>

            <div className="config-group">
              <h4>{t("optimization.configPresetTitle")}</h4>
              <label className="toggle-row">
                <span>{t("optimization.configPresetEnabled")}</span>
                <input type="checkbox" checked={config.preset_enabled} onChange={(e) => updateConfig({ preset_enabled: e.target.checked })} />
              </label>
              <label className="input-row">
                <span>{t("optimization.configPresetDefault")}</span>
                <select value={config.default_preset} onChange={(e) => updateConfig({ default_preset: e.target.value })} disabled={!config.preset_enabled}>
                  <option value="intelligence">{t("combo.sortPresetIntelligence")}</option>
                  <option value="budget">{t("combo.sortPresetBudget")}</option>
                  <option value="speed">{t("combo.sortPresetSpeed")}</option>
                </select>
              </label>
            </div>

            {saving && <div className="saving-hint">{t("optimization.savingConfig")}</div>}
          </div>
        )}
      </div>
    </>
  );

  return embedded ? content : <div className="page optimization-page">{content}</div>;
}
