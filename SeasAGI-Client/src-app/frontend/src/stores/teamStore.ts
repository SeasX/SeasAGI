import { create } from "zustand";
import { persist } from "zustand/middleware";

export interface TeamMember {
  member_id: string;
  email: string;
  role: "owner" | "admin" | "member";
  status: "active" | "disabled";
  created_at: string;
}

export interface TeamProfile {
  name: string;
  slug: string;
  status: string;
  billing_email: string;
  shared_quota: number;
  quota_used: number;
}

export interface MemberUsage {
  member_id: string;
  email: string;
  role: string;
  requests: number;
  input_tokens: number;
  output_tokens: number;
}

export interface Strategy {
  strategy_id: string;
  name: string;
  strategy_type: string;
  config: string;
  created_at: string;
  updated_at: string;
}

export interface ComboTemplate {
  template_id: string;
  name: string;
  models: string;
  routing_mode: string;
  created_at: string;
  updated_at: string;
}

export interface WebhookItem {
  webhook_id: string;
  name: string;
  url: string;
  events: string;
  secret: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface InviteLink {
  link_id: string;
  token: string;
  role: string;
  expires_at: string | null;
  used_count: number;
  max_uses: number | null;
  created_at: string;
}

export interface BillingInvoice {
  invoice_id: string;
  period_start: string;
  period_end: string;
  amount: number;
  currency: string;
  status: string;
  created_at: string;
}

export interface OverageItem {
  overage_id: string;
  user_id: string;
  plan_id: string;
  billing_period: string;
  overage_requests: number;
  overage_cost: number;
  currency: string;
  billed: number;
  created_at: string;
}

export interface SeatInfo {
  seats: number;
  active_members: number;
  available: number;
}

function generateId(): string {
  return Math.random().toString(36).substring(2, 15) + Date.now().toString(36);
}

function estimateCost(inputTokens: number, outputTokens: number, requests: number): number {
  const inputPricePerM = 2.50;
  const outputPricePerM = 10.00;
  const fixedPerRequest = 0.0001;
  const cost = (inputTokens / 1_000_000) * inputPricePerM +
    (outputTokens / 1_000_000) * outputPricePerM +
    requests * fixedPerRequest;
  return Math.round(cost * 100) / 100;
}

interface TeamStore {
  profile: TeamProfile;
  members: TeamMember[];
  memberUsage: MemberUsage[];
  strategies: Strategy[];
  templates: ComboTemplate[];
  webhooks: WebhookItem[];
  inviteLinks: InviteLink[];
  invoices: BillingInvoice[];
  overages: OverageItem[];
  seatInfo: SeatInfo;

  // computed
  getBillingSummary: () => { order_count: number; paid_orders: number; gross_revenue: number };
  getCostAttribution: () => {
    month: string;
    total_cost: number;
    previous_cost: number;
    members: (MemberUsage & { cost: number; pct: number })[];
    model_breakdown: { model: string; requests: number; input_tokens: number; output_tokens: number; cost: number }[];
  };

  // profile
  updateProfile: (data: Partial<TeamProfile>) => void;

  // members
  addMember: (email: string, role: string) => void;
  addMembersBulk: (emails: string[], role: string) => void;
  updateMemberRole: (memberId: string, role: string) => void;
  setMemberStatus: (memberId: string, status: "active" | "disabled") => void;
  removeMember: (memberId: string) => void;
  recordUsage: (memberId: string, requests: number, inputTokens: number, outputTokens: number) => void;

  // invite links
  createInviteLink: (role: string, expiresIn: number, maxUses: number | null) => InviteLink;
  deleteInviteLink: (linkId: string) => void;

  // strategies
  addStrategy: (name: string, type: string, config: string) => void;
  updateStrategy: (id: string, data: Partial<Strategy>) => void;
  deleteStrategy: (id: string) => void;

  // templates
  addTemplate: (name: string, models: string, routingMode: string) => void;
  updateTemplate: (id: string, data: Partial<ComboTemplate>) => void;
  deleteTemplate: (id: string) => void;

