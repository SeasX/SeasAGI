import type { CloudBilling, PlanTier } from "./types";

// 套餐档位只接受精确匹配。旧实现用 includes 做子串匹配，
// 任意包含 "enterprise"/"pro" 的 plan_id 都会被误判为付费档（P2-4）。
// 注意：前端判断只用于展示，真正的鉴权与额度裁决在服务端。
const KNOWN_TIERS: PlanTier[] = ["free", "pro", "teams", "enterprise"];

function exactTier(value: string | undefined | null): PlanTier | null {
  const normalized = (value || "").trim().toLowerCase();
  return KNOWN_TIERS.find((tier) => tier === normalized) ?? null;
}

export function normalizePlanTier(billing: CloudBilling | null | undefined): PlanTier | null {
  if (!billing) return null;
  // plan_id 是权威字段；plan_name 仅在旧数据缺 plan_id 时兜底，且同样精确匹配。
  return exactTier(billing.plan_id) ?? exactTier(billing.plan_name);
}

export function formatPlanTier(billing: CloudBilling | null | undefined): string {
  const tier = normalizePlanTier(billing);
  if (tier === "enterprise") return "ENTERPRISE";
  if (tier === "teams") return "TEAMS";
  if (tier === "pro") return "PRO";
  return "FREE";
}
