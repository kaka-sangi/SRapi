"use client";

import type { CSSProperties } from "react";
import { ShieldCheck, Zap, Sparkles } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { AuroraBackdrop } from "@/components/visual/aurora-backdrop";
import { SpotlightCard } from "@/components/visual/spotlight-card";
import { BrandMark } from "@/components/visual/brand-mark";
import { FirstRunRedirect } from "@/components/auth/first-run-redirect";
import { LoginForm } from "@/components/auth/login-form";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";
import { useSiteConfig } from "@/hooks/queries";

// Orchestrated entrance: each element rises with a spring, sequenced by index.
// The stagger lives in CSS (--stagger-index → animation-delay), so it stays on
// the compositor and respects prefers-reduced-motion centrally.
const rise = (i: number) => ({ "--stagger-index": i }) as CSSProperties;

export default function LandingPage() {
  const { t } = useLanguage();
  const siteConfig = useSiteConfig();
  const site = siteConfig.data;
  const siteName = site?.site_name?.trim() || "SRapi";
  const siteSubtitle = site?.site_subtitle?.trim() || t("login.subhead");
  const versionLabel = site?.version_label?.trim() || t("common.version");

  return (
    <div className="relative flex min-h-dvh flex-col overflow-hidden">
      <FirstRunRedirect />
      {/* Aurora ambient field — slow-drifting blobs build a «breathing» light
          environment on top of the cream canvas. Replaces the previous
          one-vignette restraint. */}
      <AuroraBackdrop tone="hero" className="-z-10" />
      {/* A diagonal dot grid in the upper-right adds a hint of editorial
          mesh without competing with the aurora. */}
      <div
        className="dot-grid-overlay pointer-events-none absolute right-0 top-0 -z-10 h-72 w-72 opacity-50"
        aria-hidden
      />

      {/* top bar */}
      <header className="relative z-10 mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <div className="group flex items-center gap-3">
          <BrandMark size={36} className="magnetic-icon" />
          <div className="flex items-baseline gap-2">
            <span className="text-lg font-semibold tracking-tight text-srapi-text-primary">
              {siteName}
            </span>
            <span className="rounded-full bg-srapi-card/70 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary backdrop-blur-sm">
              {versionLabel}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

      {/* hero split: editorial left + spotlight login card right */}
      <main className="relative z-10 mx-auto flex w-full max-w-6xl flex-1 items-center px-6 py-10">
        <div className="grid w-full items-center gap-x-16 gap-y-14 lg:grid-cols-[1.05fr_1fr]">
          {/* left — identity + value props */}
          <div className="max-w-xl">
            <div
              className="mb-5 inline-flex items-center gap-2 rounded-full border border-srapi-border bg-srapi-card/80 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-srapi-primary backdrop-blur-sm"
              style={rise(0)}
            >
              <Sparkles className="size-3" aria-hidden />
              {t("login.eyebrow")}
            </div>

            <h1
              className="text-balance text-4xl leading-[1.05] tracking-tight sm:text-5xl lg:text-[3.75rem]"
              style={rise(1)}
            >
              <span className="font-semibold text-srapi-text-primary">
                {t("login.headlineA")}
              </span>
              <br />
              <span className="text-aurora font-semibold">{t("login.headlineB")}</span>
            </h1>

            <p
              className="mt-6 max-w-lg text-base leading-relaxed text-srapi-text-secondary"
              style={rise(2)}
            >
              {siteSubtitle}
            </p>

            {/* Trust badges row — soft chips with icons */}
            <div className="mt-8 flex flex-wrap gap-2.5" style={rise(3)}>
              <TrustChip icon={<ShieldCheck className="size-3.5" />} label={t("login.providersLine")} />
              <TrustChip icon={<Zap className="size-3.5" />} label="OpenAI · Claude · Gemini" />
            </div>
          </div>

          {/* right — login card with mouse-following spotlight */}
          <div className="w-full lg:justify-self-end" style={rise(2)}>
            <SpotlightCard className="mx-auto w-full max-w-md rounded-xl border border-srapi-border bg-srapi-card/95 p-7 backdrop-blur-md sm:p-8">
              <LoginForm />
            </SpotlightCard>
          </div>
        </div>
      </main>

      {/* footer */}
      <footer className="relative z-10 mx-auto w-full max-w-6xl px-6 py-7" style={rise(4)}>
        <div className="flex items-center justify-between border-t border-srapi-border pt-6 text-[12px] text-srapi-text-tertiary">
          <span>© 2026 {siteName}</span>
          <span className="flex items-center gap-3">
            {site?.doc_url ? (
              <a
                href={site.doc_url}
                className="font-medium underline-offset-4 hover:text-srapi-primary hover:underline"
              >
                {t("login.docsLink")}
              </a>
            ) : null}
            {site?.contact_info ? <span>{site.contact_info}</span> : <span>{t("login.eyebrow")}</span>}
          </span>
        </div>
      </footer>
    </div>
  );
}

function TrustChip({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-srapi-border bg-srapi-card/85 px-2.5 py-1 text-[11px] font-medium text-srapi-text-secondary backdrop-blur-sm">
      <span className="text-srapi-primary">{icon}</span>
      {label}
    </span>
  );
}