  // webhooks
  addWebhook: (data: Partial<WebhookItem>) => void;
  updateWebhook: (id: string, data: Partial<WebhookItem>) => void;
  deleteWebhook: (id: string) => void;

  // seat
  updateSeats: (seats: number) => void;
  updateSeatInfo: () => void;

  // invoices / overages
  addInvoice: (inv: BillingInvoice) => void;
  addOverage: (o: OverageItem) => void;
}

const defaultProfile: TeamProfile = {
  name: "",
  slug: "my-team",
  status: "active",
  billing_email: "",
  shared_quota: 1000000,
  quota_used: 0,
};

export const useTeamStore = create<TeamStore>()(
  persist(
    (set, get) => ({
      profile: { ...defaultProfile },
      members: [
        {
          member_id: generateId(),
          email: "owner@example.com",
          role: "owner",
          status: "active",
          created_at: new Date().toISOString(),
        },
      ],
      memberUsage: [],
      strategies: [],
      templates: [],
      webhooks: [],
      inviteLinks: [],
      invoices: [],
      overages: [],
      seatInfo: { seats: 5, active_members: 1, available: 4 },

      getBillingSummary: () => {
        const { invoices } = get();
        const paid = invoices.filter((i) => i.status === "paid");
        return {
          order_count: invoices.length,
          paid_orders: paid.length,
          gross_revenue: paid.reduce((s, i) => s + i.amount, 0),
        };
      },

      getCostAttribution: () => {
        const { memberUsage } = get();
        const now = new Date();
        const month = now.toISOString().substring(0, 7);
        const items = memberUsage.map((m) => {
          const cost = estimateCost(m.input_tokens, m.output_tokens, m.requests);
          return { ...m, cost, pct: 0 };
        });
        const totalCost = items.reduce((s, i) => s + i.cost, 0);
        items.forEach((i) => {
          i.pct = totalCost > 0 ? Math.round((i.cost / totalCost) * 1000) / 10 : 0;
        });
        return {
          month,
          total_cost: Math.round(totalCost * 100) / 100,
          previous_cost: 0,
          members: items,
          model_breakdown: [],
        };
      },

      updateProfile: (data) => {
        set((s) => ({ profile: { ...s.profile, ...data } }));
      },

      addMember: (email, role) => {
        set((s) => ({
          members: [
            ...s.members,
            {
              member_id: generateId(),
              email,
              role: role as any,
              status: "active",
              created_at: new Date().toISOString(),
            },
          ],
        }));
        get().updateSeatInfo();
      },

      addMembersBulk: (emails, role) => {
        set((s) => ({
          members: [
            ...s.members,
            ...emails.map((email) => ({
              member_id: generateId(),
              email,
              role: role as any,
              status: "active" as const,
              created_at: new Date().toISOString(),
            })),
          ],
        }));
        get().updateSeatInfo();
      },

      updateMemberRole: (memberId, role) => {
        set((s) => ({
          members: s.members.map((m) =>
            m.member_id === memberId ? { ...m, role: role as any } : m
          ),
        }));
      },

      setMemberStatus: (memberId, status) => {
        set((s) => ({
          members: s.members.map((m) =>
            m.member_id === memberId ? { ...m, status } : m
          ),
        }));
        get().updateSeatInfo();
      },

      removeMember: (memberId) => {
        set((s) => ({
          members: s.members.filter((m) => m.member_id !== memberId),
          memberUsage: s.memberUsage.filter((u) => u.member_id !== memberId),
        }));
        get().updateSeatInfo();
      },

      recordUsage: (memberId, requests, inputTokens, outputTokens) => {
        set((s) => {
          const existing = s.memberUsage.find((u) => u.member_id === memberId);
          if (existing) {
            return {
              memberUsage: s.memberUsage.map((u) =>
                u.member_id === memberId
                  ? {
                      ...u,
                      requests: u.requests + requests,
                      input_tokens: u.input_tokens + inputTokens,
                      output_tokens: u.output_tokens + outputTokens,
                    }
                  : u
              ),
            };
          }
          const member = s.members.find((m) => m.member_id === memberId);
          if (!member) return s;
          return {
            memberUsage: [
              ...s.memberUsage,
              {
                member_id: memberId,
                email: member.email,
                role: member.role,
                requests,
                input_tokens: inputTokens,
                output_tokens: outputTokens,
              },
            ],
          };
        });
      },

      createInviteLink: (role, expiresIn, maxUses) => {
        const link: InviteLink = {
          link_id: generateId(),
          token: generateId() + generateId(),
          role,
          expires_at: expiresIn > 0
            ? new Date(Date.now() + expiresIn * 1000).toISOString()
            : null,
          used_count: 0,
          max_uses: maxUses,
          created_at: new Date().toISOString(),
        };
        set((s) => ({ inviteLinks: [...s.inviteLinks, link] }));
        return link;
      },

      deleteInviteLink: (linkId) => {
        set((s) => ({
          inviteLinks: s.inviteLinks.filter((l) => l.link_id !== linkId),
        }));
      },

      addStrategy: (name, strategy_type, config) => {
        const now = new Date().toISOString();
        set((s) => ({
          strategies: [
            ...s.strategies,
            {
              strategy_id: generateId(),
              name,
              strategy_type,
              config,
              created_at: now,
              updated_at: now,
            },
          ],
        }));
      },

      updateStrategy: (id, data) => {
        set((s) => ({
          strategies: s.strategies.map((st) =>
            st.strategy_id === id
              ? { ...st, ...data, updated_at: new Date().toISOString() }
              : st
          ),
        }));
      },

      deleteStrategy: (id) => {
        set((s) => ({
          strategies: s.strategies.filter((st) => st.strategy_id !== id),
        }));
      },

      addTemplate: (name, models, routing_mode) => {
        const now = new Date().toISOString();
        set((s) => ({
          templates: [
            ...s.templates,
            {
              template_id: generateId(),
              name,
              models,
              routing_mode,
              created_at: now,
              updated_at: now,
            },
          ],
        }));
      },

      updateTemplate: (id, data) => {
        set((s) => ({
          templates: s.templates.map((t) =>
            t.template_id === id
              ? { ...t, ...data, updated_at: new Date().toISOString() }
              : t
          ),
        }));
      },

      deleteTemplate: (id) => {
        set((s) => ({
          templates: s.templates.filter((t) => t.template_id !== id),
        }));
      },

      addWebhook: (data) => {
        const now = new Date().toISOString();
        set((s) => ({
          webhooks: [
            ...s.webhooks,
            {
              webhook_id: generateId(),
              name: data.name || "",
              url: data.url || "",
              events: data.events || "[]",
              secret: data.secret || "",
              enabled: data.enabled ?? true,
              created_at: now,
              updated_at: now,
            },
          ],
        }));
      },

      updateWebhook: (id, data) => {
        set((s) => ({
          webhooks: s.webhooks.map((w) =>
            w.webhook_id === id
              ? { ...w, ...data, updated_at: new Date().toISOString() }
              : w
          ),
        }));
      },

      deleteWebhook: (id) => {
        set((s) => ({
          webhooks: s.webhooks.filter((w) => w.webhook_id !== id),
        }));
      },

      updateSeats: (seats) => {
        set((s) => ({
          seatInfo: {
            seats,
            active_members: s.members.filter((m) => m.status === "active").length,
            available: seats - s.members.filter((m) => m.status === "active").length,
          },
        }));
      },

      updateSeatInfo: () => {
        set((s) => {
          const active = s.members.filter((m) => m.status === "active").length;
          return {
            seatInfo: {
              seats: Math.max(s.seatInfo.seats, active),
              active_members: active,
              available: Math.max(0, s.seatInfo.seats - active),
            },
          };
        });
      },

      addInvoice: (inv) => {
        set((s) => ({ invoices: [...s.invoices, inv] }));
      },

      addOverage: (o) => {
        set((s) => ({ overages: [...s.overages, o] }));
      },
    }),
    {
      name: "seasagi-team-workspace",
      version: 1,
    }
  )
);
