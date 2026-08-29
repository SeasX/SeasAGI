import { createContext, useContext } from "react";

export type Locale = "zh-CN" | "en" | "ja" | "ko";

export const SUPPORTED_LOCALES: { code: Locale; label: string }[] = [
  { code: "zh-CN", label: "中文" },
  { code: "en", label: "English" },
  { code: "ja", label: "日本語" },
  { code: "ko", label: "한국어" },
];

export const DEFAULT_LOCALE: Locale = "zh-CN";

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

export const I18nContext = createContext<I18nContextValue>({
  locale: DEFAULT_LOCALE,
  setLocale: () => {},
  t: (key) => key,
});

export function useTranslation() {
  return useContext(I18nContext);
}
