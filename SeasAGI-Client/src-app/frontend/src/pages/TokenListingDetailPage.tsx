import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { getPlatformAPIBaseURL, getPlatformToken } from "../utils/commands";
import { useTranslation } from "../i18n";
import type { MarketListing } from "../stores/marketStore";

interface SellerReview {
  review_id: string;
  order_id: string;
  reviewer_name: string;
  rating: number;
  comment: string;
  created_at: string;
}

interface SellerReviewData {
  reviews: SellerReview[];
  total: number;
  avg_rating: number;
}

export function TokenListingDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [listing, setListing] = useState<MarketListing | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [purchasing, setPurchasing] = useState(false);
  const [sellerReviews, setSellerReviews] = useState<SellerReviewData | null>(null);
  const [reviewLoading, setReviewLoading] = useState(false);

  useEffect(() => {
    const fetchListing = async () => {
      setLoading(true);
      setError(null);
      try {
        const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
        const resp = await fetch(`${baseURL}/token-market/listings/${id}`, {
          headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        });
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const data = await resp.json();
        setListing(data.data);
        // P11: 拉取卖家评价
        if (data.data?.seller_user_id) {
          fetchSellerReviews(data.data.seller_user_id);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    };
    fetchListing();
  }, [id]);

  // P11: 拉取卖家评价
  const fetchSellerReviews = async (sellerID: string) => {
    setReviewLoading(true);
    try {
      const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
      const resp = await fetch(`${baseURL}/token-market/sellers/${sellerID}/reviews`, {
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      setSellerReviews(data.data);
    } catch {
      setSellerReviews(null);
    } finally {
      setReviewLoading(false);
    }
  };

  const handlePurchase = async () => {
    if (!listing) return;
    setPurchasing(true);
    setError(null);
    try {
      const [baseURL, token] = await Promise.all([getPlatformAPIBaseURL(), getPlatformToken()]);
      const resp = await fetch(`${baseURL}/token-market/orders`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({ listing_id: listing.listing_id }),
      });
      if (!resp.ok) {
        const errData = await resp.json().catch(() => ({}));
        throw new Error(errData.error || `HTTP ${resp.status}`);
      }
      const data = await resp.json();
      // 跳转到扫码交易页
      navigate(`/token-market/scan-trade?order=${data.data.order_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPurchasing(false);
    }
  };

  if (loading) return <div className="page-container"><div className="loading-state">{t("common.loading")}</div></div>;
  if (error) return <div className="page-container"><div className="form-error">{error}</div></div>;
  if (!listing) return <div className="page-container"><div className="empty-state"><p>{t("tokenMarket.listingNotFound")}</p></div></div>;

  const displayPrice = listing.sale_type === "fixed_price"
    ? `${listing.price} ${listing.currency}`
    : `${(listing.discount_rate * 100).toFixed(0)}% ${t("tokenMarket.off")}`;

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{listing.token_label}</h1>
        <button className="btn btn-secondary" onClick={() => navigate("/token-market")}>
          ← {t("tokenMarket.backToMarket")}
        </button>
      </div>

      <div className="detail-card">
        <div className="detail-row">
          <span className="detail-key">{t("tokenMarket.seller")}</span>
          <span className="detail-value">{listing.seller_username}</span>
        </div>
        <div className="detail-row">
          <span className="detail-key">{t("tokenMarket.saleType")}</span>
          <span className="detail-value">{listing.sale_type === "fixed_price" ? t("tokenMarket.fixedPrice") : t("tokenMarket.discount")}</span>
        </div>
        <div className="detail-row">
          <span className="detail-key">{t("tokenMarket.price")}</span>
          <span className="detail-value price-value">{displayPrice}</span>
        </div>
        {listing.available_quota > 0 && (
          <div className="detail-row">
            <span className="detail-key">{t("tokenMarket.availableQuota")}</span>
            <span className="detail-value">{listing.available_quota}</span>
          </div>
        )}
        <div className="detail-row">
          <span className="detail-key">{t("tokenMarket.status")}</span>
          <span className={`status-badge status-${listing.status}`}>{listing.status}</span>
        </div>
        <div className="detail-row">
          <span className="detail-key">{t("tokenMarket.createdAt")}</span>
          <span className="detail-value">{listing.created_at}</span>
        </div>

        {listing.status === "active" && (
          <div className="detail-actions">
            <button className="btn btn-primary" onClick={handlePurchase} disabled={purchasing}>
              {purchasing ? t("common.processing") : t("tokenMarket.purchase")}
            </button>
          </div>
        )}
        {error && <div className="form-error">{error}</div>}
      </div>

      {/* P11: 卖家评价 */}
      <div className="detail-card" style={{ marginTop: "16px" }}>
        <h3 style={{ marginBottom: "12px" }}>{t("tokenMarket.sellerReviews") || "Seller Reviews"}</h3>
        {reviewLoading && <div className="loading-state">{t("common.loading")}</div>}
        {!reviewLoading && sellerReviews && sellerReviews.total > 0 && (
          <>
            <div className="detail-row" style={{ marginBottom: "12px" }}>
              <span className="detail-key">{t("tokenMarket.avgRating") || "Avg Rating"}</span>
              <span className="detail-value">
                {"★".repeat(Math.round(sellerReviews.avg_rating))}{"☆".repeat(5 - Math.round(sellerReviews.avg_rating))}
                {" "}
                <strong>{sellerReviews.avg_rating.toFixed(1)}</strong>
                {" "}({sellerReviews.total} {t("tokenMarket.reviews") || "reviews"})
              </span>
            </div>
            <div className="review-list">
              {sellerReviews.reviews.map((r) => (
                <div key={r.review_id} className="review-item" style={{ padding: "8px 0", borderBottom: "1px solid var(--border-color)" }}>
                  <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "4px" }}>
                    <span style={{ fontWeight: 600 }}>{r.reviewer_name || "Anonymous"}</span>
                    <span>{"★".repeat(r.rating)}{"☆".repeat(5 - r.rating)}</span>
                  </div>
                  {r.comment && <p style={{ fontSize: "13px", color: "var(--text-secondary)" }}>{r.comment}</p>}
                  <span style={{ fontSize: "11px", color: "var(--text-tertiary)" }}>{r.created_at}</span>
                </div>
              ))}
            </div>
          </>
        )}
        {!reviewLoading && (!sellerReviews || sellerReviews.total === 0) && (
          <p style={{ color: "var(--text-secondary)" }}>{t("tokenMarket.noReviews") || "No reviews yet"}</p>
        )}
      </div>
    </div>
  );
}
