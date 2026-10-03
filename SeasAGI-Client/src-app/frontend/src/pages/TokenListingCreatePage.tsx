import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { platformRequest } from "../utils/commands";
import { useTranslation } from "../i18n";
import { useMarketStore, type MarketListing } from "../stores/marketStore";

export function TokenListingCreatePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const setMyListings = useMarketStore((s) => s.setMyListings);
  const myListings = useMarketStore((s) => s.myListings);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // 表单状态
  const [tokenLabel, setTokenLabel] = useState("");
  const [tokenFingerprint, setTokenFingerprint] = useState("");
  const [saleType, setSaleType] = useState<"discount" | "fixed_price">("fixed_price");
  const [price, setPrice] = useState("");
  const [discountRate, setDiscountRate] = useState("");
  const [availableQuota, setAvailableQuota] = useState("");
  const [currency, setCurrency] = useState("USD");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      const body: Record<string, any> = {
        token_label: tokenLabel,
        token_fingerprint: tokenFingerprint,
        sale_type: saleType,
        currency,
      };
      if (saleType === "fixed_price") {
        body.price = parseFloat(price);
        if (!body.price || body.price <= 0) {
          throw new Error(t("tokenMarket.price") + " must be positive");
        }
      } else {
        body.discount_rate = parseFloat(discountRate);
        if (!body.discount_rate || body.discount_rate <= 0 || body.discount_rate > 1) {
          throw new Error(t("tokenMarket.discountRate") + " must be between 0 and 1");
        }
        body.price = parseFloat(price);
        if (!body.price || body.price <= 0) {
          throw new Error(t("tokenMarket.price") + " must be positive");
        }
      }
      if (availableQuota) {
        body.available_quota = parseFloat(availableQuota) || 0;
      }

      const resp = await platformRequest("POST", "/token-market/listings", body);

      if (resp.status >= 400) {
        const errData = resp.body || {};
        throw new Error(errData.error || `HTTP ${resp.status}`);
      }

      const data = resp.body;
      const newListing: MarketListing = data.data;
      setMyListings([newListing, ...myListings]);
      navigate("/token-market/my-listings");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="page-container">
      <div className="page-header">
        <h1>{t("tokenMarket.createListing")}</h1>
        <p className="page-subtitle">{t("tokenMarket.createListingSubtitle")}</p>
      </div>

      <form onSubmit={handleSubmit} className="form-container">
        <div className="form-group">
          <label className="form-label">{t("tokenMarket.tokenLabel")}</label>
          <input
            className="form-input"
            value={tokenLabel}
            onChange={(e) => setTokenLabel(e.target.value)}
            placeholder={t("tokenMarket.tokenLabelPlaceholder")}
            required
          />
        </div>

        <div className="form-group">
          <label className="form-label">{t("tokenMarket.tokenFingerprint")}</label>
          <input
            className="form-input"
            value={tokenFingerprint}
            onChange={(e) => setTokenFingerprint(e.target.value)}
            placeholder={t("tokenMarket.tokenFingerprintPlaceholder")}
            required
          />
        </div>

        <div className="form-group">
          <label className="form-label">{t("tokenMarket.saleType")}</label>
          <select
            className="form-input"
            value={saleType}
            onChange={(e) => setSaleType(e.target.value as "discount" | "fixed_price")}
          >
            <option value="fixed_price">{t("tokenMarket.fixedPrice")}</option>
            <option value="discount">{t("tokenMarket.discount")}</option>
          </select>
        </div>

        {saleType === "fixed_price" ? (
          <div className="form-group">
            <label className="form-label">{t("tokenMarket.price")} ({currency})</label>
            <input
              className="form-input"
              type="number"
              step="0.01"
              min="0"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              placeholder="0.00"
              required
            />
          </div>
        ) : (
          <>
            <div className="form-group">
              <label className="form-label">{t("tokenMarket.price")} ({currency})</label>
              <input
                className="form-input"
                type="number"
                step="0.01"
                min="0"
                value={price}
                onChange={(e) => setPrice(e.target.value)}
                placeholder="0.00"
                required
              />
            </div>
            <div className="form-group">
              <label className="form-label">{t("tokenMarket.discountRate")} (0-1)</label>
              <input
                className="form-input"
                type="number"
                step="0.01"
                min="0"
                max="1"
                value={discountRate}
                onChange={(e) => setDiscountRate(e.target.value)}
                placeholder="0.20"
                required
              />
            </div>
          </>
        )}

        <div className="form-group">
          <label className="form-label">{t("tokenMarket.availableQuota")}</label>
          <input
            className="form-input"
            type="number"
            step="0.01"
            min="0"
            value={availableQuota}
            onChange={(e) => setAvailableQuota(e.target.value)}
            placeholder="0.00"
          />
        </div>

        <div className="form-group">
          <label className="form-label">{t("tokenMarket.currency")}</label>
          <select
            className="form-input"
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
          >
            <option value="USD">USD</option>
            <option value="CNY">CNY</option>
            <option value="EUR">EUR</option>
            <option value="JPY">JPY</option>
          </select>
        </div>

        {error && <div className="form-error">{error}</div>}

        <div className="form-actions">
          <button type="button" className="btn btn-secondary" onClick={() => navigate("/token-market")}>
            {t("common.cancel")}
          </button>
          <button type="submit" className="btn btn-primary" disabled={loading}>
            {loading ? t("common.submitting") : t("tokenMarket.submitListing")}
          </button>
        </div>
      </form>
    </div>
  );
}
