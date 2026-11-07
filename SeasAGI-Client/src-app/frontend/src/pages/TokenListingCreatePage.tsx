import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  exportLocalAssets,
  listMyAssets,
  platformRequest,
  syncAssetsToCloud,
  asPlatformRecord,
} from "../utils/commands";
import { MoneyInput, parseMoneyInput } from "../components/MoneyInput";
import { useTranslation } from "../i18n";

type Asset = {
  asset_id: string;
  token_fingerprint: string;
  label: string;
  provider_type: string;
  wire_protocol: string;
  base_url: string;
  models: string[];
  has_credential: boolean;
};

type ModelRow = {
  row_key: string;
  model_id: string;
  wire_protocol: string;
  input_price_per_1m: string;
  output_price_per_1m: string;
  weekly_quota_usd: string;
};

type SaleType = "subscription" | "fixed_price" | "discount";

type ImportStatus =
  | { kind: "idle" }
  | { kind: "success"; text: string }
  | { kind: "error"; text: string };

const PROTOCOLS = ["openai-chat", "openai-responses", "anthropic", "gemini"];

let modelRowSeq = 0;
const nextModelRowKey = () => `model-row-${++modelRowSeq}`;

export function TokenListingCreatePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const [assets, setAssets] = useState<Asset[]>([]);
  const [assetID, setAssetID] = useState("");
  const [saleType, setSaleType] = useState<SaleType>("subscription");
  const [tokenLabel, setTokenLabel] = useState("");
  const [price, setPrice] = useState("");
  const [discountRate, setDiscountRate] = useState("");
  const [capacity, setCapacity] = useState("1");
  const [quotaPerSlot, setQuotaPerSlot] = useState("");
  const [autoApprove, setAutoApprove] = useState(true);
  const [models, setModels] = useState<ModelRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [importStatus, setImportStatus] = useState<ImportStatus>({ kind: "idle" });
  const currency = "USD";

  const selected = useMemo(() => assets.find((a) => a.asset_id === assetID), [assets, assetID]);

  const refreshAssets = async () => {
    try {
      const list = await listMyAssets();
      const normalized = (list || []).map((a: any) => ({
        ...a,
        models: Array.isArray(a.models) ? a.models : [],
      })) as Asset[];
      setAssets(normalized);
      if (normalized.length > 0 && !assetID) {
        setAssetID(normalized[0].asset_id);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  useEffect(() => {
    void refreshAssets();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 选中资产后同步 label 与默认模型行，减少手填。
  useEffect(() => {
    if (!selected) return;
    if (!tokenLabel.trim()) setTokenLabel(selected.label);
    if (models.length === 0 && selected.models.length > 0) {
      setModels(
        selected.models.map((m) => ({
          row_key: nextModelRowKey(),
          model_id: m,
          wire_protocol: selected.wire_protocol || "openai-chat",
          input_price_per_1m: "",
          output_price_per_1m: "",
          weekly_quota_usd: "",
        })),
      );
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected]);

  const handleImport = async () => {
    setImporting(true);
    setImportStatus({ kind: "idle" });
    try {
      const localAssets = await exportLocalAssets();
      if (!localAssets || localAssets.length === 0) {
        throw new Error(t("tokenMarket.noAssets"));
      }
      const resp = await syncAssetsToCloud(localAssets);
      const synced = resp?.data?.synced ?? localAssets.length;
      await refreshAssets();
      setImportStatus({
        kind: "success",
        text: t("tokenMarket.assetImported", { count: synced }),
      });
    } catch (e) {
      setImportStatus({
        kind: "error",
        text: e instanceof Error ? e.message : String(e),
      });
    } finally {
      setImporting(false);
    }
  };

  const updateModel = (index: number, patch: Partial<ModelRow>) => {
    setModels((prev) => prev.map((m, i) => (i === index ? { ...m, ...patch } : m)));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    try {
      if (!selected) {
        throw new Error(t("tokenMarket.assetSelect"));
      }
      const body: Record<string, any> = {
        asset_id: selected.asset_id,
        token_label: tokenLabel || selected.label,
        token_fingerprint: selected.token_fingerprint,
        sale_type: saleType,
        currency,
      };

      const quota = parseMoneyInput(quotaPerSlot, { min: 0.01, precision: 2 });
      const slots = parseMoneyInput(capacity, { min: 1, max: 100000, integer: true });
      const priceNum = parseMoneyInput(price, { min: 0.01, precision: 2 });
      if (saleType === "subscription") {
        if (priceNum === null) throw new Error(t("money.invalid"));
        if (slots === null) throw new Error(t("tokenMarket.capacity"));
        if (quota === null) throw new Error(t("tokenMarket.quotaPerSlot"));
        if (models.length === 0) throw new Error(t("tokenMarket.models"));
        body.price = priceNum;
        body.capacity = slots;
        body.quota_per_slot_usd = quota;
        body.available_quota = slots * quota;
        body.auto_approve = autoApprove;
        body.models = models.map((m) => ({
          model_id: m.model_id.trim(),
          wire_protocol: m.wire_protocol,
          input_price_per_1m: parseMoneyInput(m.input_price_per_1m, { min: 0, precision: 4 }) ?? 0,
          output_price_per_1m: parseMoneyInput(m.output_price_per_1m, { min: 0, precision: 4 }) ?? 0,
          weekly_quota_usd: parseMoneyInput(m.weekly_quota_usd, { min: 0, precision: 2 }) ?? quota,
          enabled: true,
        }));
      } else {
        if (priceNum === null) throw new Error(t("tokenMarket.price"));
        if (quota === null) throw new Error(t("tokenMarket.availableQuota"));
        body.price = priceNum;
        body.available_quota = quota;
        if (saleType === "discount") {
          const rate = parseMoneyInput(discountRate, { min: 0.01, max: 1, precision: 2 });
          if (rate === null) throw new Error(t("tokenMarket.discountRate"));
          body.discount_rate = rate;
        }
        if (models.length > 0) {
          body.models = models.map((m) => ({
            model_id: m.model_id.trim(),
            wire_protocol: m.wire_protocol,
            input_price_per_1m: parseMoneyInput(m.input_price_per_1m, { min: 0, precision: 4 }) ?? 0,
            output_price_per_1m: parseMoneyInput(m.output_price_per_1m, { min: 0, precision: 4 }) ?? 0,
            weekly_quota_usd: 0,
            enabled: true,
          }));
        }
      }

      const resp = await platformRequest("POST", "/token-market/listings", body);
      if (resp.status >= 400) {
        throw new Error(String(asPlatformRecord(resp.body).error || `HTTP ${resp.status}`));
      }
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
          <label className="form-label">{t("tokenMarket.assetSelect")}</label>
          <div style={{ display: "flex", gap: 8 }}>
            <select
              className="form-input"
              value={assetID}
              onChange={(e) => setAssetID(e.target.value)}
              required
            >
              {assets.length === 0 && <option value="">{t("tokenMarket.noAssets")}</option>}
              {assets.map((a) => (
                <option key={a.asset_id} value={a.asset_id}>
                  {a.label} · {a.wire_protocol} · {a.base_url}
                </option>
              ))}
            </select>
            <button type="button" className="btn btn-secondary" onClick={handleImport} disabled={importing}>
              {importing ? t("common.submitting") : t("tokenMarket.importAssets")}
            </button>
          </div>
          {importStatus.kind !== "idle" && (
            <div
              className={importStatus.kind === "success" ? "form-success" : "form-error"}
              role={importStatus.kind === "success" ? "status" : "alert"}
              style={{ marginTop: 8 }}
            >
              {importStatus.text}
            </div>
          )}
        </div>

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
          <label className="form-label">{t("tokenMarket.saleType")}</label>
          <select
            className="form-input"
            value={saleType}
            onChange={(e) => setSaleType(e.target.value as SaleType)}
          >
            <option value="subscription">{t("tokenMarket.saleTypeSubscription")}</option>
            <option value="fixed_price">{t("tokenMarket.fixedPrice")}</option>
            <option value="discount">{t("tokenMarket.discount")}</option>
          </select>
        </div>

        {saleType === "subscription" ? (
          <>
            <div className="form-group">
              <label className="form-label">{t("tokenMarket.weeklyPrice")} ({currency})</label>
              <MoneyInput
                value={price}
                onChange={setPrice}
                min={0.01}
                max={1000000}
                precision={2}
                placeholder="0.00"
                required
              />
            </div>
            <div style={{ display: "flex", gap: 12 }}>
              <div className="form-group" style={{ flex: 1 }}>
                <label className="form-label">{t("tokenMarket.capacity")}</label>
                <MoneyInput
                  value={capacity}
                  onChange={setCapacity}
                  min={1}
                  max={100000}
                  integer
                  required
                />
              </div>
              <div className="form-group" style={{ flex: 1 }}>
                <label className="form-label">{t("tokenMarket.quotaPerSlot")} ({currency})</label>
                <MoneyInput
                  value={quotaPerSlot}
                  onChange={setQuotaPerSlot}
                  min={0.01}
                  max={1000000}
                  precision={2}
                  required
                />
              </div>
            </div>
            <div className="form-group">
              <label style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <input
                  type="checkbox"
                  checked={autoApprove}
                  onChange={(e) => setAutoApprove(e.target.checked)}
                />
                {t("tokenMarket.autoApprove")}
              </label>
            </div>
          </>
        ) : (
          <>
            <div className="form-group">
              <label className="form-label">{t("tokenMarket.price")} ({currency})</label>
              <MoneyInput
                value={price}
                onChange={setPrice}
                min={0.01}
                max={1000000}
                precision={2}
                required
              />
            </div>
            {saleType === "discount" && (
              <div className="form-group">
                <label className="form-label">{t("tokenMarket.discountRate")} (0-1)</label>
                <MoneyInput
                  value={discountRate}
                  onChange={setDiscountRate}
                  min={0.01}
                  max={1}
                  precision={2}
                  required
                />
              </div>
            )}
            <div className="form-group">
              <label className="form-label">{t("tokenMarket.availableQuota")}</label>
              <MoneyInput
                value={quotaPerSlot}
                onChange={setQuotaPerSlot}
                min={0.01}
                max={1000000}
                precision={2}
                required
              />
            </div>
          </>
        )}

        <div className="form-group">
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <label className="form-label">{t("tokenMarket.models")}</label>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={() =>
                setModels((prev) => [
                  ...prev,
                  {
                    row_key: nextModelRowKey(),
                    model_id: "",
                    wire_protocol: selected?.wire_protocol || "openai-chat",
                    input_price_per_1m: "",
                    output_price_per_1m: "",
                    weekly_quota_usd: "",
                  },
                ])
              }
            >
              {t("tokenMarket.addModel")}
            </button>
          </div>
          {models.length === 0 && <p className="form-hint">{t("tokenMarket.noData")}</p>}
          {models.map((m, i) => (
            <div key={m.row_key} style={{ display: "flex", gap: 8, marginTop: 8, alignItems: "center" }}>
              <input
                className="form-input"
                style={{ flex: 2 }}
                value={m.model_id}
                onChange={(e) => updateModel(i, { model_id: e.target.value })}
                placeholder={t("tokenMarket.modelId")}
              />
              <select
                className="form-input"
                style={{ flex: 1 }}
                value={m.wire_protocol}
                onChange={(e) => updateModel(i, { wire_protocol: e.target.value })}
              >
                {PROTOCOLS.map((p) => (
                  <option key={p} value={p}>{p}</option>
                ))}
              </select>
              <div style={{ flex: 1 }}>
                <MoneyInput
                  value={m.input_price_per_1m}
                  onChange={(v) => updateModel(i, { input_price_per_1m: v })}
                  min={0}
                  max={1000000}
                  precision={4}
                  allowEmpty
                  placeholder={t("tokenMarket.inputPrice")}
                />
              </div>
              <div style={{ flex: 1 }}>
                <MoneyInput
                  value={m.output_price_per_1m}
                  onChange={(v) => updateModel(i, { output_price_per_1m: v })}
                  min={0}
                  max={1000000}
                  precision={4}
                  allowEmpty
                  placeholder={t("tokenMarket.outputPrice")}
                />
              </div>
              {saleType === "subscription" && (
                <div style={{ flex: 1 }}>
                  <MoneyInput
                    value={m.weekly_quota_usd}
                    onChange={(v) => updateModel(i, { weekly_quota_usd: v })}
                    min={0}
                    max={1000000}
                    precision={2}
                    allowEmpty
                    placeholder={t("tokenMarket.weeklyQuota")}
                  />
                </div>
              )}
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => setModels((prev) => prev.filter((_, idx) => idx !== i))}
              >
                {t("tokenMarket.remove")}
              </button>
            </div>
          ))}
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
