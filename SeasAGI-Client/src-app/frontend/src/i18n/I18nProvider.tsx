import { useState, useCallback, useMemo } from "react";
import { I18nContext, DEFAULT_LOCALE, type Locale } from "./index";
import zhCN from "./locales/zh-CN.json";
import en from "./locales/en.json";
import ja from "./locales/ja.json";
import ko from "./locales/ko.json";
import * as cmd from "../utils/commands";

const LOCALE_STORAGE_KEY = "seasagi-locale";

const translations: Record<Locale, Record<string, string>> = {
  "zh-CN": zhCN,
  en,
  ja,
  ko,
};

function detectSystemLocale(): Locale {
  const lang = navigator.language || "zh-CN";
  if (lang.startsWith("zh")) return "zh-CN";
  if (lang.startsWith("ja")) return "ja";
  if (lang.startsWith("ko")) return "ko";
  return "en";
}

function getSavedLocale(): Locale {
  try {
    const saved = localStorage.getItem(LOCALE_STORAGE_KEY) as Locale;
    if (saved && translations[saved]) return saved;
  } catch {}
  return detectSystemLocale();
}

function interpolate(template: string, params?: Record<string, string | number>): string {
  if (!params) return template;
  let result = template;
  for (const [key, value] of Object.entries(params)) {
    result = result.replace(new RegExp(`\\{\\{${key}\\}\\}`, "g"), String(value));
  }
  return result;
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(getSavedLocale);

  const setLocale = useCallback((newLocale: Locale) => {
    setLocaleState(newLocale);
    try {
      localStorage.setItem(LOCALE_STORAGE_KEY, newLocale);
    } catch {}
    try {
      cmd.setLocale(newLocale);
    } catch {}
  }, []);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const dict = translations[locale] || translations[DEFAULT_LOCALE];
      const template = dict[key] || translations[DEFAULT_LOCALE][key] || key;
      return interpolate(template, params);
    },
    [locale]
  );

  const value = useMemo(
    () => ({ locale, setLocale, t }),
    [locale, setLocale, t]
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}
