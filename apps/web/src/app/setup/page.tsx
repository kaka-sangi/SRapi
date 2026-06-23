"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { apiService } from "@/lib/api";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { AmbientCanvas } from "@/components/visual/ambient-canvas";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { FloatingInput } from "@/components/ui/floating-input";
import { DataPill } from "@/components/ui/data-pill";
import { Kbd } from "@/components/ui/kbd";

type HealthStatus = "checking" | "ok" | "error";

function StatusDot({ status }: { status: HealthStatus }) {
  const color =
    status === "ok"
      ? "bg-emerald-500"
      : status === "error"
        ? "bg-srapi-error"
        : "bg-srapi-text-tertiary animate-pulse";
  return <span className={`inline-block h-2 w-2 rounded-full ${color}`} />;
}

export default function SetupPage() {
  const { t } = useLanguage();
  const router = useRouter();
  const [checking, setChecking] = useState(true);
  const [step, setStep] = useState<"health" | "account">("health");
  const [apiHealth, setApiHealth] = useState<HealthStatus>("checking");
  const [dbHealth, setDbHealth] = useState<HealthStatus>("checking");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [emailTouched, setEmailTouched] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const emailLooksValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
  const emailInlineError = emailTouched && email.length > 0 && !emailLooksValid
    ? t("setup.email")
    : undefined;
  const passwordStrongEnough = password.length >= 8;
  const passwordInlineError = password.length > 0 && !passwordStrongEnough
    ? t("setup.passwordHint")
    : undefined;
  const formValid = name.trim().length > 0 && emailLooksValid && passwordStrongEnough;
  const formDirty = name.length > 0 || email.length > 0 || password.length > 0;

  const checkHealth = useCallback(async () => {
    setApiHealth("checking");
    setDbHealth("checking");
    try {
      // Read the real per-dependency status so "Database" reflects the actual DB
      // probe rather than mirroring the API check (which would mask a DB outage).
      const res = await fetch("/api/v1/health", { headers: { accept: "application/json" } });
      if (!res.ok) {
        setApiHealth("error");
        setDbHealth("error");
        return;
      }
      const body = (await res.json()) as { data?: { dependencies?: { database?: string } } };
      setApiHealth("ok");
      setDbHealth(body.data?.dependencies?.database === "ok" ? "ok" : "error");
    } catch {
      setApiHealth("error");
      setDbHealth("error");
    }
  }, []);

  useEffect(() => {
    let active = true;
    apiService
      .getSetupStatus()
      .then((needsSetup) => {
        if (!active) return;
        if (!needsSetup) {
          router.replace("/");
          return;
        }
        setChecking(false);
        void checkHealth();
      })
      .catch(() => {
        if (active) setChecking(false);
      });
    return () => {
      active = false;
    };
  }, [router, checkHealth]);

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (password.length < 8) {
      setError(t("setup.passwordHint"));
      return;
    }
    setSubmitting(true);
    try {
      await apiService.completeSetup({ email, name, password });
      router.replace("/?setup=done");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to complete setup.");
    } finally {
      setSubmitting(false);
    }
  }

  if (checking) {
    return (
      <div className="relative flex min-h-dvh items-center justify-center">
        <AmbientCanvas />
        <p className="text-xs font-medium uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("setup.checking")}</p>
      </div>
    );
  }

  const stepTone = (active: boolean): "accent" | "neutral" => (active ? "accent" : "neutral");

  return (
    <div className="relative flex min-h-dvh flex-col">
      <AmbientCanvas />
      <main className="mx-auto flex w-full max-w-lg flex-1 flex-col justify-center px-6 py-10">
        <div className="space-y-6">
          <div className="space-y-3">
            <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("setup.eyebrow")}
            </p>
            <div className="flex items-center gap-2">
              <DataPill tone={stepTone(step === "health")}>1 · {t("setup.stepHealth")}</DataPill>
              <span className="h-px w-4 bg-srapi-border" />
              <DataPill tone={stepTone(step === "account")}>2 · {t("setup.stepAccount")}</DataPill>
            </div>
          </div>

          {step === "health" && (
            <Card className="p-7 sm:p-8">
              <h1 className="text-3xl font-semibold tracking-tight leading-tight text-srapi-text-primary">
                {t("setup.healthTitle")}
              </h1>
              <p className="mt-3 text-sm leading-relaxed text-srapi-text-secondary">
                {t("setup.healthSubtitle")}
              </p>

              <div className="mt-7 divide-y divide-srapi-border/70 rounded-xl border border-srapi-border bg-srapi-card-muted/40">
                <div className="flex items-center justify-between gap-3 px-4 py-3">
                  <span className="text-sm font-medium text-srapi-text-primary">{t("setup.apiServer")}</span>
                  <div className="flex items-center gap-2">
                    <StatusDot status={apiHealth} />
                    <span className="text-xs font-medium text-srapi-text-secondary">
                      {apiHealth === "ok" ? t("setup.connected") : apiHealth === "error" ? t("setup.unreachable") : t("setup.checking")}
                    </span>
                  </div>
                </div>
                <div className="flex items-center justify-between gap-3 px-4 py-3">
                  <span className="text-sm font-medium text-srapi-text-primary">{t("setup.database")}</span>
                  <div className="flex items-center gap-2">
                    <StatusDot status={dbHealth} />
                    <span className="text-xs font-medium text-srapi-text-secondary">
                      {dbHealth === "ok" ? t("setup.connected") : dbHealth === "error" ? t("setup.unreachable") : t("setup.checking")}
                    </span>
                  </div>
                </div>
              </div>

              <div className="mt-6 flex gap-3">
                <Button variant="ghost" onClick={() => void checkHealth()} className="h-11 flex-1 rounded-xl">
                  {t("setup.recheckHealth")}
                </Button>
                <Button
                  variant="primary"
                  onClick={() => setStep("account")}
                  disabled={apiHealth !== "ok"}
                  className="h-11 flex-1 rounded-xl"
                >
                  {t("setup.continueToAccount")}
                </Button>
              </div>
            </Card>
          )}

          {step === "account" && (
            <Card className="p-7 sm:p-8">
              <h1 className="text-3xl font-semibold tracking-tight leading-tight text-srapi-text-primary">
                {t("setup.title")}
              </h1>
              <p className="mt-3 text-sm leading-relaxed text-srapi-text-secondary">
                {t("setup.subtitle")}
              </p>
              <form onSubmit={onSubmit} className="mt-7 space-y-5">
                <FloatingInput
                  id="setup-name"
                  label={t("setup.name")}
                  autoComplete="name"
                  value={name}
                  onChange={setName}
                  required
                />
                <div onBlur={() => setEmailTouched(true)}>
                  <FloatingInput
                    id="setup-email"
                    label={t("setup.email")}
                    type="email"
                    autoComplete="username"
                    value={email}
                    onChange={setEmail}
                    error={emailInlineError}
                    required
                  />
                </div>
                <div>
                  <FloatingInput
                    id="setup-password"
                    label={t("setup.password")}
                    type="password"
                    autoComplete="new-password"
                    value={password}
                    onChange={setPassword}
                    hint={password.length === 0 ? t("setup.passwordHint") : undefined}
                    error={passwordInlineError}
                    required
                  />
                  {password.length > 0 && <PasswordStrength password={password} />}
                </div>
                {error && (
                  <p role="alert" className="anim-shake rounded-xl bg-srapi-error/10 px-3 py-2 text-sm text-srapi-error">
                    {error}
                  </p>
                )}
                <div className="flex gap-3 pt-1">
                  <Button type="button" variant="ghost" onClick={() => setStep("health")} className="h-11 flex-1 rounded-xl">
                    {t("setup.back")}
                  </Button>
                  <Button type="submit" variant="primary" loading={submitting} disabled={submitting || !formValid} className="h-11 flex-1 rounded-xl">
                    <span className="inline-flex items-center gap-2">
                      {t("setup.submit")}
                      {formValid && formDirty && !submitting ? (
                        <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
                      ) : null}
                    </span>
                  </Button>
                </div>
              </form>
            </Card>
          )}
        </div>
      </main>
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
    <div className="mt-2 flex gap-1">
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
