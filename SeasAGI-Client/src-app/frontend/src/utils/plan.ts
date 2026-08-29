import type { CloudBilling, PlanTier } from "./types";

export function normalizePlanTier(billing: CloudBilling | null | undefined): PlanTier | null {
  if (!billing) return null;
  const value = `${billing.plan_id || ""} ${billing.plan_name || ""}`.toLowerCase();
  if (value.includes("enterprise")) return "enterprise";
  if (value.includes("pro")) return "pro";
  if (value.includes("free")) return "free";
  return null;
}

export function formatPlanTier(billing: CloudBilling | null | undefined): string {
  const tier = normalizePlanTier(billing);
  if (tier === "enterprise") return "ENTERPRISE";
  if (tier === "pro") return "PRO";
  return "FREE";
}
