import React, { createContext, useContext, useState, useCallback } from "react";

type Locale = "zh-CN" | "en" | "ja" | "ko";

interface I18nContextValue {
  locale: Locale;
  setLocale: (l: Locale) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

const I18nContext = createContext<I18nContextValue>({
  locale: "zh-CN",
  setLocale: () => {},
  t: (key: string) => key,
});

export function useTranslation() {
  return useContext(I18nContext);
}

// Lazy-loaded locale data
let locales: Record<string, Record<string, string>> | null = null;

async function loadLocales() {
  if (locales) return locales;
  const langs: Locale[] = ["zh-CN", "en", "ja", "ko"];
  const result: Record<string, Record<string, string>> = {};
  for (const lang of langs) {
    try {
      const mod = await import(`./locales/${lang}.json`);
      result[lang] = mod.default || mod;
    } catch {
      result[lang] = {};
    }
  }
  locales = result;
  return result;
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("zh-CN");
  const [data, setData] = useState<Record<string, Record<string, string>> | null>(null);

  React.useEffect(() => {
    loadLocales().then(setData);
  }, []);

  const setLocale = useCallback((l: Locale) => {
    setLocaleState(l);
  }, []);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>) => {
      if (!data) return key;
      let msg = data[locale]?.[key];
      if (!msg) msg = data["zh-CN"]?.[key];
      if (!msg) return key;
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          msg = msg.replace(`{{${k}}}`, String(v));
        }
      }
      return msg;
    },
    [locale, data]
  );

  return (
    <I18nContext.Provider value={{ locale, setLocale, t }}>
      {children}
    </I18nContext.Provider>
  );
}