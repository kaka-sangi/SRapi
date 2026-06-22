"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { apiService } from "@/lib/api";
import type { OAuthPendingSession } from "@/lib/sdk-types";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE, SIGN_IN_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { FloatingInput } from "@/components/ui/floating-input";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";
import { Kbd } from "@/components/ui/kbd";

type Phase = "loading" | "bind" | "twofactor" | "create" | "email" | "error";

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export default function OAuthCallbackPage() {
  const router = useRouter();
  const { t } = useLanguage();
  const [phase, setPhase] = useState<Phase>("loading");
  const [pending, setPending] = useState<OAuthPendingSession | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Step inputs.
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [name, setName] = useState("");
  const [challengeId, setChallengeId] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [token, setToken] = useState("");
  const [codeSent, setCodeSent] = useState(false);

  const goHome = useCallback(
    (role: string) => router.replace(role === "admin" ? ADMIN_HOME_ROUTE : USER_HOME_ROUTE),
    [router],
  );

  const inspect = useCallback(async () => {
    setPhase("loading");
    setError(null);
    try {
      const session = await apiService.getOAuthPending();
      setPending(session);
      setEmail(session.profile.resolved_email || "");
      setName(session.profile.display_name || "");
      switch (session.next_step) {
        case "ready_for_login":
        case "bind_existing_login_required":
          setPhase("bind");
          break;
        case "create_account_required":
          setPhase("create");
          break;
        case "email_completion_required":
          setPhase("email");
          break;
        default:
          setError(t("oauthCallback.expired"));
          setPhase("error");
      }
    } catch {
      try {
        const user = await apiService.getLiveCurrentUser();
        goHome(user.role);
      } catch {
        setError(t("oauthCallback.expired"));
        setPhase("error");
      }
    }
  }, [goHome, t]);

  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void inspect();
  }, [inspect]);

  const emailLooksValid = EMAIL_RE.test(email.trim());

  async function handleBind(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const result = await apiService.bindOAuthPendingLogin(email.trim(), password, true);
      if (result.kind === "twoFactor") {
        setChallengeId(result.challengeId);
        setPhase("twofactor");
      } else {
        goHome(result.user.role);
      }
    } catch {
      setError(t("oauthCallback.bindFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function handleVerify2fa(e: React.FormEvent) {
    e.preventDefault();
    if (!challengeId) return;
    setBusy(true);
    setError(null);
    try {
      const user = await apiService.verifyOAuthBindLoginTwoFactor(challengeId, code.trim());
      goHome(user.role);
    } catch {
      setError(t("login.err2fa"));
    } finally {
      setBusy(false);
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    const actionToken = pending?.create_account_action?.token;
    if (!actionToken) {
      setError(t("oauthCallback.expired"));
      setPhase("error");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const accountEmail = pending?.profile.resolved_email || email.trim();
      const user = await apiService.createOAuthPendingAccount(
        accountEmail,
        password,
        actionToken,
        name.trim() || undefined,
      );
      goHome(user.role);
    } catch {
      setError(t("oauthCallback.createFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function handleSendCode(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await apiService.sendOAuthEmailCode(email.trim());
      setCodeSent(true);
    } catch {
      setError(t("oauthCallback.sendFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function handleConfirmEmail(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await apiService.confirmOAuthEmailCompletion(token.trim());
      await inspect(); // advances next_step (create / bind)
    } catch {
      setError(t("oauthCallback.confirmFailed"));
    } finally {
      setBusy(false);
    }
  }

  const bindReady = emailLooksValid && password.length > 0;
  const codeReady = code.length === 6;
  const createReady = password.length >= 8;
  const tokenReady = token.trim().length > 0;

  return (
    <div className="grid min-h-dvh place-items-center bg-srapi-bg px-4">
      <div className="w-full max-w-sm">
        {phase === "loading" && (
          <Card className="flex flex-col items-center gap-3 p-8 text-center">
            <Spinner className="size-5 text-srapi-text-tertiary" />
            <p className="text-sm text-srapi-text-secondary">{t("oauthCallback.signingIn")}</p>
          </Card>
        )}

        {phase === "bind" && (
          <Card className="p-7 sm:p-8">
            <h2 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">
              {t("oauthCallback.bindTitle")}
            </h2>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("oauthCallback.bindHint")}</p>
            <form onSubmit={handleBind} noValidate className="mt-7 space-y-5">
              <FloatingInput
                id="email"
                label={t("oauthCallback.emailLabel")}
                type="email"
                autoComplete="email"
                value={email}
                onChange={setEmail}
              />
              <div className="relative">
                <FloatingInput
                  id="password"
                  label={t("oauthCallback.password")}
                  type={showPassword ? "text" : "password"}
                  autoComplete="current-password"
                  value={password}
                  onChange={setPassword}
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
              {error && (
                <p role="alert" className="anim-shake rounded-xl bg-srapi-error/10 px-3 py-2 text-sm text-srapi-error">
                  {error}
                </p>
              )}
              <Button
                type="submit"
                variant="primary"
                size="lg"
                className="h-11 w-full rounded-xl"
                loading={busy}
                disabled={busy || !bindReady}
              >
                <span className="inline-flex items-center gap-2">
                  {busy ? t("login.signingIn") : t("oauthCallback.linkAndSignIn")}
                  {bindReady && !busy ? (
                    <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
                  ) : null}
                </span>
              </Button>
            </form>
          </Card>
        )}

        {phase === "twofactor" && (
          <Card className="p-7 sm:p-8">
            <h2 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">
              {t("login.twoFactorTitle")}
            </h2>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("login.twoFactorHint")}</p>
            <form onSubmit={handleVerify2fa} noValidate className="mt-7 space-y-5">
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
                className="h-11 w-full rounded-xl"
                loading={busy}
                disabled={busy || !codeReady}
              >
                <span className="inline-flex items-center gap-2">
                  {busy ? t("login.verifying") : t("login.verify")}
                  {codeReady && !busy ? (
                    <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
                  ) : null}
                </span>
              </Button>
            </form>
          </Card>
        )}

        {phase === "create" && (
          <Card className="p-7 sm:p-8">
            <h2 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">
              {t("oauthCallback.createTitle")}
            </h2>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">
              {t("oauthCallback.createHint")}
            </p>
            <form onSubmit={handleCreate} noValidate className="mt-7 space-y-5">
              <FloatingInput
                id="email"
                label={t("oauthCallback.emailLabel")}
                type="email"
                value={pending?.profile.resolved_email || email}
                onChange={() => {}}
                disabled
              />
              <FloatingInput
                id="name"
                label={t("oauthCallback.displayName")}
                value={name}
                onChange={setName}
              />
              <div>
                <div className="relative">
                  <FloatingInput
                    id="password"
                    label={t("oauthCallback.password")}
                    type={showPassword ? "text" : "password"}
                    autoComplete="new-password"
                    value={password}
                    onChange={setPassword}
                    hint={password.length === 0 ? t("authRegister.passwordHint") : undefined}
                    error={password.length > 0 && !createReady ? t("authRegister.passwordHint") : undefined}
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
                className="h-11 w-full rounded-xl"
                loading={busy}
                disabled={busy || !createReady}
              >
                <span className="inline-flex items-center gap-2">
                  {busy ? t("common.loading") : t("oauthCallback.createAccount")}
                  {createReady && !busy ? (
                    <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
                  ) : null}
                </span>
              </Button>
            </form>
          </Card>
        )}

        {phase === "email" && (
          <Card className="p-7 sm:p-8">
            <h2 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">
              {t("oauthCallback.emailTitle")}
            </h2>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">
              {t("oauthCallback.emailHint")}
            </p>
            {!codeSent ? (
              <form onSubmit={handleSendCode} noValidate className="mt-7 space-y-5">
                <FloatingInput
                  id="email"
                  label={t("oauthCallback.emailLabel")}
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={setEmail}
                />
                {error && (
                  <p role="alert" className="anim-shake text-sm text-srapi-error">
                    {error}
                  </p>
                )}
                <Button
                  type="submit"
                  variant="primary"
                  size="lg"
                  className="h-11 w-full rounded-xl"
                  loading={busy}
                  disabled={busy || !emailLooksValid}
                >
                  <span className="inline-flex items-center gap-2">
                    {busy ? t("oauthCallback.sending") : t("oauthCallback.sendCode")}
                    {emailLooksValid && !busy ? (
                      <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
                    ) : null}
                  </span>
                </Button>
              </form>
            ) : (
              <form onSubmit={handleConfirmEmail} noValidate className="mt-7 space-y-5">
                <FloatingInput
                  id="token"
                  label={t("oauthCallback.tokenLabel")}
                  value={token}
                  onChange={setToken}
                  hint={t("oauthCallback.tokenHint", { email: email.trim() })}
                />
                {error && (
                  <p role="alert" className="anim-shake text-sm text-srapi-error">
                    {error}
                  </p>
                )}
                <Button
                  type="submit"
                  variant="primary"
                  size="lg"
                  className="h-11 w-full rounded-xl"
                  loading={busy}
                  disabled={busy || !tokenReady}
                >
                  <span className="inline-flex items-center gap-2">
                    {busy ? t("common.loading") : t("oauthCallback.confirm")}
                    {tokenReady && !busy ? (
                      <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
                    ) : null}
                  </span>
                </Button>
              </form>
            )}
          </Card>
        )}

        {phase === "error" && (
          <Card className="p-7 text-center sm:p-8">
            <h2 className="text-xl font-semibold tracking-tight text-srapi-text-primary">
              {t("oauthCallback.errorTitle")}
            </h2>
            <p className="mt-2 text-sm text-srapi-text-secondary">{error}</p>
            <Button
              variant="outline"
              size="lg"
              className="mt-6 h-11 w-full rounded-xl"
              onClick={() => router.replace(SIGN_IN_ROUTE)}
            >
              {t("oauthCallback.backToSignIn")}
            </Button>
          </Card>
        )}
      </div>
    </div>
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
