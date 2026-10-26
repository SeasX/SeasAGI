import { useState, type CSSProperties } from "react";
import { useTranslation } from "../i18n";

export interface MoneyInputOptions {
  /** 最小值（含） */
  min?: number;
  /** 最大值（含） */
  max?: number;
  /** 允许的小数位数 */
  precision?: number;
  /** 是否必须为整数 */
  integer?: boolean;
  /** 空字符串是否视为合法（可选字段） */
  allowEmpty?: boolean;
}

/**
 * 解析金额/配额输入：拒绝 NaN、Infinity、负数、越界与精度超限。
 * 返回 null 表示不合法（空字符串在 allowEmpty 时同样返回 null）。
 */
export function parseMoneyInput(raw: string, options: MoneyInputOptions = {}): number | null {
  const text = raw.trim();
  if (text === "") return null;
  // 仅接受普通十进制数字，避免 Number("0x10")、Number("1e3") 等隐式转换。
  if (!/^\d+(\.\d+)?$/.test(text)) return null;
  const value = Number(text);
  if (!Number.isFinite(value)) return null;
  if (value < 0) return null;
  if (options.integer && !Number.isInteger(value)) return null;
  if (options.min !== undefined && value < options.min) return null;
  if (options.max !== undefined && value > options.max) return null;
  if (!options.integer && options.precision !== undefined) {
    const decimals = text.includes(".") ? text.split(".")[1].length : 0;
    if (decimals > options.precision) return null;
  }
  return value;
}

interface MoneyInputProps extends MoneyInputOptions {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  required?: boolean;
  disabled?: boolean;
  style?: CSSProperties;
  className?: string;
}

/**
 * 金额/配额输入框：在输入时即时校验有限性、非负、精度与上下限，
 * 避免 Infinity/NaN 被 JSON 序列化成 null 提交到后端。
 */
export function MoneyInput({
  value,
  onChange,
  min,
  max,
  precision = 2,
  integer = false,
  allowEmpty = false,
  placeholder,
  required = false,
  disabled = false,
  style,
  className = "form-input",
}: MoneyInputProps) {
  const { t } = useTranslation();
  const [error, setError] = useState<string | null>(null);

  const validate = (text: string): string | null => {
    if (!text.trim()) return required ? t("money.required") : null;
    if (!/^\d+(\.\d+)?$/.test(text.trim())) return t("money.invalid");
    const num = Number(text);
    if (!Number.isFinite(num)) return t("money.invalid");
    if (num < 0) return t("money.nonNegative");
    if (min !== undefined && num < min) return t("money.min", { min });
    if (max !== undefined && num > max) return t("money.max", { max });
    if (integer && !Number.isInteger(num)) return t("money.integer");
    if (!integer && !allowEmpty) {
      const decimals = text.includes(".") ? text.split(".")[1].length : 0;
      if (decimals > precision) return t("money.precision", { precision });
    }
    return null;
  };

  return (
    <div>
      <input
        className={className}
        type="text"
        inputMode={integer ? "numeric" : "decimal"}
        value={value}
        onChange={(e) => {
          const next = e.target.value;
          onChange(next);
          setError(validate(next));
        }}
        placeholder={placeholder}
        required={required}
        disabled={disabled}
        aria-invalid={!!error}
        style={style}
      />
      {error && (
        <span className="form-hint" style={{ color: "var(--danger, #ef4444)", display: "block", marginTop: 4 }}>
          {error}
        </span>
      )}
    </div>
  );
}
