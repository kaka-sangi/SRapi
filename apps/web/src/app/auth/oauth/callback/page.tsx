"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { apiService } from "@/lib/api";
import type { OAuthPendingSession } from "@/lib/sdk-types";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE, SIGN_IN_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";

type Phase = "loading" | "bind" | "twofactor" | "create" | "email" | "error";

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
      setError(t("oauthCallback.expired"));
      setPhase("error");
    }
  }, [t]);

  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void inspect();
  }, [inspect]);

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
            <h2 className="font-serif text-2xl text-srapi-text-primary">
              {t("oauthCallback.bindTitle")}
            </h2>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("oauthCallback.bindHint")}</p>
            <form onSubmit={handleBind} noValidate className="mt-7 space-y-5">
              <div>
                <Label htmlFor="email">{t("oauthCallback.emailLabel")}</Label>
                <Input
                  id="email"
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="password">{t("oauthCallback.password")}</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  autoFocus
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
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
                disabled={busy || !email.trim() || !password}
              >
                {busy ? t("login.signingIn") : t("oauthCallback.linkAndSignIn")}
              </Button>
            </form>
          </Card>
        )}

        {phase === "twofactor" && (
          <Card className="p-7 sm:p-8">
            <h2 className="font-serif text-2xl text-srapi-text-primary">
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
                disabled={busy || code.length < 6}
              >
                {busy ? t("login.verifying") : t("login.verify")}
              </Button>
            </form>
          </Card>
        )}

        {phase === "create" && (
          <Card className="p-7 sm:p-8">
            <h2 className="font-serif text-2xl text-srapi-text-primary">
              {t("oauthCallback.createTitle")}
            </h2>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">
              {t("oauthCallback.createHint")}
            </p>
            <form onSubmit={handleCreate} noValidate className="mt-7 space-y-5">
              <div>
                <Label htmlFor="email">{t("oauthCallback.emailLabel")}</Label>
                <Input
                  id="email"
                  type="email"
                  value={pending?.profile.resolved_email || email}
                  readOnly
                  disabled
                />
              </div>
              <div>
                <Label htmlFor="name">{t("oauthCallback.displayName")}</Label>
                <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div>
                <Label htmlFor="password">{t("oauthCallback.password")}</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
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
                disabled={busy || !password}
              >
                {busy ? t("common.loading") : t("oauthCallback.createAccount")}
              </Button>
            </form>
          </Card>
        )}

        {phase === "email" && (
          <Card className="p-7 sm:p-8">
            <h2 className="font-serif text-2xl text-srapi-text-primary">
              {t("oauthCallback.emailTitle")}
            </h2>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">
              {t("oauthCallback.emailHint")}
            </p>
            {!codeSent ? (
              <form onSubmit={handleSendCode} noValidate className="mt-7 space-y-5">
                <div>
                  <Label htmlFor="email">{t("oauthCallback.emailLabel")}</Label>
                  <Input
                    id="email"
                    type="email"
                    autoComplete="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="you@example.com"
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
                  disabled={busy || !email.trim()}
                >
                  {busy ? t("oauthCallback.sending") : t("oauthCallback.sendCode")}
                </Button>
              </form>
            ) : (
              <form onSubmit={handleConfirmEmail} noValidate className="mt-7 space-y-5">
                <div>
                  <Label htmlFor="token">{t("oauthCallback.tokenLabel")}</Label>
                  <Input
                    id="token"
                    autoFocus
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                  />
                  <p className="mt-1.5 text-2xs text-srapi-text-tertiary">
                    {t("oauthCallback.tokenHint", { email: email.trim() })}
                  </p>
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
                  disabled={busy || !token.trim()}
                >
                  {busy ? t("common.loading") : t("oauthCallback.confirm")}
                </Button>
              </form>
            )}
          </Card>
        )}

        {phase === "error" && (
          <Card className="p-7 text-center sm:p-8">
            <h2 className="font-serif text-xl text-srapi-text-primary">
              {t("oauthCallback.errorTitle")}
            </h2>
            <p className="mt-2 text-sm text-srapi-text-secondary">{error}</p>
            <Button
              variant="outline"
              size="lg"
              className="mt-6 w-full"
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
