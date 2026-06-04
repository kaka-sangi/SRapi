"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { apiService } from "@/lib/api";
import { useLanguage } from "@/context/LanguageContext";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";

/**
 * Password reset, both halves of the flow on one route:
 *  - no token  → request mode: enter email, backend mails a reset link
 *  - ?token=…  → confirm mode: choose a new password
 * The email-existence response is always neutral (no account enumeration).
 */
export default function ResetPasswordPage() {
  const { t } = useLanguage();
  return (
    <div className="relative flex min-h-dvh flex-col">
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <Link href="/" className="font-serif text-2xl leading-none text-srapi-text-primary">
          SRapi
        </Link>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-6xl flex-1 items-center justify-center px-6 py-10">
        <div className="animate-bloom w-full max-w-sm">
          <Suspense fallback={null}>
            <ResetForm />
          </Suspense>
          <p className="mt-4 text-center font-mono text-2xs text-srapi-text-tertiary">
            {t("authReset.eyebrow")}
          </p>
        </div>
      </main>
    </div>
  );
}

function ResetForm() {
  const { t } = useLanguage();
  const router = useRouter();
  const params = useSearchParams();
  const token = params.get("token") ?? "";
  const isConfirm = token.length > 0;

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function onRequest(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!email) return;
    setSubmitting(true);
    try {
      await apiService.requestPasswordReset(email);
    } catch {
      // Swallow: we show the same neutral confirmation either way so the page
      // never reveals whether an address is registered.
    } finally {
      setSubmitting(false);
      setDone(true);
    }
  }

  async function onConfirm(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (password.length < 8) {
      setError(t("authReset.weakPassword"));
      return;
    }
    setSubmitting(true);
    try {
      await apiService.confirmPasswordReset(token, password);
      setDone(true);
    } catch {
      setError(t("authReset.confirmError"));
    } finally {
      setSubmitting(false);
    }
  }

  // —— Success surfaces ——
  if (done) {
    const confirmed = isConfirm;
    return (
      <Card className="card-raised p-7 sm:p-8">
        <h1 className="font-serif text-2xl text-srapi-text-primary">
          {confirmed ? t("authReset.resetTitle") : t("authReset.sentTitle")}
        </h1>
        <p className="mt-1.5 text-sm leading-relaxed text-srapi-text-secondary">
          {confirmed ? t("authReset.resetBody") : t("authReset.sentBody")}
        </p>
        <Button
          variant={confirmed ? "primary" : "outline"}
          size="lg"
          className="mt-7 w-full"
          onClick={() => router.replace("/")}
        >
          {t("authReset.backToSignIn")}
        </Button>
      </Card>
    );
  }

  // —— Confirm mode (token in URL) ——
  if (isConfirm) {
    return (
      <Card className="card-raised p-7 sm:p-8">
        <h1 className="font-serif text-2xl text-srapi-text-primary">{t("authReset.confirmTitle")}</h1>
        <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("authReset.confirmSubtitle")}</p>
        <form onSubmit={onConfirm} noValidate className="mt-7 space-y-5">
          <div>
            <Label htmlFor="new-password">{t("authReset.newPassword")}</Label>
            <div className="relative">
              <Input
                id="new-password"
                type={showPassword ? "text" : "password"}
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="pr-10"
              />
              <button
                type="button"
                onClick={() => setShowPassword((v) => !v)}
                aria-label={t(showPassword ? "login.hidePassword" : "login.showPassword")}
                className="absolute inset-y-0 right-0 flex w-10 items-center justify-center rounded-r-lg text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
              >
                {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
          </div>
          {error && (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
          )}
          <Button
            type="submit"
            variant="primary"
            size="lg"
            className="w-full"
            disabled={submitting || password.length < 8}
          >
            {submitting ? t("authReset.submitting") : t("authReset.confirmCta")}
          </Button>
        </form>
      </Card>
    );
  }

  // —— Request mode (no token) ——
  return (
    <Card className="card-raised p-7 sm:p-8">
      <h1 className="font-serif text-2xl text-srapi-text-primary">{t("authReset.requestTitle")}</h1>
      <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("authReset.requestSubtitle")}</p>
      <form onSubmit={onRequest} noValidate className="mt-7 space-y-5">
        <div>
          <Label htmlFor="reset-email">{t("login.email")}</Label>
          <Input
            id="reset-email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
          />
        </div>
        <Button
          type="submit"
          variant="primary"
          size="lg"
          className="w-full"
          disabled={submitting || !email}
        >
          {submitting ? t("authReset.submitting") : t("authReset.requestCta")}
        </Button>
        <Button asChild variant="ghost" className="w-full">
          <Link href="/">{t("authReset.backToSignIn")}</Link>
        </Button>
      </form>
    </Card>
  );
}
