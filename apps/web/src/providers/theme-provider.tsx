"use client";

import * as React from "react";
import { ThemeProvider as NextThemesProvider, type ThemeProviderProps } from "next-themes";

/**
 * SRapi v0.1.0 theme provider.
 *
 * Uses the `dark` class on `<html>` so existing CSS variables in
 * `globals.css` keep working. `next-themes` reads `localStorage` and the
 * system preference and applies the correct class before paint, eliminating
 * the flash-of-wrong-theme that the previous hand-written code produced.
 */
export function ThemeProvider(props: ThemeProviderProps) {
  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
      storageKey="srapi_theme"
      {...props}
    />
  );
}
