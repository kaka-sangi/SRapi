"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { apiService } from "@/lib/api";
import type { EnabledOAuthProvider } from "@/lib/sdk-types";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
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

export function LoginForm() {
  const router = useRouter();
  const { t } = useLanguage();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [providers, setProviders] = useState<EnabledOAuthProvider[]>([]);
  const [passwordlessSent, setPasswordlessSent] = useState(false);
  // When set, the password step succeeded but TOTP is required to finish.
  const [challengeId, setChallengeId] = useState<string | null>(null);
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

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!email || !password) {
      setError(t("login.errRequired"));
      return;
    }
    setSubmitting(true);
    try {
      const result = await apiService.login(email, password, captcha.token);
      if (result.kind === "twoFactor") {
        setChallengeId(result.challengeId);
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
    return (
      <Card className="card-raised p-7 sm:p-8">
        <h2 className="font-serif text-2xl text-srapi-text-primary">{t("login.twoFactorTitle")}</h2>
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
              className="text-center font-mono text-lg tracking-[0.4em]"
            />
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
            disabled={submitting || code.length < 6}
          >
            {submitting ? t("login.verifying") : t("login.verify")}
          </Button>
          <button
            type="button"
            onClick={() => {
              setChallengeId(null);
              setError(null);
            }}
            className="w-full text-center font-mono text-2xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
          >
            {t("login.back")}
          </button>
        </form>
      </Card>
    );
  }

  // ---- Password + OAuth step ----
  return (
    <Card className="card-raised p-7 sm:p-8">
      <h2 className="font-serif text-2xl text-srapi-text-primary">{t("login.title")}</h2>
      <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("login.subtitle")}</p>

      <form onSubmit={handleSubmit} noValidate className="mt-7 space-y-5">
        <div>
          <Label htmlFor="email">{t("login.email")}</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
          />
        </div>
        <div>
          <div className="flex items-baseline justify-between">
            <Label htmlFor="password">{t("login.password")}</Label>
            <a
              href="/auth/reset"
              className="font-mono text-2xs text-srapi-text-tertiary underline-offset-2 transition-colors hover:text-srapi-text-secondary hover:underline"
            >
              {t("login.forgot")}
            </a>
          </div>
          <div className="relative">
            <Input
              id="password"
              type={showPassword ? "text" : "password"}
              autoComplete="current-password"
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
        {captcha.node}
        <Button
          type="submit"
          variant="primary"
          size="lg"
          className="w-full"
          disabled={submitting || (captcha.required && !captcha.token)}
        >
          {submitting ? t("login.signingIn") : t("login.signIn")}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="w-full"
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
            <span className="font-mono text-2xs uppercase tracking-wide text-srapi-text-tertiary">
              {t("login.orContinueWith")}
            </span>
            <span className="h-px flex-1 bg-srapi-border" />
          </div>
          <div className="space-y-2.5">
            {providers.map((p) => (
              <a
                key={`${p.provider}:${p.provider_key}`}
                href={startOAuthHref(p.provider, p.provider_key)}
                className="flex h-11 w-full items-center justify-center rounded-lg border border-srapi-border-strong bg-srapi-card text-sm font-medium text-srapi-text-primary transition-colors hover:border-srapi-text-tertiary hover:bg-srapi-card-muted"
              >
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
      <p className="mt-2 text-center text-2xs text-srapi-text-tertiary">
        <a href="/key-usage" className="underline-offset-4 hover:text-srapi-text-secondary hover:underline">
          {t("login.keyUsageLink")}
        </a>
      </p>
    </Card>
  );
}
