"use client";

import dynamic from "next/dynamic";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";
import { useAuthRedirect } from "@/hooks/use-auth-redirect";
import { useSpotlight } from "@/hooks/use-spotlight";
import { useLanguage } from "@/context/LanguageContext";
import { LoginForm } from "@/features/auth/login-form";
import { Hero } from "./hero";
import { FeatureBento } from "./feature-bento";
import { LiveStatus } from "./live-status";

// Code-split + client-only: the ambient canvas never ships in the main bundle
// and never runs on the server.
const AmbientCanvas = dynamic(() => import("@/components/visual/ambient-canvas"), {
  ssr: false,
  loading: () => null,
});

export default function Landing() {
  useAuthRedirect();
  const { t } = useLanguage();
  const spotlightRef = useSpotlight<HTMLDivElement>();

  return (
    <div
      ref={spotlightRef}
      className="spotlight paper-grain relative min-h-screen overflow-hidden bg-srapi-bg text-srapi-text-primary"
    >
      <div className="aurora" aria-hidden="true" />
      <div className="pointer-events-none absolute inset-0 z-0 opacity-50 dark:opacity-30">
        <AmbientCanvas />
      </div>

      <header className="relative z-20 mx-auto flex max-w-6xl items-center justify-between px-6 py-6 md:px-10">
        <div className="flex items-center gap-3">
          <span className="font-serif text-2xl font-semibold italic tracking-tight text-srapi-primary">
            SRapi.
          </span>
          <span className="rounded-full border border-srapi-border bg-srapi-card-muted/50 px-2 py-0.5 font-mono text-2xs font-bold uppercase tracking-wider text-srapi-text-secondary">
            v0.1.0
          </span>
          <LiveStatus className="hidden sm:inline-flex" />
        </div>
        <div className="flex items-center gap-3">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

      <main className="relative z-10 mx-auto grid max-w-6xl gap-12 px-6 pb-20 pt-6 md:px-10 lg:grid-cols-[1.05fr_0.95fr] lg:gap-16 lg:pb-28 lg:pt-12">
        <section className="animate-bloom flex flex-col justify-center gap-10">
          <Hero />
          <FeatureBento />
          <p className="font-mono text-2xs text-srapi-text-secondary">{t("mktFooter")}</p>
        </section>

        <section id="login" className="flex items-center">
          <div className="glass-card animate-bloom-soft mx-auto w-full max-w-md rounded-3xl p-8 md:p-10">
            <div className="mb-8 space-y-2">
              <h2 className="font-serif text-2xl font-normal tracking-tight text-srapi-text-primary">
                {t("verifyOperator")}
              </h2>
              <p className="text-xs leading-relaxed text-srapi-text-secondary">
                {t("consolePassphraseDesc")}
              </p>
            </div>
            <LoginForm />
          </div>
        </section>
      </main>
    </div>
  );
}
