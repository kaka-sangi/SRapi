"use client";

import { useMemo, useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { apiService } from "@/lib/api";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { Card } from "@/components/ui/card";
import { FloatingInput } from "@/components/ui/floating-input";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

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
        <Link href="/" className="text-2xl font-semibold tracking-tight leading-none text-srapi-text-primary">
          SRapi
        </Link>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-6xl flex-1 items-center justify-center px-6 py-10">
        <div className="animate-bloom w-full max-w-sm">
          <ResetForm />
          <p className="mt-4 text-center text-xs font-medium uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
  // Read ?token off the URL rather than via useSearchParams(): that hook bails
  // the server render out to client-side rendering, which Next 16 + Turbopack
  // fails to recover, leaving the page blank. useSyncExternalStore yields "" for
  // the server + first client render (request mode, so hydration matches), then
  // re-renders with the real query string, flipping to confirm mode.
  const search = useSyncExternalStore(
    () => () => {},
    () => window.location.search,
    () => "",
  );
  const token = new URLSearchParams(search).get("token") ?? "";
  const isConfirm = token.length > 0;

  const [email, setEmail] = useState("");
  const [emailTouched, setEmailTouched] = useState(false);
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const emailLooksValid = EMAIL_RE.test(email.trim());
  const emailError = emailTouched && email.length > 0 && !emailLooksValid
    ? t("login.errRequired")
    : undefined;

  const passwordLongEnough = password.length >= 8;
  const passwordError = password.length > 0 && !passwordLongEnough
    ? t("authReset.weakPassword")
    : undefined;

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
      <Card className="p-7 sm:p-8">
        <h1 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">
          {confirmed ? t("authReset.resetTitle") : t("authReset.sentTitle")}
        </h1>
        <p className="mt-1.5 text-sm leading-relaxed text-srapi-text-secondary">
          {confirmed ? t("authReset.resetBody") : t("authReset.sentBody")}
        </p>
        <Button
          variant={confirmed ? "primary" : "outline"}
          size="lg"
          className="mt-7 h-11 w-full rounded-xl btn-raise"
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
      <Card className="p-7 sm:p-8">
        <h1 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">{t("authReset.confirmTitle")}</h1>
        <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("authReset.confirmSubtitle")}</p>
        <form onSubmit={onConfirm} noValidate className="mt-7 space-y-5">
          <div>
            <div className="relative">
              <FloatingInput
                id="new-password"
                label={t("authReset.newPassword")}
                type={showPassword ? "text" : "password"}
                autoComplete="new-password"
                value={password}
                onChange={setPassword}
                error={passwordError}
                className="[&_input]:pr-12"
              />
              <button
                type="button"
                onClick={() => setShowPassword((v) => !v)}
                aria-label={t(showPassword ? "login.hidePassword" : "login.showPassword")}
                className="absolute right-0 top-0 flex h-14 w-12 items-center justify-center rounded-r-2xl text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
              >
                {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
            {password.length > 0 ? <PasswordStrength password={password} /> : null}
          </div>
          {error && (
            <p role="alert" className="anim-shake rounded-xl bg-srapi-error/10 px-3 py-2 text-sm text-srapi-error">
              {error}
            </p>
          )}
          <Button
            type="submit"
            variant="primary"
            size="lg"
            className="h-11 w-full rounded-xl btn-raise"
            loading={submitting}
            disabled={submitting || password.length < 8}
          >
            <span className="inline-flex items-center gap-2">
              {submitting ? t("authReset.submitting") : t("authReset.confirmCta")}
              {passwordLongEnough && !submitting ? (
                <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
              ) : null}
            </span>
          </Button>
        </form>
      </Card>
    );
  }

  // —— Request mode (no token) ——
  return (
    <Card className="p-7 sm:p-8">
      <h1 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">{t("authReset.requestTitle")}</h1>
      <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("authReset.requestSubtitle")}</p>
      <form onSubmit={onRequest} noValidate className="mt-7 space-y-5">
        <div onBlur={() => setEmailTouched(true)}>
          <FloatingInput
            id="reset-email"
            label={t("login.email")}
            type="email"
            autoComplete="email"
            value={email}
            onChange={setEmail}
            error={emailError}
          />
        </div>
        <Button
          type="submit"
          variant="primary"
          size="lg"
          className="h-11 w-full rounded-xl btn-raise"
          loading={submitting}
          disabled={submitting || !emailLooksValid}
        >
          <span className="inline-flex items-center gap-2">
            {submitting ? t("authReset.submitting") : t("authReset.requestCta")}
            {emailLooksValid && !submitting ? (
              <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
            ) : null}
          </span>
        </Button>
        <Button asChild variant="ghost" className="h-11 w-full rounded-xl">
          <Link href="/">{t("authReset.backToSignIn")}</Link>
        </Button>
      </form>
    </Card>
  );
}

function PasswordStrength({ password }: { password: string }) {
  const strength = useMemo(() => {
    let score = 0;
    if (password.length >= 8) score++;
    if (password.length >= 12) score++;
    if (/[a-z]/.test(password) && /[A-Z]/.test(password)) score++;
    if (/\d/.test(password)) score++;
    if (/[^a-zA-Z0-9]/.test(password)) score++;
    return Math.min(score, 4);
  }, [password]);

  const color =
    strength <= 1
      ? "bg-srapi-error"
      : strength === 2
        ? "bg-srapi-warning"
        : "bg-srapi-success";

  return (
    <div className="mt-2 flex gap-1" aria-hidden>
      {Array.from({ length: 4 }).map((_, i) => (
        <div
          key={i}
          className={cn(
            "h-1 flex-1 rounded-full transition-colors",
            i < strength ? color : "bg-srapi-border",
          )}
        />
      ))}
    </div>
  );
}
