"use client";

import { ThemeProvider } from "./theme-provider";
import { QueryProvider } from "./query-provider";
import { LanguageProvider } from "@/context/LanguageContext";
import { ToastUIProvider } from "@/context/ToastContext";
import { CopilotSessionProvider } from "@/context/CopilotSessionContext";
import { TooltipProvider } from "@/components/ui/tooltip";
import { MaintenanceBanner } from "@/components/layout/maintenance-banner";

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
            <ToastUIProvider>
              {/* Holds the copilot chat session + in-flight stream above the
                  router so generation survives client-side navigation. */}
              <CopilotSessionProvider>
                <MaintenanceBanner />
                {children}
              </CopilotSessionProvider>
            </ToastUIProvider>
          </TooltipProvider>
        </LanguageProvider>
      </QueryProvider>
    </ThemeProvider>
  );
}
