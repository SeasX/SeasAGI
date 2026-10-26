import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import * as cmd from "../utils/commands";
import { getErrorMessage } from "../utils/errors";
import { useTranslation } from "../i18n";
import type { MITMAction, MITMRoutingConfig, MITMRoutingRule, ModelCombo, RequestLog } from "../utils/types";

interface MITMTarget {
  id: string;
  name: string;
  icon?: string;
  hosts?: string[];
  endpoint_patterns?: string[];
  default_models?: { id: string; name: string }[];
}

const DATA_POLICY_KEYS: Record<string, string> = {
  any: "mitmRouting.dataPolicyAny",
  local_only: "mitmRouting.dataPolicyLocalOnly",
  no_train: "mitmRouting.dataPolicyNoTrain",
};

const ACTION_KEYS: Record<MITMAction, string> = {
  passthrough: "mitmRouting.actionPassthrough",
  combo: "mitmRouting.actionCombo",
  model: "mitmRouting.actionModel",
};

const emptyRule = (): MITMRoutingRule => ({
  id: "",
  enabled: true,
  priority: 0,
  target_id: "",
  endpoint_path: "",
  request_model: "",
  action: "combo",
  combo_name: "",
  model: "",
});

// 与后端 matchMITMModel 对齐：后缀 `*` 前缀通配，否则等值匹配。
function matchModel(pattern: string, model: string): boolean {
  if (!pattern) return true;
  if (pattern.endsWith("*")) return model.startsWith(pattern.slice(0, -1));
  return pattern === model;
}

// 与后端 mitmRuleMatches 对齐：三维全部满足才命中（target_id 大小写不敏感）。
function ruleMatches(rule: MITMRoutingRule, target: string, path: string, model: string): boolean {
  if (rule.target_id && rule.target_id !== "*" && rule.target_id.toLowerCase() !== target.toLowerCase()) return false;
  if (rule.endpoint_path && !path.startsWith(rule.endpoint_path)) return false;
  if (rule.request_model && !matchModel(rule.request_model, model)) return false;
  return true;
}

// 按 priority 升序（小者优先，同级保持配置顺序）取首条命中规则；无命中则用默认动作。
function resolveRoute(cfg: MITMRoutingConfig, target: string, path: string, model: string) {
  if (!cfg.enabled) return { rule: null as MITMRoutingRule | null, action: "passthrough" as MITMAction, resolved: "" };
  const ordered = [...cfg.rules]
    .map((rule, index) => ({ rule, index }))
    .filter(({ rule }) => rule.enabled)
    .sort((a, b) => {
      // 与后端 sortedMITMRules 对齐：本地规则优先、企业下发规则置底，再按 priority 升序。
      const ea = a.rule.source === "enterprise" ? 1 : 0;
      const eb = b.rule.source === "enterprise" ? 1 : 0;
      if (ea !== eb) return ea - eb;
      return (a.rule.priority - b.rule.priority) || (a.index - b.index);
    })
    .map(({ rule }) => rule);
  for (const rule of ordered) {
    if (ruleMatches(rule, target, path, model)) {
      return {
        rule,
        action: rule.action,
        resolved: rule.action === "combo" ? rule.combo_name || "" : rule.action === "model" ? rule.model || "" : "",
      };
    }
  }
  return {
    rule: null,
    action: cfg.default_action,
    resolved: cfg.default_action === "combo" ? cfg.default_combo_name || "" : cfg.default_action === "model" ? cfg.default_model || "" : "",
  };
}

// 清理空约束，避免向后端写入空串/空对象。
function cleanConstraints(constraints?: Record<string, any>): Record<string, any> | undefined {
  if (!constraints) return undefined;
  const out: Record<string, any> = {};
  for (const [k, v] of Object.entries(constraints)) {
    if (v === "" || v === null || v === undefined) continue;
    out[k] = v;
  }
  return Object.keys(out).length ? out : undefined;
}

function describeMatch(
  rule: MITMRoutingRule,
  targetName: (id: string) => string,
  t: (key: string, params?: Record<string, string | number>) => string,
): string {
  const parts: string[] = [];
  parts.push(rule.target_id && rule.target_id !== "*" ? targetName(rule.target_id) : t("mitmRouting.matchAnyTool"));
  if (rule.endpoint_path) parts.push(t("mitmRouting.matchPath", { path: rule.endpoint_path }));
  if (rule.request_model) {
    parts.push(
      rule.request_model.includes("*")
        ? t("mitmRouting.matchModelPrefix", { model: rule.request_model })
        : t("mitmRouting.matchModelEqual", { model: rule.request_model }),
    );
  }
  return parts.join(" · ");
}

