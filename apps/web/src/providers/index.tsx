"use client";

import * as React from "react";
import { LanguageProvider } from "@/context/LanguageContext";
import { QueryProvider } from "./query-provider";
import { ThemeProvider } from "./theme-provider";

/**
 * Composed root providers for SRapi v0.1.0.
 *
 * Order matters:
 *   - ThemeProvider goes first so subsequent providers can read CSS variables.
 *   - QueryProvider wraps anything that fetches data.
 *   - LanguageProvider exposes copy via `useLanguage`.
 */
export function AppProviders({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider>
      <QueryProvider>
        <LanguageProvider>{children}</LanguageProvider>
      </QueryProvider>
    </ThemeProvider>
  );
}
