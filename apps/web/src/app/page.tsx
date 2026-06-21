"use client";

import type { CSSProperties } from "react";
import { useLanguage } from "@/context/LanguageContext";
import { AmbientCanvas } from "@/components/visual/ambient-canvas";
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
    <div className="relative flex min-h-dvh flex-col">
      <FirstRunRedirect />
      <AmbientCanvas />

      {/* top bar — chrome stays static so the toggles are instantly usable */}
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <div className="flex items-baseline gap-2">
          <span className="font-serif text-2xl leading-none text-srapi-text-primary">{siteName}</span>
          <span className="font-mono text-2xs text-srapi-text-tertiary">{versionLabel}</span>
        </div>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

      {/* centered split: editorial narrative + sign-in card */}
      <main className="mx-auto flex w-full max-w-6xl flex-1 items-center px-6 py-10">
        <div className="grid w-full items-center gap-x-20 gap-y-14 lg:grid-cols-2">
          {/* left — identity only, no operational data */}
          <div className="max-w-lg">
            <div
              className="anim-rise mb-8 flex items-center gap-3 font-mono text-2xs uppercase tracking-[0.18em] text-srapi-text-tertiary"
              style={rise(0)}
            >
              <span className="anim-rule h-px w-8 origin-left bg-srapi-primary" style={rise(0)} />
              {t("login.eyebrow")}
            </div>
            <h1
              className="anim-rise font-serif text-hero font-medium text-balance text-srapi-text-primary [word-break:keep-all]"
              style={rise(1)}
            >
              {t("login.headlineA")}
              <br />
              <span className="italic text-srapi-primary">{t("login.headlineB")}</span>
            </h1>
            <p
              className="anim-rise mt-8 max-w-md text-md leading-relaxed text-srapi-text-secondary"
              style={rise(2)}
            >
              {siteSubtitle}
            </p>
            <p
              className="anim-rise mt-10 border-t border-srapi-border pt-5 font-mono text-2xs leading-relaxed text-srapi-text-tertiary"
              style={rise(3)}
            >
              {t("login.providersLine")}
            </p>
          </div>

          {/* right — the one job of this page */}
          <div className="anim-rise w-full lg:justify-self-end" style={rise(2)}>
            <div className="mx-auto w-full max-w-sm">
              <LoginForm />
            </div>
          </div>
        </div>
      </main>

      {/* footer */}
      <footer className="anim-rise mx-auto w-full max-w-6xl px-6 py-7" style={rise(4)}>
        <div className="flex items-center justify-between border-t border-srapi-border pt-6 font-mono text-2xs text-srapi-text-tertiary">
          <span>© 2026 {siteName}</span>
          <span className="flex items-center gap-3">
            {site?.doc_url ? (
              <a href={site.doc_url} className="underline-offset-4 hover:text-srapi-text-secondary hover:underline">
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
