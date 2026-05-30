"use client";

import * as React from "react";
import Link from "next/link";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";
import { useAuthRedirect } from "@/hooks/use-auth-redirect";
import { useSpotlight } from "@/hooks/use-spotlight";
import { useLanguage } from "@/context/LanguageContext";
import { LoginForm } from "@/features/auth/login-form";

/**
 * Focused sign-in page. Same form as the landing, but a calm centered layout
 * for users who navigate straight to `/login`.
 */
export default function LoginPage() {
  useAuthRedirect();
  const { t } = useLanguage();
  const spotlightRef = useSpotlight<HTMLDivElement>();

  return (
    <div
      ref={spotlightRef}
      className="spotlight paper-grain relative flex min-h-screen items-center justify-center overflow-hidden bg-srapi-bg px-6 text-srapi-text-primary"
    >
      <div className="aurora" aria-hidden="true" />

      <div className="absolute right-6 top-6 z-20 flex items-center gap-3">
        <LanguageToggle />
        <ThemeToggle />
      </div>

      <div className="glass-card animate-bloom-soft relative z-10 w-full max-w-md rounded-3xl p-8 md:p-10">
        <Link
          href="/"
          className="font-serif text-2xl font-semibold italic tracking-tight text-srapi-primary"
        >
          SRapi.
        </Link>
        <div className="mb-8 mt-6 space-y-2">
          <h1 className="font-serif text-2xl font-normal tracking-tight text-srapi-text-primary">
            {t("verifyOperator")}
          </h1>
          <p className="text-xs leading-relaxed text-srapi-text-secondary">
            {t("consolePassphraseDesc")}
          </p>
        </div>
        <LoginForm />
      </div>
    </div>
  );
}
