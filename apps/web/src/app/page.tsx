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
      <header className="relative z-10 mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <div className="flex items-center gap-3">
          <div className="grid size-9 place-items-center rounded-xl bg-gradient-to-br from-srapi-primary to-srapi-primary-hover text-sm font-semibold text-white shadow-[0_4px_12px_-4px_rgba(194,85,59,0.45)]">
            {siteName.charAt(0)}
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-lg font-semibold tracking-tight text-srapi-text-primary">
              {siteName}
            </span>
            <span className="text-[11px] font-medium text-srapi-text-tertiary">{versionLabel}</span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>

      {/* centered split: modern hero + sign-in card */}
      <main className="relative z-10 mx-auto flex w-full max-w-6xl flex-1 items-center px-6 py-10">
        <div className="grid w-full items-center gap-x-16 gap-y-14 lg:grid-cols-[1.05fr_1fr]">
          {/* left — identity + value props */}
          <div className="max-w-xl">
            <div
              className="anim-rise mb-5 inline-flex items-center gap-2 rounded-full bg-srapi-accent-soft px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-primary"
              style={rise(0)}
            >
              <span className="size-1.5 rounded-full bg-srapi-primary" />
              {t("login.eyebrow")}
            </div>
            <h1
              className="anim-rise text-balance text-4xl font-semibold leading-[1.08] tracking-tight text-srapi-text-primary sm:text-5xl lg:text-[3.5rem]"
              style={rise(1)}
            >
              {t("login.headlineA")}
              <br />
              <span className="text-srapi-primary">{t("login.headlineB")}</span>
            </h1>
            <p
              className="anim-rise mt-6 max-w-lg text-base leading-relaxed text-srapi-text-secondary"
              style={rise(2)}
            >
              {siteSubtitle}
            </p>
            <div
              className="anim-rise mt-8 flex flex-wrap items-center gap-x-4 gap-y-2 text-[12px] font-medium text-srapi-text-tertiary"
              style={rise(3)}
            >
              <span className="inline-flex items-center gap-1.5">
                <span className="size-1.5 rounded-full bg-srapi-success" />
                {t("login.providersLine")}
              </span>
            </div>
          </div>

          {/* right — the one job of this page, wrapped in a strong soft card */}
          <div className="anim-rise w-full lg:justify-self-end" style={rise(2)}>
            <div className="card-raised mx-auto w-full max-w-md rounded-2xl border border-srapi-border bg-srapi-card p-7 sm:p-8">
              <LoginForm />
            </div>
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
