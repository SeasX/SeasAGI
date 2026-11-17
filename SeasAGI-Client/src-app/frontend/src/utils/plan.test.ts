import { describe, expect, it } from "vitest";
import type { CloudBilling } from "../utils/types";
import { formatPlanTier, normalizePlanTier } from "../utils/plan";

const billing = (plan_id: string, plan_name = "") =>
  ({ plan_id, plan_name } as CloudBilling);

describe("normalizePlanTier", () => {
  it("matches known plan ids exactly", () => {
    expect(normalizePlanTier(billing("pro"))).toBe("pro");
    expect(normalizePlanTier(billing("teams"))).toBe("teams");
    expect(normalizePlanTier(billing("enterprise"))).toBe("enterprise");
    expect(normalizePlanTier(billing("free"))).toBe("free");
  });

  it("does not treat substrings as paid tiers", () => {
    // 旧实现用 includes，以下值都会被误判为付费档。
    expect(normalizePlanTier(billing("pro-2024-legacy"))).toBeNull();
    expect(normalizePlanTier(billing("enterprise_trial"))).toBeNull();
    expect(normalizePlanTier(billing("my-pro"))).toBeNull();
  });

  it("falls back to an exact plan_name only", () => {
    expect(normalizePlanTier(billing("", "Pro"))).toBe("pro");
    expect(normalizePlanTier(billing("", "pro plus"))).toBeNull();
  });

  it("handles missing billing", () => {
    expect(normalizePlanTier(null)).toBeNull();
    expect(normalizePlanTier(undefined)).toBeNull();
  });
});

describe("formatPlanTier", () => {
  it("renders uppercase labels", () => {
    expect(formatPlanTier(billing("pro"))).toBe("PRO");
    expect(formatPlanTier(billing("teams"))).toBe("TEAMS");
    expect(formatPlanTier(billing("enterprise"))).toBe("ENTERPRISE");
    expect(formatPlanTier(billing("unknown"))).toBe("FREE");
    expect(formatPlanTier(null)).toBe("FREE");
  });
});
