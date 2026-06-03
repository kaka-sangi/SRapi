"use client";

import { Suspense } from "react";
import { useLanguage } from "@/context/LanguageContext";
import { AmbientCanvas } from "@/components/visual/ambient-canvas";
import { FirstRunRedirect } from "@/components/auth/first-run-redirect";
import { LoginForm } from "@/components/auth/login-form";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";

export default function LandingPage() {
  const { t } = useLanguage();

  return (
    <div className="relative flex min-h-dvh flex-col">
      <FirstRunRedirect />
      <AmbientCanvas />

      {/* top bar */}
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <div className="flex items-baseline gap-2">
          <span className="font-serif text-2xl leading-none text-srapi-text-primary">SRapi</span>
          <span className="font-mono text-2xs text-srapi-text-tertiary">{t("common.version")}</span>
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
          <div className="animate-bloom max-w-lg">
            <div className="mb-8 flex items-center gap-3 font-mono text-2xs uppercase tracking-[0.18em] text-srapi-text-tertiary">
              <span className="h-px w-7 bg-srapi-primary" />
              {t("login.eyebrow")}
            </div>
            <h1 className="font-serif text-5xl font-medium leading-[1.02] text-balance text-srapi-text-primary [word-break:keep-all] sm:text-6xl">
              {t("login.headlineA")}
              <br />
              <span className="italic text-srapi-primary">{t("login.headlineB")}</span>
            </h1>
            <p className="mt-8 max-w-md text-md leading-relaxed text-srapi-text-secondary">
              {t("login.subhead")}
            </p>
            <p className="mt-10 border-t border-srapi-border pt-5 font-mono text-2xs leading-relaxed text-srapi-text-tertiary">
              {t("login.providersLine")}
            </p>
          </div>

          {/* right — the one job of this page */}
          <div className="animate-bloom bloom-delay-1 w-full lg:justify-self-end">
            <div className="mx-auto w-full max-w-sm">
              <Suspense>
                <LoginForm />
              </Suspense>
            </div>
          </div>
        </div>
      </main>

      {/* footer */}
      <footer className="mx-auto w-full max-w-6xl px-6 py-7">
        <div className="flex items-center justify-between border-t border-srapi-border pt-6 font-mono text-2xs text-srapi-text-tertiary">
          <span>© 2026 SRapi</span>
          <span>{t("login.eyebrow")}</span>
        </div>
      </footer>
    </div>
  );
}
