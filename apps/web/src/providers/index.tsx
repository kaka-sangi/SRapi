"use client";

import { ThemeProvider } from "./theme-provider";
import { QueryProvider } from "./query-provider";
import { LanguageProvider } from "@/context/LanguageContext";
import { ToastUIProvider } from "@/context/ToastContext";
import { TooltipProvider } from "@/components/ui/tooltip";

export function Providers({
  children,
  nonce,
}: {
  children: React.ReactNode;
  nonce?: string;
}) {
  return (
    <ThemeProvider nonce={nonce}>
      <QueryProvider>
        <LanguageProvider>
          {/* App-wide tooltip context: a brief hover delay, instant re-show when
              skating across adjacent triggers (icon buttons, truncated values). */}
          <TooltipProvider delayDuration={250} skipDelayDuration={400}>
            <ToastUIProvider>{children}</ToastUIProvider>
          </TooltipProvider>
        </LanguageProvider>
      </QueryProvider>
    </ThemeProvider>
  );
}
