'use client';

import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
} from 'react';
import { applyVariables, flatLookup, type Locale } from '@/i18n/messages';

type Language = Locale;

interface LanguageContextType {
  language: Language;
  toggleLanguage: () => void;
  t: (key: string, variables?: Record<string, string | number>) => string;
}

// SRapi v0.1.0 i18n shim. The full per-namespace dictionary lives in
// `src/i18n/messages/{en,zh}.ts`. Pages keep using the same `t(key)` API.

const STORAGE_KEY = 'srapi_lang';
const STORAGE_EVENT = 'srapi:language-change';

function readLanguageFromStorage(): Language {
  if (typeof window === 'undefined') return 'en';
  const saved = window.localStorage.getItem(STORAGE_KEY);
  return saved === 'zh' ? 'zh' : 'en';
}

function subscribeToLanguage(notify: () => void): () => void {
  if (typeof window === 'undefined') return () => {};
  const listener = (event: StorageEvent | Event) => {
    if (event instanceof StorageEvent && event.key && event.key !== STORAGE_KEY) return;
    notify();
  };
  window.addEventListener('storage', listener);
  window.addEventListener(STORAGE_EVENT, listener);
  return () => {
    window.removeEventListener('storage', listener);
    window.removeEventListener(STORAGE_EVENT, listener);
  };
}

function setLanguageInStorage(next: Language): void {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(STORAGE_KEY, next);
  // Mirror into a cookie so the edge/server can read the locale (e.g. to set
  // <html lang> without a flash) in a later pass. Carries no credentials.
  document.cookie = `${STORAGE_KEY}=${next}; Path=/; Max-Age=${60 * 60 * 24 * 365}; SameSite=Lax`;
  window.dispatchEvent(new Event(STORAGE_EVENT));
}

const LanguageContext = createContext<LanguageContextType | undefined>(undefined);

export function LanguageProvider({ children }: { children: ReactNode }) {
  const language = useSyncExternalStore<Language>(
    subscribeToLanguage,
    readLanguageFromStorage,
    () => 'en',
  );

  // Keep the document language attribute in sync for accessibility / SEO.
  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = language;
    }
  }, [language]);

  const toggleLanguage = useCallback(() => {
    setLanguageInStorage(language === 'en' ? 'zh' : 'en');
  }, [language]);

  const value = useMemo<LanguageContextType>(() => {
    const dict = flatLookup(language);
    const fallback = flatLookup('en');
    return {
      language,
      toggleLanguage,
      t: (key, variables) => applyVariables(dict[key] ?? fallback[key] ?? key, variables),
    };
  }, [language, toggleLanguage]);

  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}

export function useLanguage() {
  const context = useContext(LanguageContext);
  if (!context) {
    throw new Error('useLanguage must be used within a LanguageProvider');
  }
  return context;
}
