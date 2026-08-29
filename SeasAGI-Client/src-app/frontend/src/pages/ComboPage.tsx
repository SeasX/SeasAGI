import { useEffect, useMemo, useState } from "react";
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useAppStore } from "../stores/appStore";
import * as cmd from "../utils/commands";
import { getErrorMessage } from "../utils/errors";
import type { Channel, ModelCombo, ModelComboStep, QuickStrategy } from "../utils/types";
import { useTranslation } from "../i18n";
import { validateForm, comboFormSchema } from "../utils/validation";

type ComboTab = "combos" | "templates";

const emptyDraft = (): ModelCombo => ({
  name: "",
  description: "",
  tags: [],
  steps: [{ channel_id: "", model: "" }],
  models: [],
  strategy: "fallback",
  sticky_uses: 1,
});

export function ComboPage({ embedded, showTemplatesTab }: { embedded?: boolean; showTemplatesTab?: boolean }) {
  const { t } = useTranslation();
  const auth = useAppStore((s) => s.auth);
  const combos = useAppStore((s) => s.combos);
  const templates = useAppStore((s) => s.templates);
  const quickStrategies = useAppStore((s) => s.quickStrategies);
  const setCombos = useAppStore((s) => s.setCombos);
  const setTemplates = useAppStore((s) => s.setTemplates);
  const setQuickStrategies = useAppStore((s) => s.setQuickStrategies);

  const [channels, setChannels] = useState<Channel[]>([]);
  const [activeTab, setActiveTab] = useState<ComboTab>(showTemplatesTab ? "templates" : "combos");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [templateError, setTemplateError] = useState("");
  const [templateSearch, setTemplateSearch] = useState("");
  const [editing, setEditing] = useState<ModelCombo | null>(null);
  const [editingSource, setEditingSource] = useState<"create" | "edit" | "template">("create");
  const [originalName, setOriginalName] = useState("");
  const [tagsInput, setTagsInput] = useState("");
  const [cloudCombos, setCloudCombos] = useState<ModelCombo[]>([]);
  const [syncing, setSyncing] = useState(false);
  const [syncMsg, setSyncMsg] = useState("");

  const loadPageData = async () => {
    setLoading(true);
    setError("");
    setTemplateError("");
    try {
      const [comboItems, channelItems, quickStrategyItems] = await Promise.all([
        cmd.listModelCombos(),
        cmd.listChannels(),
        cmd.getQuickStrategies().catch(() => [] as QuickStrategy[]),
      ]);
      setCombos(comboItems || []);
      setChannels(channelItems || []);
      setQuickStrategies(quickStrategyItems || []);
      if (auth.is_logged_in) {
        try {
          const [templateItems, cloudItems] = await Promise.all([
            cmd.getOfficialComboTemplates(),
            cmd.fetchCloudCombos().catch(() => [] as Record<string, any>[]),
          ]);
          setTemplates((templateItems || []) as ModelCombo[]);
          // Map cloud combos to local ModelCombo format
          setCloudCombos((cloudItems || []).map((cc: Record<string, any>) => ({
            combo_id: cc.combo_id,
            name: cc.logical_name || cc.display_name || cc.combo_id,
            logical_name: cc.logical_name,
            display_name: cc.display_name,
            description: cc.description,
            tags: cc.tags ? (typeof cc.tags === 'string' ? JSON.parse(cc.tags) : cc.tags) : [],
            quick_strategy: cc.quick_strategy,
            task_profile: cc.task_profile,
            steps: (cc.steps || []).map((s: Record<string, any>) => ({
              channel_id: s.channel_id,
              model: s.model,
              step_role: s.step_role,
            })),
            strategy: cc.strategy || "fallback",
            sticky_uses: cc.sticky_uses || 1,
            status: cc.status,
            source: "cloud",
            version: cc.version,
          })));
        } catch (err) {
          setTemplates([]);
          setTemplateError(getErrorMessage(err, t("template.loadFailed")));
        }
      } else {
        setTemplates([]);
        setCloudCombos([]);
      }
    } catch (err) {
      setError(getErrorMessage(err, t("combo.loadFailed")));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadPageData();
  }, [auth.is_logged_in]);

  // Merge local and cloud combos for display, dedup by logical_name
  const allCombos = useMemo(() => {
    const localSet = new Set(combos.map((c) => c.name));
    const merged = [...combos];
    for (const cc of cloudCombos) {
      if (!localSet.has(cc.name)) {
        merged.push(cc);
      }
    }
    return merged;
  }, [combos, cloudCombos]);

  const templateResults = useMemo(() => {
    const keyword = templateSearch.trim().toLowerCase();
    if (!keyword) return templates;
    return templates.filter((item) => {
      return (
        item.name.toLowerCase().includes(keyword) ||
        item.description?.toLowerCase().includes(keyword) ||
        item.tags?.some((tag) => tag.toLowerCase().includes(keyword))
      );
    });
  }, [templateSearch, templates]);

  const openCreate = () => {
    setOriginalName("");
    setTagsInput("");
    setEditingSource("create");
    setEditing(emptyDraft());
  };

  const openEdit = (combo: ModelCombo) => {
    setOriginalName(combo.name);
    setTagsInput((combo.tags || []).join(", "));
    setEditingSource("edit");
    setEditing({
      ...combo,
      steps: combo.steps?.length ? combo.steps : [{ channel_id: "", model: "" }],
    });
  };

  const openFromTemplate = (template: ModelCombo) => {
    setOriginalName("");
    setTagsInput((template.tags || []).join(", "));
    setEditingSource("template");
    setEditing({
      ...template,
      name: "",
      steps: template.steps?.length ? template.steps : [{ channel_id: "", model: "" }],
    });
  };

  const closeEditor = () => {
    setEditing(null);
    setOriginalName("");
    setTagsInput("");
  };

  const updateStep = (index: number, key: "channel_id" | "model", value: string) => {
    if (!editing) return;
    const nextSteps = editing.steps.map((step, stepIndex) => (stepIndex === index ? { ...step, [key]: value } : step));
    setEditing({ ...editing, steps: nextSteps });
  };

  const updateStepMeta = (index: number, key: string, value: any) => {
    if (!editing) return;
    const nextSteps = editing.steps.map((step, stepIndex) =>
      stepIndex === index ? { ...step, [key]: value } : step
    );
    setEditing({ ...editing, steps: nextSteps });
  };

  const addStepProvider = (stepIndex: number, provider: { channel_id: string; model: string; priority: number }) => {
    if (!editing) return;
    const nextSteps = editing.steps.map((step, index) => {
      if (index !== stepIndex) return step;
      const providers = [...(step.providers || []), provider];
      return { ...step, providers };
    });
    setEditing({ ...editing, steps: nextSteps });
  };

  const removeStepProvider = (stepIndex: number, providerIndex: number) => {
    if (!editing) return;
    const nextSteps = editing.steps.map((step, index) => {
      if (index !== stepIndex) return step;
      const providers = (step.providers || []).filter((_, pIdx) => pIdx !== providerIndex);
      return { ...step, providers };
    });
    setEditing({ ...editing, steps: nextSteps });
  };

  const toggleStepChannel = (stepIndex: number, channelId: string) => {
    if (!editing) return;
    const nextSteps = editing.steps.map((step, index) => {
      if (index !== stepIndex) return step;
      const channels = step.channels || [];
      if (channels.includes(channelId)) {
        return { ...step, channels: channels.filter((c) => c !== channelId) };
      }
      return { ...step, channels: [...channels, channelId] };
    });
    setEditing({ ...editing, steps: nextSteps });
  };

  const addStep = () => {
    if (!editing) return;
    setEditing({
      ...editing,
      steps: [...(editing.steps || []), { channel_id: "", model: "" }],
    });
  };

  const removeStep = (index: number) => {
    if (!editing) return;
    const nextSteps = editing.steps.filter((_, stepIndex) => stepIndex !== index);
    setEditing({
      ...editing,
      steps: nextSteps.length ? nextSteps : [{ channel_id: "", model: "" }],
    });
  };

  const applyQuickStrategy = (alias: "stable_first" | "cost_first" | "speed_first" | "tools_first") => {
    const strategies = {
      stable_first: {
        name: t("combo.stableFirst"),
        description: t("combo.stableFirstDesc"),
        strategy: "fallback",
        steps: [
          { channel_id: "openai", model: "gpt-4o" },
          { channel_id: "anthropic", model: "claude-sonnet-5" },
          { channel_id: "google", model: "gemini-2.5-flash" },
        ],
      },
      cost_first: {
        name: t("combo.costFirst"),
        description: t("combo.costFirstDesc"),
        strategy: "fallback",
        steps: [
          { channel_id: "openrouter", model: "minimax/minimax-m3" },
          { channel_id: "google", model: "gemini-2.5-flash" },
          { channel_id: "openai", model: "gpt-4o-mini" },
        ],
      },
      speed_first: {
        name: t("combo.speedFirst"),
        description: t("combo.speedFirstDesc"),
        strategy: "fallback",
        steps: [
          { channel_id: "google", model: "gemini-2.5-flash" },
          { channel_id: "openai", model: "gpt-4o-mini" },
          { channel_id: "anthropic", model: "claude-haiku-4-5" },
        ],
      },
      tools_first: {
        name: t("combo.toolsFirst"),
        description: t("combo.toolsFirstDesc"),
        strategy: "fallback",
        steps: [
          { channel_id: "openai", model: "gpt-4o" },
          { channel_id: "anthropic", model: "claude-sonnet-5" },
          { channel_id: "google", model: "gemini-2.5-flash" },
        ],
      },
    };

    const strategy = strategies[alias];
    const matchedQuickStrategy = quickStrategies.find((item) => item.alias === alias);
    setEditing({
      name: strategy.name,
      description: strategy.description,
      tags: ["quick-strategy", alias],
      steps: strategy.steps,
      strategy: strategy.strategy,
      sticky_uses: 1,
      models: strategy.steps.map((s) => s.model),
      quick_strategy: alias,
      task_profile: matchedQuickStrategy?.task_profile,
    });
    setOriginalName("");
    setTagsInput(["quick-strategy", alias].join(", "));
    setEditingSource("create");
  };

  const handleSave = async () => {
    if (!editing) return;
    const payload: ModelCombo = {
      ...editing,
      name: editing.name.trim(),
      description: editing.description?.trim() || "",
      tags: tagsInput.split(",").map((tag) => tag.trim()).filter(Boolean),
      steps: editing.steps.map((step) => ({
        ...step,
        channel_id: step.channel_id?.trim() || "",
        model: step.model.trim(),
      })).filter((step) => step.model),
    };
    if (!payload.name) {
      setError(t("combo.nameRequired"));
      return;
    }
    if (!payload.steps.length) {
      setError(t("combo.stepsSuccessRequired"));
      return;
    }
    const v = validateForm(comboFormSchema, payload);
    if (!v.success) {
      setError(Object.values(v.errors).join("; "));
      return;
    }

    try {
      setSaving(true);
      setError("");
      if (editingSource === "edit" && originalName && originalName !== payload.name) {
        await cmd.deleteModelCombo(originalName);
      }
      await cmd.saveModelCombo(payload);

      // If logged in, also sync to cloud
      if (auth.is_logged_in) {
        try {
          const stepsJSON = JSON.stringify(payload.steps.map((s) => ({
            channel_id: s.channel_id,
            model: s.model,
          })));
          await cmd.pushCloudCombo(
            payload.logical_name || payload.name.toLowerCase().replace(/\s+/g, "-"),
            payload.display_name || payload.name,
            payload.description || "",
            payload.strategy,
            payload.sticky_uses,
            payload.quick_strategy || "",
            payload.task_profile || {},
            stepsJSON
          );
        } catch {
          // Cloud sync is best-effort
        }
      }

      const latest = await cmd.listModelCombos();
      setCombos(latest || []);
      closeEditor();
      setActiveTab("combos");
    } catch (err) {
      setError(getErrorMessage(err, t("combo.saveFailed")));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (name: string) => {
    if (!window.confirm(t("combo.confirmDelete", { name }))) return;
    try {
      // Find combo to get combo_id for cloud delete
      const combo = combos.find((c) => c.name === name) || cloudCombos.find((c) => c.name === name);
      await cmd.deleteModelCombo(name);
      // If it has a combo_id and user is logged in, also delete from cloud
      if (combo?.combo_id && auth.is_logged_in) {
        try {
          await cmd.deleteCloudCombo(combo.combo_id);
        } catch {
          // Cloud delete is best-effort
        }
      }
      const [localCombos, cloudItems] = await Promise.all([
        cmd.listModelCombos(),
        auth.is_logged_in ? cmd.fetchCloudCombos().catch(() => [] as Record<string, any>[]) : Promise.resolve([] as Record<string, any>[]),
      ]);
      setCombos(localCombos || []);
      if (auth.is_logged_in) {
        setCloudCombos((cloudItems || []).map((cc: Record<string, any>) => ({
          combo_id: cc.combo_id,
          name: cc.logical_name || cc.display_name || cc.combo_id,
          logical_name: cc.logical_name,
          display_name: cc.display_name,
          description: cc.description,
          tags: cc.tags ? (typeof cc.tags === 'string' ? JSON.parse(cc.tags) : cc.tags) : [],
          quick_strategy: cc.quick_strategy,
          task_profile: cc.task_profile,
          steps: (cc.steps || []).map((s: Record<string, any>) => ({
            channel_id: s.channel_id,
            model: s.model,
            step_role: s.step_role,
          })),
          strategy: cc.strategy || "fallback",
          sticky_uses: cc.sticky_uses || 1,
          status: cc.status,
          source: "cloud",
          version: cc.version,
        })));
      }
    } catch (err) {
      setError(getErrorMessage(err, t("combo.deleteFailed")));
    }
  };

  const handleSortPreset = async (comboName: string, preset: string) => {
    try {
      await cmd.applyComboSortPreset(comboName, preset);
      setCombos(await cmd.listModelCombos());
    } catch (err) {
      setError(getErrorMessage(err, t("combo.sortPresetFailed")));
    }
  };

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    if (!editing) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = editing.steps.findIndex((_, i) => `step-${i}` === active.id);
    const newIndex = editing.steps.findIndex((_, i) => `step-${i}` === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const steps = [...editing.steps];
    const [moved] = steps.splice(oldIndex, 1);
    steps.splice(newIndex, 0, moved);
    setEditing({ ...editing, steps });
  };

  if (loading) {
    return <div className="loading">{t("combo.loading")}</div>;
  }

  return (
    <div className={embedded ? "combo-page-embedded" : "page combo-page"}>
      {!embedded && (
        <div className="page-header page-hero">
          <div className="page-hero-head">
            <div className="page-hero-copy">
              <h1>{t("combo.title")}</h1>
              <p className="page-subtitle">{t("combo.subtitle")}</p>
            </div>
            <div className="page-hero-actions">
              <button onClick={() => setActiveTab("templates")} className="btn-secondary">
                {t("settings.templates")}
              </button>
              <button onClick={openCreate} className="btn-primary">
                {t("combo.createCombo")}
              </button>
            </div>
          </div>
          <div className="hero-metrics">
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("settings.combos")}</span>
              <strong className="hero-metric-value">{allCombos.length}</strong>
            </div>
            <div className="hero-metric-card">
              <span className="hero-metric-label">{t("settings.templates")}</span>
              <strong className="hero-metric-value">{templates.length}</strong>
            </div>
          </div>
        </div>
      )}

      {/* Quick Strategy Toolbar */}
      {!embedded && (
        <div className="quick-strategy-toolbar">
          <div className="quick-strategy-label">{t("combo.quickStrategy")}:</div>
          <div className="quick-strategy-buttons">
            <button 
              className="quick-strategy-btn stable-first" 
              onClick={() => applyQuickStrategy("stable_first")}
              title={t("combo.stableFirstDesc")}
            >
              {t("combo.stableFirst")}
            </button>
            <button 
              className="quick-strategy-btn cost-first" 
              onClick={() => applyQuickStrategy("cost_first")}
              title={t("combo.costFirstDesc")}
            >
              {t("combo.costFirst")}
            </button>
            <button 
              className="quick-strategy-btn speed-first" 
              onClick={() => applyQuickStrategy("speed_first")}
              title={t("combo.speedFirstDesc")}
            >
              {t("combo.speedFirst")}
            </button>
            <button 
              className="quick-strategy-btn tools-first" 
              onClick={() => applyQuickStrategy("tools_first")}
              title={t("combo.toolsFirstDesc")}
            >
              {t("combo.toolsFirst")}
            </button>
          </div>
        </div>
      )}

      {error && <div className="error-msg">{error}</div>}

      {!embedded && (
        <div className="tab-bar">
          <button className={activeTab === "combos" ? "active" : ""} onClick={() => setActiveTab("combos")}>
            {t("settings.combos")}
          </button>
          <button className={activeTab === "templates" ? "active" : ""} onClick={() => setActiveTab("templates")}>
            {t("settings.templates")}
          </button>
        </div>
      )}

      {activeTab === "combos" && (
        <div className="tab-content">
          <div className="section-heading">
            <h2>{t("combo.localCombos")}</h2>
            <p className="hint">{t("combo.hint")}</p>
          </div>

          {syncMsg && <div className="info-msg">{syncMsg}</div>}

          <div className="form-row combo-toolbar">
            {auth.is_logged_in && (
              <div className="combo-sync-actions">
                <button
                  onClick={async () => {
                    setSyncing(true);
                    setSyncMsg(t("combo.syncing"));
                    try {
                      const cloudItems = await cmd.fetchCloudCombos();
                      setCloudCombos((cloudItems || []).map((cc: Record<string, any>) => ({
                        combo_id: cc.combo_id,
                        name: cc.logical_name || cc.display_name || cc.combo_id,
                        logical_name: cc.logical_name,
                        display_name: cc.display_name,
                        description: cc.description,
                        tags: cc.tags ? (typeof cc.tags === 'string' ? JSON.parse(cc.tags) : cc.tags) : [],
                        steps: (cc.steps || []).map((s: Record<string, any>) => ({
                          channel_id: s.channel_id,
                          model: s.model,
                          step_role: s.step_role,
                        })),
                        strategy: cc.strategy || "fallback",
                        sticky_uses: cc.sticky_uses || 1,
                        status: cc.status,
                        source: "cloud",
                        version: cc.version,
                      })));
                      setSyncMsg(t("combo.synced"));
                    } catch {
                      setSyncMsg(t("combo.syncFailed"));
                    }
                    setTimeout(() => { setSyncMsg(""); setSyncing(false); }, 3000);
                  }}
                  disabled={syncing}
                  className="btn-secondary btn-sm"
                >
                  {syncing ? t("combo.syncing") : t("combo.syncFromCloud")}
                </button>
              </div>
            )}
          </div>

          {allCombos.length === 0 && <div className="empty-state">{t("combo.noCombos")}</div>}

          {allCombos.length > 0 && (
            <div className="combo-list">
              {allCombos.map((combo) => (
                <div key={`${combo.source || "local"}-${combo.name}`} className="combo-card">
                  <div className="combo-card-main">
                    <div className="combo-card-content">
                      <div className="combo-header">
                        <h3>{combo.display_name || combo.name}</h3>
                        <span className="combo-strategy badge">{combo.strategy}</span>
                        {combo.source && combo.source !== "local" && (
                          <span className={`badge badge-${combo.source}`}>{combo.source}</span>
                        )}
                      </div>
                      {combo.description && <p className="combo-desc">{combo.description}</p>}
                      <div className="combo-models">
                        {(combo.steps || []).map((step, index) => (
                          <span key={`${combo.name}-${index}`} className="model-tag">
                            {step.channel_id ? `${step.channel_id} / ${step.model}` : step.model}
                          </span>
                        ))}
                      </div>
                      <div className="combo-meta-row">
                        <span>{t("combo.steps", { count: combo.steps?.length || 0 })}</span>
                        <span>{t("combo.stickyUses")}：{combo.sticky_uses}</span>
                        {combo.version && <span>v{combo.version}</span>}
                      </div>
                    </div>
                    <div className="combo-card-side">
                      <div className="combo-actions-row">
                        {combo.source !== "cloud" && (
                          <button onClick={() => openEdit(combo)} className="btn-secondary btn-sm">
                            {t("channel.edit")}
                          </button>
                        )}
                        <button onClick={() => handleDelete(combo.name)} className="btn-danger btn-sm">
                          {t("channel.delete")}
                        </button>
                      </div>
                      <div className="combo-card-chevron" aria-hidden="true">›</div>
                    </div>
                  </div>
                  {combo.strategy === "fallback" && (combo.steps?.length || 0) > 1 && combo.source !== "cloud" && (
                    <div className="combo-card-footer combo-sort-presets">
                      <span className="sort-preset-label">{t("combo.sortPreset")}：</span>
                      <button onClick={() => handleSortPreset(combo.name, "intelligence")} className="btn-outline btn-xs">
                        <span className="preset-icon preset-icon-intelligence" />
                        {t("combo.sortPresetIntelligence")}
                      </button>
                      <button onClick={() => handleSortPreset(combo.name, "speed")} className="btn-outline btn-xs">
                        <span className="preset-icon preset-icon-speed" />
                        {t("combo.sortPresetSpeed")}
                      </button>
                      <button onClick={() => handleSortPreset(combo.name, "budget")} className="btn-outline btn-xs">
                        <span className="preset-icon preset-icon-budget" />
                        {t("combo.sortPresetBudget")}
                      </button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {activeTab === "templates" && (
        <div className="tab-content">
          <div className="section-heading">
            <h2>{t("template.title")}</h2>
            <p className="hint">{t("template.cloudHint")}</p>
          </div>
          {templateError && <div className="error-msg">{templateError}</div>}

          {!auth.is_logged_in ? (
            <div className="empty-state">{t("template.loginRequired")}</div>
          ) : (
            <>
              <div className="form-row">
                <div className="form-group">
                  <label>{t("template.searchPlaceholder")}</label>
                  <input value={templateSearch} onChange={(e) => setTemplateSearch(e.target.value)} placeholder={t("template.searchPlaceholder")} />
                </div>
                <div className="form-group combo-filter-action">
                  <label>&nbsp;</label>
                  <button
                    onClick={() => {
                      setTemplateSearch("");
                      void loadPageData();
                    }}
                    className="btn-secondary"
                  >
                    {t("template.refresh")}
                  </button>
                </div>
              </div>

              {templates.length === 0 && <div className="empty-state">{t("template.noTemplates")}</div>}
              {templates.length > 0 && templateResults.length === 0 && <div className="empty-state">{t("template.noMatch")}</div>}

              {templateResults.length > 0 && (
                <div className="combo-list">
                  {templateResults.map((template) => (
                    <div key={template.name} className="combo-card">
                      <div className="combo-card-main">
                        <div className="combo-card-content">
                          <div className="combo-header">
                            <h3>{template.name}</h3>
                            <span className="combo-strategy badge">{template.strategy}</span>
                          </div>
                          {template.description && <p className="combo-desc">{template.description}</p>}
                          {template.tags?.length ? (
                            <div className="combo-models">
                              {template.tags.map((tag) => (
                                <span key={`${template.name}-${tag}`} className="badge badge-blue">
                                  {tag}
                                </span>
                              ))}
                            </div>
                          ) : null}
                          <div className="combo-models">
                            {(template.steps || []).map((step, index) => (
                              <span key={`${template.name}-step-${index}`} className="model-tag">
                                {step.channel_id ? `${step.channel_id} / ${step.model}` : step.model}
                              </span>
                            ))}
                          </div>
                          <div className="combo-meta-row">
                            <span>{t("combo.steps", { count: template.steps?.length || 0 })}</span>
                            <span>{t("combo.stickyUses")}：{template.sticky_uses || 1}</span>
                          </div>
                        </div>
                        <div className="combo-card-side">
                          <div className="combo-actions-row">
                            <button onClick={() => openFromTemplate(template)} className="btn-primary btn-sm">
                              {t("template.apply")}
                            </button>
                          </div>
                          <div className="combo-card-chevron" aria-hidden="true">›</div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      )}

      {editing && (
        <div className="modal-overlay">
          <div className="modal combo-editor-modal">
            <h2>{editingSource === "edit" ? t("combo.editCombo") : t("combo.createComboTitle")}</h2>

            <div className="form-group">
              <label>{t("combo.comboName")}</label>
              <input value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} placeholder={t("combo.comboNamePlaceholder")} />
            </div>

            <div className="form-group">
              <label>{t("combo.description")}</label>
              <textarea value={editing.description || ""} onChange={(e) => setEditing({ ...editing, description: e.target.value })} placeholder={t("combo.descriptionPlaceholder")} />
            </div>

            <div className="form-group">
              <label>{t("combo.tags")}</label>
              <input value={tagsInput} onChange={(e) => setTagsInput(e.target.value)} placeholder={t("combo.tagsPlaceholder")} />
            </div>

            <div className="form-row">
              <div className="form-group">
                <label>{t("combo.strategy")}</label>
                <select value={editing.strategy} onChange={(e) => setEditing({ ...editing, strategy: e.target.value })}>
                  <option value="fallback">fallback</option>
                  <option value="round_robin">round_robin</option>
                </select>
              </div>
              <div className="form-group">
                <label>{t("combo.stickyUses")}</label>
                <input
                  type="number"
                  min={1}
                  value={editing.sticky_uses}
                  onChange={(e) => setEditing({ ...editing, sticky_uses: Number(e.target.value) || 1 })}
                />
              </div>
            </div>

            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
              <SortableContext items={editing.steps.map((_, i) => `step-${i}`)} strategy={verticalListSortingStrategy}>
                <div className="combo-step-list">
                  <div className="section-title">{t("combo.stepChain")}</div>
                  {editing.steps.map((step, index) => (
                    <SortableStepCard key={`step-${index}`} id={`step-${index}`} step={step} index={index} channels={channels} onUpdate={updateStep} onRemove={removeStep} onUpdateMeta={updateStepMeta} onAddProvider={addStepProvider} onRemoveProvider={removeStepProvider} onToggleChannel={toggleStepChannel} t={t} />
                  ))}
                </div>
              </SortableContext>
            </DndContext>

            <button onClick={addStep} className="btn-secondary">
              {t("combo.addStep")}
            </button>

            <div className="modal-actions">
              <button onClick={closeEditor} className="btn-secondary">
                {t("combo.cancel")}
              </button>
              <button onClick={handleSave} className="btn-primary" disabled={saving}>
                {saving ? t("combo.saving") : t("combo.save")}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function SortableStepCard({ id, step, index, channels, onUpdate, onRemove, onUpdateMeta, onAddProvider, onRemoveProvider, onToggleChannel, t }: {
  id: string;
  step: ModelComboStep;
  index: number;
  channels: Channel[];
  onUpdate: (index: number, key: "channel_id" | "model", value: string) => void;
  onRemove: (index: number) => void;
  onUpdateMeta: (index: number, key: string, value: any) => void;
  onAddProvider: (stepIndex: number, provider: { channel_id: string; model: string; priority: number }) => void;
  onRemoveProvider: (stepIndex: number, providerIndex: number) => void;
  onToggleChannel: (stepIndex: number, channelId: string) => void;
  t: (key: string, vars?: any) => string;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  const roleLabels = [t("combo.stepRolePrimary"), t("combo.stepRoleBackup"), t("combo.stepRoleLastResort")];
  const roleHints = [t("combo.stepHintPrimary"), t("combo.stepHintBackup"), t("combo.stepHintLastResort")];
  const roleColors = ["role-primary", "role-backup", "role-last-resort"];
  const roleLabel = index < roleLabels.length ? roleLabels[index] : t("combo.stepRoleStep", { n: index + 1 });
  const roleHint = index < roleHints.length ? roleHints[index] : "";
  const roleColor = index < roleColors.length ? roleColors[index] : "";

  const [showProviders, setShowProviders] = useState(false);
  const [showChannels, setShowChannels] = useState(false);
  const [newProviderChannel, setNewProviderChannel] = useState("");
  const [newProviderModel, setNewProviderModel] = useState("");
  const [newProviderPriority, setNewProviderPriority] = useState(0);
  const [newProviderError, setNewProviderError] = useState("");

  const handleAddProvider = () => {
    if (!newProviderChannel) {
      setNewProviderError(t("combo.selectChannelFirst"));
      return;
    }
    onAddProvider(index, {
      channel_id: newProviderChannel,
      model: newProviderModel || step.model,
      priority: newProviderPriority,
    });
    setNewProviderChannel("");
    setNewProviderModel("");
    setNewProviderPriority(0);
    setNewProviderError("");
  };

  const providers = step.providers || [];
  const stepChannels = step.channels || [];

  return (
    <div ref={setNodeRef} style={style} className="combo-step-card">
      <div className="combo-step-drag-handle" {...attributes} {...listeners}>
        ⠿
      </div>
      <div className="combo-step-index">
        <span className={`step-role-badge ${roleColor}`}>{roleLabel}</span>
        {roleHint && <span className="step-role-hint">{roleHint}</span>}
      </div>
      <div className="combo-step-fields">
        <div className="form-group">
          <label>{t("combo.autoMatchChannel")}</label>
          <select value={step.channel_id || ""} onChange={(e) => onUpdate(index, "channel_id", e.target.value)}>
            <option value="">{t("combo.autoMatch")}</option>
            {channels.map((channel) => (
              <option key={channel.channel_id} value={channel.channel_id}>
                {channel.display_name || channel.channel_id}
              </option>
            ))}
          </select>
        </div>
        <div className="form-group">
          <label>{t("combo.modelName")}</label>
          <input value={step.model} onChange={(e) => onUpdate(index, "model", e.target.value)} placeholder={t("combo.modelPlaceholder")} />
        </div>
      </div>

      <div className="combo-step-advanced" style={{ marginTop: 8 }}>
        <div className="form-row" style={{ gap: 8, alignItems: "center", marginBottom: 8 }}>
          <label className="checkbox-label" style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 12 }}>
            <input
              type="checkbox"
              checked={!!step.allow_provider_fallback}
              onChange={(e) => onUpdateMeta(index, "allow_provider_fallback", e.target.checked)}
            />
            <span>{t("combo.allowProviderFallback")}</span>
          </label>
          <label className="checkbox-label" style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 12 }}>
            <input
              type="checkbox"
              checked={!!step.allow_cross_provider_fallback}
              onChange={(e) => onUpdateMeta(index, "allow_cross_provider_fallback", e.target.checked)}
            />
            <span>{t("combo.allowCrossProviderFallback")}</span>
          </label>
          <select
            value={step.selection_policy || ""}
            onChange={(e) => onUpdateMeta(index, "selection_policy", e.target.value)}
            style={{ width: "auto", marginLeft: 8, fontSize: 12, padding: "2px 4px" }}
          >
            <option value="">{t("combo.selectionDefault")}</option>
            <option value="stability">{t("combo.selectionStability")}</option>
            <option value="cost">{t("combo.selectionCost")}</option>
            <option value="latency">{t("combo.selectionLatency")}</option>
            <option value="throughput">{t("combo.selectionThroughput")}</option>
          </select>
        </div>

        <div className="form-row" style={{ gap: 4, marginBottom: 4 }}>
          <button
            className="btn-outline btn-xs"
            onClick={() => { setShowProviders(!showProviders); setShowChannels(false); }}
          >
            {showProviders ? t("combo.providersCollapse") : t("combo.providersExpand", { count: providers.length })}
          </button>
          <button
            className="btn-outline btn-xs"
            onClick={() => { setShowChannels(!showChannels); setShowProviders(false); }}
          >
            {showChannels ? t("combo.channelsCollapse") : t("combo.channelsExpand", { count: stepChannels.length })}
          </button>
        </div>

        {showProviders && (
          <div className="candidate-provider-list" style={{ marginTop: 4 }}>
            {newProviderError && <div className="error-msg" style={{ fontSize: 11, margin: "2px 0" }}>{newProviderError}</div>}
            <div className="form-row" style={{ gap: 4, flexWrap: "wrap" }}>
              <select
                value={newProviderChannel}
                onChange={(e) => setNewProviderChannel(e.target.value)}
                style={{ flex: 1, minWidth: 120, fontSize: 12 }}
              >
                <option value="">{t("combo.selectChannel")}</option>
                {channels
                  .filter((ch) => !(providers as Array<{channel_id: string; model: string; priority: number}>).some((p) => p.channel_id === ch.channel_id))
                  .map((ch) => (
                    <option key={ch.channel_id} value={ch.channel_id}>
                      {ch.display_name || ch.channel_id}
                    </option>
                  ))}
              </select>
              <input
                type="text"
                value={newProviderModel}
                onChange={(e) => setNewProviderModel(e.target.value)}
                placeholder={t("combo.providerModelOptional")}
                style={{ flex: 1, minWidth: 100, fontSize: 12 }}
              />
              <input
                type="number"
                value={newProviderPriority}
                onChange={(e) => setNewProviderPriority(Number(e.target.value))}
                placeholder={t("combo.providerPriority")}
                style={{ width: 72, fontSize: 12 }}
              />
              <button className="btn-primary btn-xs" onClick={handleAddProvider}>+</button>
            </div>
            {providers.length > 0 && (
              <div className="provider-list" style={{ marginTop: 4 }}>
                {(providers as Array<{channel_id: string; model: string; priority: number}>).map((p, pIdx) => {
                  const ch = channels.find((c) => c.channel_id === p.channel_id);
                  return (
                    <div key={pIdx} className="provider-item" style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 12, padding: "2px 0" }}>
                      <span className="badge" style={{ fontSize: 10, background: "#e9ecef", padding: "1px 4px", borderRadius: 3 }}>P{p.priority}</span>
                      <span>{ch?.display_name || p.channel_id}</span>
                      {p.model && <span className="text-muted" style={{ color: "#888", fontSize: 11 }}>→ {p.model}</span>}
                      <button className="btn-danger btn-xs" onClick={() => onRemoveProvider(index, pIdx)} style={{ marginLeft: "auto" }}>×</button>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}

        {showChannels && (
          <div className="channel-toggle-list" style={{ marginTop: 4 }}>
            <div className="form-row" style={{ gap: 4, flexWrap: "wrap" }}>
              {channels.map((ch) => {
                const selected = stepChannels.includes(ch.channel_id);
                return (
                  <button
                    key={ch.channel_id}
                    className={`btn-xs ${selected ? "btn-primary" : "btn-outline"}`}
                    onClick={() => onToggleChannel(index, ch.channel_id)}
                    style={{ fontSize: 11 }}
                  >
                    {ch.display_name || ch.channel_id}
                  </button>
                );
              })}
            </div>
          </div>
        )}
      </div>

      <button onClick={() => onRemove(index)} className="btn-danger btn-sm">
        {t("channel.delete")}
      </button>
    </div>
  );
}