// P3-18：判断两条规则的匹配范围是否可能重叠（三者全部可能重叠才算冲突）。
function dimOverlap(a: string, b: string): boolean {
  const A = (a || "").trim();
  const B = (b || "").trim();
  if (!A || A === "*" || !B || B === "*") return true;
  const prefix = (s: string) => (s.endsWith("*") ? s.slice(0, -1) : s);
  const na = prefix(A);
  const nb = prefix(B);
  return na.startsWith(nb) || nb.startsWith(na);
}

function targetOverlap(a: string, b: string): boolean {
  const A = (a || "").trim().toLowerCase();
  const B = (b || "").trim().toLowerCase();
  if (!A || A === "*" || !B || B === "*") return true;
  return A === B;
}

function ruleScopeOverlap(a: MITMRoutingRule, b: MITMRoutingRule): boolean {
  return targetOverlap(a.target_id || "", b.target_id || "")
    && dimOverlap(a.endpoint_path || "", b.endpoint_path || "")
    && dimOverlap(a.request_model || "", b.request_model || "");
}

// 返回「规则下标 → 与之范围重叠的其他规则下标列表」（仅启用规则）。
function findConflicts(rules: MITMRoutingRule[]): Record<number, number[]> {
  const out: Record<number, number[]> = {};
  for (let i = 0; i < rules.length; i++) {
    if (!rules[i].enabled) continue;
    for (let j = 0; j < rules.length; j++) {
      if (i === j || !rules[j].enabled) continue;
      if (ruleScopeOverlap(rules[i], rules[j])) {
        (out[i] = out[i] || []).push(j);
      }
    }
  }
  return out;
}

