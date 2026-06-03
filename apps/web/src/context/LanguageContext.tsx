"use client";

import { createContext, useCallback, useContext, useMemo, useSyncExternalStore } from "react";
import { DEFAULT_LOCALE, type Locale, translate } from "@/i18n/messages";

const STORAGE_KEY = "srapi_lang";
const CHANGE_EVENT = "srapi:language-change";
const ONE_YEAR = 60 * 60 * 24 * 365;

function readLocale(): Locale {
  if (typeof window === "undefined") return DEFAULT_LOCALE;
  const stored = window.localStorage.getItem(STORAGE_KEY);
  return stored === "en" || stored === "zh" ? stored : DEFAULT_LOCALE;
}

function subscribe(callback: () => void): () => void {
  if (typeof window === "undefined") return () => {};
  window.addEventListener(CHANGE_EVENT, callback);
  window.addEventListener("storage", callback);
  return () => {
    window.removeEventListener(CHANGE_EVENT, callback);
    window.removeEventListener("storage", callback);
  };
}

function writeLocale(locale: Locale) {
  window.localStorage.setItem(STORAGE_KEY, locale);
  document.cookie = `${STORAGE_KEY}=${locale}; path=/; max-age=${ONE_YEAR}; samesite=lax`;
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

interface LanguageContextValue {
  language: Locale;
  setLanguage: (locale: Locale) => void;
  toggleLanguage: () => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}

const LanguageContext = createContext<LanguageContextValue | null>(null);

export function LanguageProvider({ children }: { children: React.ReactNode }) {
  const language = useSyncExternalStore(subscribe, readLocale, () => DEFAULT_LOCALE);

  const setLanguage = useCallback((locale: Locale) => writeLocale(locale), []);
  const toggleLanguage = useCallback(
    () => writeLocale(language === "zh" ? "en" : "zh"),
    [language],
  );
  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => translate(language, key, vars),
    [language],
  );

  const value = useMemo(
    () => ({ language, setLanguage, toggleLanguage, t }),
    [language, setLanguage, toggleLanguage, t],
  );

  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}

export function useLanguage(): LanguageContextValue {
  const ctx = useContext(LanguageContext);
  if (!ctx) throw new Error("useLanguage must be used within LanguageProvider");
  return ctx;
}
