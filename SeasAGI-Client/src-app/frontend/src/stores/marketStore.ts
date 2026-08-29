import { create } from "zustand";

// Token 市场上架信息
export interface MarketListing {
  listing_id: string;
  seller_user_id: string;
  seller_username: string;
  token_label: string;
  token_fingerprint: string;
  sale_type: "discount" | "fixed_price";
  price: number;
  discount_rate: number;
  available_quota: number;
  currency: string;
  status: "active" | "locked" | "sold" | "cancelled";
  created_at: string;
  updated_at: string;
}

// Token 市场订单
export interface MarketOrder {
  order_id: string;
  listing_id: string;
  buyer_user_id: string;
  seller_user_id: string;
  amount: number;
  commission_amount: number;
  settlement_amount: number;
  commission_rate: number;
  currency: string;
  status: "pending" | "confirmed" | "cancelled" | "settled";
  scan_code: string;
  trade_confirmed_at: string;
  created_at: string;
  updated_at: string;
  role: "buyer" | "seller";
}

// 结算记录
export interface MarketSettlement {
  settlement_id: string;
  order_id: string;
  listing_id: string;
  token_label: string;
  trade_amount: number;
  seller_payout: number;
  platform_commission: number;
  settlement_status: "pending" | "settled" | "failed";
  settled_at: string;
  created_at: string;
}

interface MarketStore {
  marketVisible: boolean;
  marketListings: MarketListing[];
  myListings: MarketListing[];
  myOrders: MarketOrder[];
  mySettlements: MarketSettlement[];
  tradeSession: MarketOrder | null;
  commissionPreview: { amount: number; commission: number; payout: number } | null;
  loading: boolean;
  error: string | null;
  freeChannelsCache: unknown[] | null;
  freeChannelsCacheTime: number | null;
  setMarketVisible: (visible: boolean) => void;
  setMarketListings: (listings: MarketListing[]) => void;
  setMyListings: (listings: MarketListing[]) => void;
  setMyOrders: (orders: MarketOrder[]) => void;
  setMySettlements: (settlements: MarketSettlement[]) => void;
  setTradeSession: (session: MarketOrder | null) => void;
  setCommissionPreview: (preview: { amount: number; commission: number; payout: number } | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  setFreeChannelsCache: (data: unknown[]) => void;
  reset: () => void;
}

export const useMarketStore = create<MarketStore>((set) => ({
  marketVisible: false,
  marketListings: [],
  myListings: [],
  myOrders: [],
  mySettlements: [],
  tradeSession: null,
  commissionPreview: null,
  loading: false,
  error: null,
  freeChannelsCache: null,
  freeChannelsCacheTime: null,
  setMarketVisible: (marketVisible) => set({ marketVisible }),
  setMarketListings: (marketListings) => set({ marketListings }),
  setMyListings: (myListings) => set({ myListings }),
  setMyOrders: (myOrders) => set({ myOrders }),
  setMySettlements: (mySettlements) => set({ mySettlements }),
  setTradeSession: (tradeSession) => set({ tradeSession }),
  setCommissionPreview: (commissionPreview) => set({ commissionPreview }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  setFreeChannelsCache: (data) => set({ freeChannelsCache: data, freeChannelsCacheTime: Date.now() }),
  reset: () => set({
    marketVisible: false,
    marketListings: [],
    myListings: [],
    myOrders: [],
    mySettlements: [],
    tradeSession: null,
    commissionPreview: null,
    loading: false,
    error: null,
    freeChannelsCache: null,
    freeChannelsCacheTime: null,
  }),
}));