export function MITMRoutingPanel() {
  const { t } = useTranslation();
  const [config, setConfig] = useState<MITMRoutingConfig | null>(null);
  const [targets, setTargets] = useState<MITMTarget[]>([]);
  const [combos, setCombos] = useState<ModelCombo[]>([]);
  const [enterpriseRules, setEnterpriseRules] = useState<MITMRoutingRule[]>([]);
  const [fetchingEnt, setFetchingEnt] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  // 预览模拟器输入
  const [simTarget, setSimTarget] = useState("");
  const [simModel, setSimModel] = useState("");
  const [simPath, setSimPath] = useState("");
  // P3-15/P3-19：接管日志（用于实时命中统计与「未命中」聚合）
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [searchParams] = useSearchParams();
  const prefillDone = useRef(false);

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }));

  const load = async () => {
    setLoading(true);
    setErr("");
    try {
      const [cfg, tgt, comboItems, entRules, logItems] = await Promise.all([
        cmd.getMITMRoutingConfig(),
        cmd.getMITMTargets().catch(() => [] as Record<string, any>[]),
        cmd.listModelCombos().catch(() => [] as ModelCombo[]),
        cmd.getMITMEnterpriseRules().catch(() => [] as Record<string, any>[]),
        cmd.listLogs(500, 0).catch(() => [] as RequestLog[]),
      ]);
      if (!cfg.rules.length && cfg.default_action === "passthrough") cfg.rules = [];
      setConfig(cfg);
      setTargets((tgt || []) as MITMTarget[]);
      setCombos(comboItems || []);
      setEnterpriseRules((entRules || []) as MITMRoutingRule[]);
      setLogs((logItems || []) as RequestLog[]);
    } catch (e) {
      setErr(getErrorMessage(e, t("mitmRouting.loadFailed")));
    } finally {
      setLoading(false);
    }
  };

  // P3-16：从日志页「创建映射规则」跳转时，按 query 预填一条新规则。
  useEffect(() => {
    if (prefillDone.current || !config) return;
    const ruleTarget = searchParams.get("rule_target") || "";
    const ruleModel = searchParams.get("rule_model") || "";
    const ruleEndpoint = searchParams.get("rule_endpoint") || "";
    if (!ruleTarget && !ruleModel && !ruleEndpoint) return;
    prefillDone.current = true;
    const rule: MITMRoutingRule = {
      ...emptyRule(),
      id: `rule-${Date.now()}`,
      target_id: ruleTarget,
      request_model: ruleModel,
      endpoint_path: ruleEndpoint,
      action: "combo",
    };
    setConfig((c) => (c ? { ...c, rules: [...c.rules, rule] } : c));
    setMsg(t("mitmRouting.prefillNotice"));
    window.setTimeout(() => setMsg(""), 4000);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config, searchParams]);

  // P2-12：从企业服务端拉取下发的路由规则（只读、置底）。
  const fetchEnterprise = async () => {
    setFetchingEnt(true);
    setErr("");
    setMsg("");
    try {
      const res = await cmd.fetchMITMRoutingFromEnterprise();
      if (res && res.ok === false) {
        setErr(
          t("mitmRouting.enterpriseFetchFailedDetail", {
            error: res.error || t("mitmRouting.unknownError"),
          }),
        );
        return;
      }
      const ent = await cmd.getMITMEnterpriseRules().catch(() => [] as Record<string, any>[]);
      setEnterpriseRules((ent || []) as MITMRoutingRule[]);
      setMsg(t("mitmRouting.enterpriseFetchSuccess", { count: ent?.length || 0 }));
      window.setTimeout(() => setMsg(""), 3000);
    } catch (e) {
      setErr(getErrorMessage(e, t("mitmRouting.enterpriseRulesLoadFailed")));
    } finally {
      setFetchingEnt(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const targetName = (id: string) => targets.find((target) => target.id === id)?.name || id;

  const updateRule = (index: number, patch: Partial<MITMRoutingRule>) => {
    if (!config) return;
    const rules = config.rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule));
    setConfig({ ...config, rules });
  };

  const addRule = () => {
    if (!config) return;
    setConfig({ ...config, rules: [...config.rules, { ...emptyRule(), id: `rule-${Date.now()}` }] });
  };

  const removeRule = (index: number) => {
    if (!config) return;
    setConfig({ ...config, rules: config.rules.filter((_, i) => i !== index) });
  };

  const handleDragEnd = (event: DragEndEvent) => {
    if (!config) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const from = config.rules.findIndex((_, i) => `rule-${i}` === active.id);
    const to = config.rules.findIndex((_, i) => `rule-${i}` === over.id);
    if (from === -1 || to === -1) return;
    const rules = [...config.rules];
    const [moved] = rules.splice(from, 1);
    rules.splice(to, 0, moved);
    setConfig({ ...config, rules });
  };

  const handleSave = async () => {
    if (!config) return;
    // 优先级按列表顺序归一化（后端按 priority 升序匹配）。
    const normalized: MITMRoutingConfig = {
      ...config,
      default_combo_name: config.default_action === "combo" ? (config.default_combo_name || "").trim() : "",
      default_model: config.default_action === "model" ? (config.default_model || "").trim() : "",
      rules: config.rules.map((rule, index) => {
        const constraints = cleanConstraints(rule.constraints);
        return {
          ...rule,
          priority: index,
          target_id: (rule.target_id || "").trim(),
          endpoint_path: (rule.endpoint_path || "").trim(),
          request_model: (rule.request_model || "").trim(),
          combo_name: rule.action === "combo" ? (rule.combo_name || "").trim() : "",
          model: rule.action === "model" ? (rule.model || "").trim() : "",
          constraints,
        };
      }),
    };
    setSaving(true);
    setErr("");
    setMsg("");
    try {
      await cmd.setMITMRoutingConfig(normalized);
      setConfig(normalized);
      setMsg(t("mitmRouting.saved"));
      window.setTimeout(() => setMsg(""), 3000);
    } catch (e) {
      setErr(getErrorMessage(e, t("mitmRouting.saveFailed")));
    } finally {
      setSaving(false);
    }
  };

  const defaultResolved = (cfg: MITMRoutingConfig) =>
    cfg.default_action === "combo" ? cfg.default_combo_name : cfg.default_action === "model" ? cfg.default_model : "";

  // P4-25：预览/概览纳入企业下发规则（企业置底）。
  const effectiveConfig = useMemo<MITMRoutingConfig | null>(() => {
    if (!config) return config;
    if (!enterpriseRules.length) return config;
    return {
      ...config,
      rules: [...config.rules, ...enterpriseRules.map((r) => ({ ...r, source: "enterprise" as const }))],
    };
  }, [config, enterpriseRules]);

  const simulation = useMemo(
    () => (effectiveConfig ? resolveRoute(effectiveConfig, simTarget, simPath, simModel) : null),
    [effectiveConfig, simTarget, simPath, simModel],
  );

  // P3-18：潜在冲突
  const conflicts = useMemo(() => findConflicts(config?.rules || []), [config]);

  // P3-15/P3-19：从接管日志派生的实时命中统计与未命中聚合
  const mitmLogs = useMemo(() => logs.filter((l) => !!l.mitm_target), [logs]);
  const routeStats = useMemo(() => {
    const m = new Map<string, RequestLog>();
    for (const l of mitmLogs) {
      const key = l.mitm_target as string;
      const prev = m.get(key);
      if (!prev || l.created_at > prev.created_at) m.set(key, l);
    }
    return m;
  }, [mitmLogs]);
  const unmatched = useMemo(() => mitmLogs.filter((l) => !l.resolved_target), [mitmLogs]);

  // P3-20：预览命中 Combo 时的步骤（拓扑图展开）
  const simCombo = useMemo(
    () =>
      simulation && simulation.action === "combo" && simulation.resolved
        ? combos.find((c) => c.name === simulation.resolved) || null
        : null,
    [simulation, combos],
  );

  if (loading) return <div className="tab-content section-card"><div className="loading">{t("mitmRouting.loading")}</div></div>;
  if (!config) return <div className="tab-content section-card"><div className="error-msg">{err || t("mitmRouting.configUnavailable")}</div></div>;

  return (
    <div className="settings-stack">
      {/* 全局开关与默认动作 */}
      <div className="tab-content section-card">
        <div className="section-heading">
          <h2>{t("mitmRouting.title")}</h2>
          <p className="hint">{t("mitmRouting.subtitle")}</p>
        </div>
        {err && <div className="error-msg" style={{ marginTop: 8 }}>{err}</div>}
        {msg && <div className="info-msg" style={{ marginTop: 8 }}>{msg}</div>}

        <label className="checkbox-label" style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 12 }}>
          <input
            type="checkbox"
            checked={config.enabled}
            onChange={(e) => setConfig({ ...config, enabled: e.target.checked })}
            aria-label={t("mitmRouting.enableLabel")}
          />
          <span>{t("mitmRouting.enableLabel")}</span>
        </label>

        <div className="form-row" style={{ marginTop: 12 }}>
          <div className="form-group">
            <label>{t("mitmRouting.defaultActionLabel")}</label>
            <select
              value={config.default_action}
              onChange={(e) => setConfig({ ...config, default_action: e.target.value as MITMAction })}
              aria-label={t("mitmRouting.defaultActionLabel")}
            >
              <option value="passthrough">{t("mitmRouting.defaultActionPassthroughOption")}</option>
              <option value="combo">{t("mitmRouting.defaultActionComboOption")}</option>
              <option value="model">{t("mitmRouting.defaultActionModelOption")}</option>
            </select>
          </div>
          {config.default_action === "combo" && (
            <div className="form-group">
              <label>{t("mitmRouting.defaultComboLabel")}</label>
              <select
                value={config.default_combo_name || ""}
                onChange={(e) => setConfig({ ...config, default_combo_name: e.target.value })}
                aria-label={t("mitmRouting.defaultComboLabel")}
              >
                <option value="">{t("mitmRouting.selectComboPlaceholder")}</option>
                {combos.map((c) => (
                  <option key={c.name} value={c.name}>{c.display_name || c.name}</option>
                ))}
              </select>
            </div>
          )}
          {config.default_action === "model" && (
            <div className="form-group">
              <label>{t("mitmRouting.defaultModelLabel")}</label>
              <input
                value={config.default_model || ""}
                onChange={(e) => setConfig({ ...config, default_model: e.target.value })}
                placeholder={t("mitmRouting.modelPlaceholder")}
                aria-label={t("mitmRouting.defaultModelLabel")}
              />
            </div>
          )}
        </div>

        {/* P2-13：头部一致性校验模式 */}
        <div className="form-row" style={{ marginTop: 12 }}>
          <div className="form-group">
            <label>{t("mitmRouting.securityModeLabel")}</label>
            <select
              value={config.security_mode || "lenient"}
              onChange={(e) => setConfig({ ...config, security_mode: e.target.value as MITMRoutingConfig["security_mode"] })}
              aria-label={t("mitmRouting.securityModeLabel")}
            >
              <option value="lenient">{t("mitmRouting.securityLenient")}</option>
              <option value="strict">{t("mitmRouting.securityStrict")}</option>
            </select>
          </div>
          <div className="form-group">
            <label>{t("mitmRouting.enterpriseRulesLabel")}</label>
            <button type="button" className="btn-secondary" onClick={fetchEnterprise} disabled={fetchingEnt}>
              {fetchingEnt
                ? t("mitmRouting.fetchingEnterprise")
                : t("mitmRouting.fetchEnterprise", { count: enterpriseRules.length })}
            </button>
          </div>
        </div>

        {/* 优先级链提示 */}
        <div className="hint" style={{ marginTop: 12, display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
          <span>{t("mitmRouting.priorityChain")}</span>
          <span className="badge badge-blue">{t("mitmRouting.priorityChainRule")}</span>
          <span>›</span>
          <span className="badge">
            {defaultResolved(config)
              ? t("mitmRouting.priorityChainDefaultWithTarget", {
                  action: t(ACTION_KEYS[config.default_action]),
                  target: defaultResolved(config) || "",
                })
              : t("mitmRouting.priorityChainDefault", { action: t(ACTION_KEYS[config.default_action]) })}
          </span>
          <span>›</span>
          <span className="badge">{t("mitmRouting.priorityChainDirect")}</span>
        </div>
      </div>

      {/* 规则编辑器 */}
      <div className="tab-content section-card">
        <div className="section-heading">
          <h2>{t("mitmRouting.rulesTitle")}</h2>
          <p className="hint">{t("mitmRouting.rulesHint")}</p>
        </div>

        {config.rules.length === 0 && <p className="hint">{t("mitmRouting.noRules")}</p>}

        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={config.rules.map((_, i) => `rule-${i}`)} strategy={verticalListSortingStrategy}>
            <div className="combo-step-list">
              {config.rules.map((rule, index) => (
                <SortableRuleCard
                  key={`rule-${index}`}
                  id={`rule-${index}`}
                  rule={rule}
                  index={index}
                  targets={targets}
                  combos={combos}
                  targetName={targetName}
                  conflicts={conflicts[index] || []}
                  onChange={(patch) => updateRule(index, patch)}
                  onRemove={() => removeRule(index)}
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>

        <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
          <button onClick={addRule} className="btn-secondary">{t("mitmRouting.addRule")}</button>
          <button onClick={handleSave} className="btn-primary" disabled={saving}>
            {saving ? t("mitmRouting.saving") : t("mitmRouting.saveConfig")}
          </button>
        </div>
      </div>

      {/* P2-12：企业下发规则（只读、置底） */}
      <div className="tab-content section-card">
        <div className="section-heading">
          <h2>{t("mitmRouting.enterpriseRulesLabel")}</h2>
          <p className="hint">{t("mitmRouting.enterpriseSectionHint")}</p>
        </div>
        {enterpriseRules.length === 0 ? (
          <p className="hint">{t("mitmRouting.noEnterpriseRules")}</p>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", fontSize: 13, borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ textAlign: "left", borderBottom: "1px solid var(--border, #333)" }}>
                  <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colSourceTool")}</th>
                  <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colEndpointModel")}</th>
                  <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colTarget")}</th>
                  <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colStatus")}</th>
                </tr>
              </thead>
              <tbody>
                {enterpriseRules.map((rule, i) => (
                  <tr key={rule.id || i} style={{ borderBottom: "1px solid var(--border-light, #222)" }}>
                    <td style={{ padding: "4px 8px" }}>
                      {rule.target_id && rule.target_id !== "*" ? targetName(rule.target_id) : t("mitmRouting.any")}
                    </td>
                    <td style={{ padding: "4px 8px" }}>
                      {rule.endpoint_path || "-"} / {rule.request_model || "-"}
                    </td>
                    <td style={{ padding: "4px 8px" }}>
                      {rule.action === "passthrough"
                        ? t("mitmRouting.direct")
                        : t("mitmRouting.actionWithTarget", {
                            action: t(ACTION_KEYS[rule.action]),
                            target: (rule.action === "combo" ? rule.combo_name : rule.model) || "",
                          })}
                    </td>
                    <td style={{ padding: "4px 8px" }}>
                      <span className={`badge ${rule.enabled ? "badge-green" : "badge-yellow"}`}>
                        {rule.enabled ? t("mitmRouting.enabled") : t("mitmRouting.disabled")}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 实时预览 */}
      <div className="tab-content section-card">
        <div className="section-heading">
          <h2>{t("mitmRouting.previewTitle")}</h2>
          <p className="hint">{t("mitmRouting.previewHint")}</p>
        </div>
        <div className="form-row">
          <div className="form-group">
            <label>{t("mitmRouting.sourceToolLabel")}</label>
            <select
              value={simTarget}
              onChange={(e) => setSimTarget(e.target.value)}
              aria-label={t("mitmRouting.sourceToolLabel")}
            >
              <option value="">{t("mitmRouting.anyOption")}</option>
              {targets.map((target) => (
                <option key={target.id} value={target.id}>{target.name}</option>
              ))}
              <option value="__other__">{t("mitmRouting.otherOption")}</option>
            </select>
          </div>
          <div className="form-group">
            <label>{t("mitmRouting.requestModelLabel")}</label>
            <input
              value={simModel}
              onChange={(e) => setSimModel(e.target.value)}
              placeholder={t("mitmRouting.simModelPlaceholder")}
              aria-label={t("mitmRouting.requestModelLabel")}
            />
          </div>
          <div className="form-group">
            <label>{t("mitmRouting.endpointPathLabel")}</label>
            <input
              value={simPath}
              onChange={(e) => setSimPath(e.target.value)}
              placeholder={t("mitmRouting.simPathPlaceholder")}
              aria-label={t("mitmRouting.endpointPathLabel")}
            />
          </div>
        </div>
        {simulation && (
          <div style={{ marginTop: 8, display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
            <span className="hint" style={{ marginBottom: 0 }}>{t("mitmRouting.resultLabel")}</span>
            {simulation.rule ? (
              <span className="badge badge-blue">
                {t("mitmRouting.hitRule", { match: describeMatch(simulation.rule, targetName, t) })}
              </span>
            ) : (
              <span className="badge badge-yellow">{t("mitmRouting.missRule")}</span>
            )}
            <span>→</span>
            <span className={`badge ${simulation.action === "passthrough" ? "badge-green" : "badge-blue"}`}>
              {simulation.resolved
                ? t("mitmRouting.actionWithTarget", {
                    action: t(ACTION_KEYS[simulation.action]),
                    target: simulation.resolved,
                  })
                : t(ACTION_KEYS[simulation.action])}
            </span>
          </div>
        )}
        {/* P3-20：路由拓扑图 */}
        {simulation && (
          <div
            className="hint"
            style={{ marginTop: 12, marginBottom: 0, display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}
          >
            <span className="badge">{simTarget ? targetName(simTarget) : t("mitmRouting.matchAnyTool")}</span>
            <span>--({simModel || t("mitmRouting.anyModel")})--&gt;</span>
            <span className="badge badge-blue">
              {simulation.rule
                ? simulation.rule.source === "enterprise"
                  ? t("mitmRouting.enterpriseRuleBadge")
                  : t("mitmRouting.ruleIndex", { index: config.rules.indexOf(simulation.rule) + 1 })
                : t("mitmRouting.defaultActionBadge")}
            </span>
            <span>--({t("mitmRouting.rewriteStep")})--&gt;</span>
            <span className={`badge ${simulation.action === "passthrough" ? "badge-green" : "badge-blue"}`}>
              {simulation.resolved || t("mitmRouting.directTarget")}
            </span>
            {simCombo && (simCombo.steps?.length || 0) > 0 && (
              <>
                <span>--({t("mitmRouting.distribute")})--&gt;</span>
                <span className="badge">
                  {simCombo.steps!.map((s) => `${s.channel_id || "?"} / ${s.model}`).join(" · ")}
                </span>
              </>
            )}
          </div>
        )}
      </div>

      <MITMOverview config={effectiveConfig ?? config} targets={targets} routeStats={routeStats} unmatched={unmatched} />
    </div>
  );
}

function SortableRuleCard({
  id, rule, index, targets, combos, targetName, conflicts, onChange, onRemove,
}: {
  id: string;
  rule: MITMRoutingRule;
  index: number;
  targets: MITMTarget[];
  combos: ModelCombo[];
  targetName: (id: string) => string;
  conflicts: number[];
  onChange: (patch: Partial<MITMRoutingRule>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  const style = { transform: CSS.Transform.toString(transform), transition, opacity: isDragging ? 0.5 : 1 };

  const wildcardHint = rule.request_model?.includes("*")
    ? t("mitmRouting.wildcardHint", { prefix: rule.request_model.replace(/\*+$/, "") })
    : "";
  // P2-11：端点细分——给出所选工具的端点预设，便于按 API 细分规则。
  const endpointPresets = targets.find((target) => target.id === rule.target_id)?.endpoint_patterns || [];
  const datalistId = `${id}-endpoints`;

  return (
    <div ref={setNodeRef} style={style} className={`combo-step-card ${isDragging ? "is-dragging" : ""}`}>
      <div className="combo-step-drag-handle" {...attributes} {...listeners}>⠿</div>
      <div className="combo-step-index">
        <span className="step-role-badge role-primary">P{index + 1}</span>
        <label className="checkbox-label" style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 12, marginTop: 4 }}>
          <input
            type="checkbox"
            checked={rule.enabled}
            onChange={(e) => onChange({ enabled: e.target.checked })}
            aria-label={t("mitmRouting.enable")}
          />
          <span>{t("mitmRouting.enable")}</span>
        </label>
      </div>

      <div className="combo-step-fields" style={{ flexWrap: "wrap" }}>
        <div className="form-group">
          <label>{t("mitmRouting.sourceToolLabel")}</label>
          <select
            value={rule.target_id || ""}
            onChange={(e) => onChange({ target_id: e.target.value })}
            aria-label={t("mitmRouting.sourceToolLabel")}
          >
            <option value="">{t("mitmRouting.any")}</option>
            <option value="*">{t("mitmRouting.anyStar")}</option>
            {targets.map((target) => (
              <option key={target.id} value={target.id}>{target.name}</option>
            ))}
          </select>
        </div>
        <div className="form-group">
          <label>{t("mitmRouting.endpointPathPrefixLabel")}</label>
          <input
            value={rule.endpoint_path || ""}
            onChange={(e) => onChange({ endpoint_path: e.target.value })}
            placeholder={t("mitmRouting.simPathPlaceholder")}
            aria-label={t("mitmRouting.endpointPathPrefixLabel")}
            list={endpointPresets.length ? datalistId : undefined}
          />
          {endpointPresets.length > 0 && (
            <datalist id={datalistId}>
              {endpointPresets.map((p) => (
                <option key={p} value={p} />
              ))}
            </datalist>
          )}
        </div>
        <div className="form-group">
          <label>{t("mitmRouting.requestModelOptionalLabel")}</label>
          <input
            value={rule.request_model || ""}
            onChange={(e) => onChange({ request_model: e.target.value })}
            placeholder={t("mitmRouting.requestModelPlaceholder")}
            aria-label={t("mitmRouting.requestModelOptionalLabel")}
          />
          {wildcardHint && <span className="hint" style={{ marginBottom: 0, fontSize: 11 }}>{wildcardHint}</span>}
        </div>
        <div className="form-group">
          <label>{t("mitmRouting.actionLabel")}</label>
          <select
            value={rule.action}
            onChange={(e) => onChange({ action: e.target.value as MITMAction })}
            aria-label={t("mitmRouting.actionLabel")}
          >
            <option value="combo">{t("mitmRouting.actionCombo")}</option>
            <option value="model">{t("mitmRouting.actionModel")}</option>
            <option value="passthrough">{t("mitmRouting.actionPassthrough")}</option>
          </select>
        </div>
        {rule.action === "combo" && (
          <div className="form-group">
            <label>{t("mitmRouting.targetComboLabel")}</label>
            <select
              value={rule.combo_name || ""}
              onChange={(e) => onChange({ combo_name: e.target.value })}
              aria-label={t("mitmRouting.targetComboLabel")}
            >
              <option value="">{t("mitmRouting.selectComboPlaceholder")}</option>
              {combos.map((c) => (
                <option key={c.name} value={c.name}>{c.display_name || c.name}</option>
              ))}
            </select>
            {/* P3-17：一键跳转 Combo 详情 */}
            {rule.combo_name && (
              <Link
                to={`/combo-workbench?tab=combos&combo=${encodeURIComponent(rule.combo_name)}`}
                className="hint"
                style={{ marginBottom: 0, fontSize: 11 }}
              >
                {t("mitmRouting.viewComboConfig")}
              </Link>
            )}
          </div>
        )}
        {rule.action === "model" && (
          <div className="form-group">
            <label>{t("mitmRouting.targetModelLabel")}</label>
            <input
              value={rule.model || ""}
              onChange={(e) => onChange({ model: e.target.value })}
              placeholder={t("mitmRouting.modelPlaceholder")}
              aria-label={t("mitmRouting.targetModelLabel")}
            />
          </div>
        )}
        {/* P2-10：随映射注入的请求级约束 */}
        <div className="form-group" style={{ flexBasis: "100%" }}>
          <label>{t("mitmRouting.constraintsLabel")}</label>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <input
              type="number"
              min="0"
              step="0.1"
              style={{ maxWidth: 120 }}
              value={rule.constraints?.max_price ?? ""}
              onChange={(e) =>
                onChange({
                  constraints: { ...(rule.constraints || {}), max_price: e.target.value === "" ? "" : Number(e.target.value) },
                })
              }
              placeholder="max_price"
              aria-label="max_price"
            />
            <input
              type="number"
              min="0"
              step="50"
              style={{ maxWidth: 140 }}
              value={rule.constraints?.max_latency_ms ?? ""}
              onChange={(e) =>
                onChange({
                  constraints: { ...(rule.constraints || {}), max_latency_ms: e.target.value === "" ? "" : Number(e.target.value) },
                })
              }
              placeholder="max_latency_ms"
              aria-label="max_latency_ms"
            />
            <select
              style={{ maxWidth: 170 }}
              value={(rule.constraints?.data_policy as string) || ""}
              onChange={(e) => onChange({ constraints: { ...(rule.constraints || {}), data_policy: e.target.value } })}
              aria-label={t("mitmRouting.dataPolicyLabel")}
            >
              <option value="">{t("mitmRouting.dataPolicyUnset")}</option>
              {Object.entries(DATA_POLICY_KEYS).map(([key, value]) => (
                <option key={key} value={key}>{t(value)}</option>
              ))}
            </select>
          </div>
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 6 }}>
        {conflicts.length > 0 && (
          <span
            className="badge badge-yellow"
            title={t("mitmRouting.conflictTitle", { rules: conflicts.map((i) => i + 1).join(", ") })}
          >
            {t("mitmRouting.conflictBadge", { rules: conflicts.map((i) => i + 1).join(", ") })}
          </span>
        )}
        <button onClick={onRemove} className="btn-danger btn-sm">{t("mitmRouting.delete")}</button>
        <span className="hint" style={{ marginBottom: 0, fontSize: 11, maxWidth: 200, textAlign: "right" }}>
          {t("mitmRouting.matchLabel", { match: describeMatch(rule, targetName, t) })}
        </span>
      </div>
    </div>
  );
}

// MITMOverview（P1-10 / P3-15 / P3-19）：展示「工具 → 当前路由目标 + 最近命中」，
// 并提供未命中规则的聚合视图与日志快捷入口。
function MITMOverview({
  config,
  targets,
  routeStats,
  unmatched,
}: {
  config: MITMRoutingConfig;
  targets: MITMTarget[];
  routeStats: Map<string, RequestLog>;
  unmatched: RequestLog[];
}) {
  const { t } = useTranslation();
  const rows = useMemo(
    () =>
      targets.map((target) => {
        const r = resolveRoute(config, target.id, "", "");
        return { target, ...r };
      }),
    [config, targets],
  );

  const fmtHit = (v?: string) => {
    if (!v) return "-";
    const d = new Date(v);
    return Number.isNaN(d.getTime()) ? v : d.toLocaleString();
  };

  return (
    <div className="tab-content section-card">
      <div className="section-heading">
        <h2>{t("mitmRouting.overviewTitle")}</h2>
        <p className="hint">{t("mitmRouting.overviewHint")}</p>
      </div>
      {!config.enabled && <div className="info-msg">{t("mitmRouting.disabledNotice")}</div>}
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", fontSize: 13, borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ textAlign: "left", borderBottom: "1px solid var(--border, #333)" }}>
              <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colSourceTool")}</th>
              <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colCurrentRoute")}</th>
              <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colLastHit")}</th>
              <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colLastRuleRewrite")}</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr><td colSpan={4} style={{ padding: "8px", color: "var(--text-secondary, #888)" }}>{t("mitmRouting.noTargets")}</td></tr>
            )}
            {rows.map(({ target, rule, action, resolved }) => {
              const hit = routeStats.get(target.id);
              return (
                <tr key={target.id} style={{ borderBottom: "1px solid var(--border-light, #222)" }}>
                  <td style={{ padding: "4px 8px" }}>{target.name}</td>
                  <td style={{ padding: "4px 8px" }}>
                    {action === "passthrough" ? (
                      <span className="badge badge-green">{t("mitmRouting.actionPassthrough")}</span>
                    ) : (
                      <span className="badge badge-blue">
                        {t("mitmRouting.actionWithTarget", {
                          action: t(ACTION_KEYS[action]),
                          target: resolved,
                        })}
                        {rule ? t("mitmRouting.sourceRule") : t("mitmRouting.sourceDefault")}
                      </span>
                    )}
                  </td>
                  <td style={{ padding: "4px 8px", color: "var(--text-secondary, #888)" }}>
                    {hit ? fmtHit(hit.created_at) : "-"}
                  </td>
                  <td style={{ padding: "4px 8px" }}>
                    {!hit ? (
                      <span className="hint" style={{ marginBottom: 0 }}>-</span>
                    ) : hit.resolved_target ? (
                      <span className="badge badge-blue">{hit.resolved_target}</span>
                    ) : (
                      <span className="badge badge-yellow">{t("mitmRouting.notRewritten")}</span>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* P3-19：未命中规则的接管请求聚合 */}
      <div style={{ marginTop: 16 }}>
        <div className="section-heading">
          <h3 style={{ margin: 0 }}>{t("mitmRouting.unmatchedTitle", { count: unmatched.length })}</h3>
          <p className="hint">{t("mitmRouting.unmatchedHint")}</p>
        </div>
        {unmatched.length === 0 ? (
          <p className="hint">{t("mitmRouting.noUnmatched")}</p>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", fontSize: 12, borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ textAlign: "left", borderBottom: "1px solid var(--border, #333)" }}>
                  <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colTime")}</th>
                  <th style={{ padding: "4px 8px" }}>{t("mitmRouting.colSourceTool")}</th>
                  <th style={{ padding: "4px 8px" }}>{t("mitmRouting.requestModelLabel")}</th>
                </tr>
              </thead>
              <tbody>
                {unmatched.slice(0, 5).map((l) => (
                  <tr key={l.request_id} style={{ borderBottom: "1px solid var(--border-light, #222)" }}>
                    <td style={{ padding: "4px 8px", color: "var(--text-secondary, #888)" }}>{fmtHit(l.created_at)}</td>
                    <td style={{ padding: "4px 8px" }}>{l.mitm_target}</td>
                    <td style={{ padding: "4px 8px" }}>{l.requested_model || "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div style={{ marginTop: 12 }}>
        <Link to="/logs?mitm=1" className="btn-secondary btn-sm">{t("mitmRouting.viewMitmLogs")}</Link>
        <span className="hint" style={{ marginLeft: 8, marginBottom: 0 }}>
          {t("mitmRouting.mitmLogsHint")}
        </span>
      </div>
    </div>
  );
}
