"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { apiService } from "@/lib/api";
import { cn } from "@/lib/cn";
import type { EnabledOAuthProvider } from "@/lib/sdk-types";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { FloatingInput } from "@/components/ui/floating-input";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
import { Kbd } from "@/components/ui/kbd";
import { useCaptcha } from "@/components/auth/captcha";

// Where the provider redirects the browser after the callback creates the
// short-lived pending session; that page finishes the sign-in.
const OAUTH_CALLBACK_PATH = "/auth/oauth/callback";

function startOAuthHref(provider: string, providerKey: string): string {
  const params = new URLSearchParams();
  if (providerKey) params.set("provider_key", providerKey);
  params.set("redirect", OAUTH_CALLBACK_PATH);
  return `/api/v1/auth/oauth/${encodeURIComponent(provider)}/start?${params.toString()}`;
}

// Cheap email shape check — matches "x@y.z" with no spaces. Strict RFC-5322 is
// overkill for inline UX; the server still validates definitively on submit.
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function LoginForm() {
  const router = useRouter();
  const { t } = useLanguage();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [emailTouched, setEmailTouched] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<{ email?: string; password?: string }>({});
  const [submitting, setSubmitting] = useState(false);
  const [providers, setProviders] = useState<EnabledOAuthProvider[]>([]);
  const [passwordlessSent, setPasswordlessSent] = useState(false);
  // When set, the password step succeeded but TOTP is required to finish.
  const [challengeId, setChallengeId] = useState<string | null>(null);
  const [challengeExpiresAt, setChallengeExpiresAt] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const captcha = useCaptcha();

  useEffect(() => {
    let active = true;
    apiService.listOAuthProviders().then((list) => {
      if (active) setProviders(list.filter((item) => !["wechat", "dingtalk"].includes(item.provider)));
    });
    return () => {
      active = false;
    };
  }, []);

  function goHome(role: string) {
    // Read ?from directly off the URL instead of next/navigation's
    // useSearchParams(): that hook marks the component as dynamic and bails the
    // server render out to client-side rendering, which Next 16 + Turbopack
    // fails to recover — leaving the sign-in page blank. goHome only runs after
    // a submit handler (browser-only), so window is always defined here.
    const from = new URLSearchParams(window.location.search).get("from");
    const home = role === "admin" ? ADMIN_HOME_ROUTE : USER_HOME_ROUTE;
    router.replace(from && from.startsWith("/") ? from : home);
  }

  // Inline validation for the email shape — only "complains" after the field
  // has been touched (blurred), so the user isn't yelled at while typing.
  const emailLooksValid = EMAIL_RE.test(email.trim());
  const inlineEmailError = emailTouched && email.length > 0 && !emailLooksValid
    ? t("login.errRequired")
    : undefined;

  const formValid = emailLooksValid && password.length > 0 && !(captcha.required && !captcha.token);
  const formDirty = email.length > 0 || password.length > 0;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setFieldErrors({});
    if (!email || !password) {
      const next: { email?: string; password?: string } = {};
      if (!email) next.email = t("login.errRequired");
      if (!password) next.password = t("login.errRequired");
      setFieldErrors(next);
      setError(t("login.errRequired"));
      return;
    }
    setSubmitting(true);
    try {
      const result = await apiService.login(email, password, captcha.token);
      if (result.kind === "twoFactor") {
        setChallengeId(result.challengeId);
        setChallengeExpiresAt(result.expiresAt);
        setCode("");
      } else {
        goHome(result.user.role);
      }
    } catch {
      setError(t("login.errWrong"));
    } finally {
      setSubmitting(false);
    }
  }

  async function requestPasswordless() {
    setError(null);
    if (!email) {
      setFieldErrors({ email: t("login.errRequired") });
      setError(t("login.errRequired"));
      return;
    }
    setSubmitting(true);
    try {
      await apiService.requestPasswordlessCode(email, undefined, [], captcha.token);
      setPasswordlessSent(true);
    } catch {
      setError(t("login.errWrong"));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault();
    if (!challengeId) return;
    if (challengeExpiresAt && Date.parse(challengeExpiresAt) <= Date.now()) {
      // Challenge already expired — return to the password step with a clear
      // reason instead of submitting an expired code for a generic "2fa failed".
      setChallengeId(null);
      setChallengeExpiresAt(null);
      setCode("");
      setError(t("login.err2faExpired"));
      return;
    }
    setError(null);
    setSubmitting(true);
    try {
      const user = await apiService.verifyLoginTwoFactor(challengeId, code.trim());
      goHome(user.role);
    } catch {
      setError(t("login.err2fa"));
    } finally {
      setSubmitting(false);
    }
  }

  // ---- Two-factor step ----
  if (challengeId) {
    const codeReady = code.length === 6;
    return (
      <Card className="p-7 sm:p-8">
        <h2 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">{t("login.twoFactorTitle")}</h2>
        <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("login.twoFactorHint")}</p>
        <form onSubmit={handleVerify} noValidate className="mt-7 space-y-5">
          <div>
            <Label htmlFor="totp">{t("login.code")}</Label>
            <Input
              id="totp"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              placeholder="000000"
              className={cn(
                "text-center font-mono text-lg tracking-[0.4em]",
                error && "anim-shake",
              )}
            />
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
            disabled={submitting || !codeReady}
          >
            <span className="inline-flex items-center gap-2">
              {submitting ? t("login.verifying") : t("login.verify")}
              {codeReady && !submitting ? <Kbd className="bg-white/10 border-white/20 text-white/90 shadow-none">↵</Kbd> : null}
            </span>
          </Button>
          <button
            type="button"
            onClick={() => {
              setChallengeId(null);
              setChallengeExpiresAt(null);
              setError(null);
            }}
            className="w-full text-center text-xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
          >
            {t("login.back")}
          </button>
        </form>
      </Card>
    );
  }

  // ---- Password + OAuth step ----
  return (
    <Card className="p-7 sm:p-8">
      <h2 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">{t("login.title")}</h2>
      <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("login.subtitle")}</p>

      <form onSubmit={handleSubmit} noValidate className="mt-7 space-y-5">
        <div onBlur={() => setEmailTouched(true)}>
          <FloatingInput
            id="email"
            label={t("login.email")}
            type="email"
            autoComplete="email"
            value={email}
            onChange={(v) => {
              setEmail(v);
              if (fieldErrors.email) setFieldErrors((prev) => ({ ...prev, email: undefined }));
            }}
            error={fieldErrors.email ?? inlineEmailError}
          />
        </div>
        <div>
          <div className="relative">
            <FloatingInput
              id="password"
              label={t("login.password")}
              type={showPassword ? "text" : "password"}
              autoComplete="current-password"
              value={password}
              onChange={(v) => {
                setPassword(v);
                if (fieldErrors.password) setFieldErrors((prev) => ({ ...prev, password: undefined }));
              }}
              error={fieldErrors.password}
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
          <div className="mt-1 flex justify-end">
            <a
              href="/auth/reset"
              className="text-xs text-srapi-text-tertiary underline-offset-2 transition-colors hover:text-srapi-text-secondary hover:underline"
            >
              {t("login.forgot")}
            </a>
          </div>
        </div>
        {error && !fieldErrors.email && !fieldErrors.password && (
          <p role="alert" className="anim-shake rounded-xl bg-srapi-error/10 px-3 py-2 text-sm text-srapi-error">
            {error}
          </p>
        )}
        {captcha.node}
        <Button
          type="submit"
          variant="primary"
          size="lg"
          className="h-11 w-full rounded-xl btn-raise"
          disabled={submitting || (captcha.required && !captcha.token)}
        >
          <span className="inline-flex items-center gap-2">
            {submitting ? t("login.signingIn") : t("login.signIn")}
            {formValid && formDirty && !submitting ? (
              <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
            ) : null}
          </span>
        </Button>
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="h-11 w-full rounded-xl"
          disabled={submitting || !email || (captcha.required && !captcha.token)}
          onClick={requestPasswordless}
        >
          Send email sign-in link
        </Button>
        {passwordlessSent ? (
          <p className="text-center text-xs text-srapi-text-secondary">
            Check your email for a one-time sign-in link.
          </p>
        ) : null}
      </form>

      {providers.length > 0 && (
        <>
          <div className="my-6 flex items-center gap-3">
            <span className="h-px flex-1 bg-srapi-border" />
            <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("login.orContinueWith")}
            </span>
            <span className="h-px flex-1 bg-srapi-border" />
          </div>
          <div className="flex flex-wrap gap-2">
            {providers.map((p) => (
              <a
                key={`${p.provider}:${p.provider_key}`}
                href={startOAuthHref(p.provider, p.provider_key)}
                className={cn(
                  "inline-flex h-10 flex-1 min-w-[8rem] items-center justify-center gap-2 rounded-full border px-4 text-[13px] font-medium transition-colors",
                  p.provider === "linuxdo"
                    ? "border-amber-300/70 bg-amber-50 text-amber-900 hover:bg-amber-100 dark:border-amber-700/60 dark:bg-amber-950/30 dark:text-amber-200 dark:hover:bg-amber-900/40"
                    : p.provider === "github"
                      ? "border-srapi-border bg-srapi-card-soft text-srapi-text-primary hover:bg-srapi-card-muted"
                      : "border-srapi-border bg-srapi-card-soft text-srapi-text-primary hover:bg-srapi-card-muted",
                )}
              >
                {p.provider === "linuxdo" ? (
                  <span className="text-xs font-bold">L</span>
                ) : null}
                {t("login.continueWith", { name: p.display_name })}
              </a>
            ))}
          </div>
        </>
      )}

      <p className="mt-6 text-center text-sm text-srapi-text-secondary">
        {t("login.noAccount")}{" "}
        <a href="/auth/register" className="text-srapi-primary underline-offset-4 hover:underline">
          {t("login.signUp")}
        </a>
      </p>
      <p className="mt-2 text-center text-xs text-srapi-text-tertiary">
        <a href="/key-usage" className="underline-offset-4 hover:text-srapi-text-secondary hover:underline">
          {t("login.keyUsageLink")}
        </a>
      </p>
    </Card>
  );
}
