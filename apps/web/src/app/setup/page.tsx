"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { apiService } from "@/lib/api";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { AmbientCanvas } from "@/components/visual/ambient-canvas";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type HealthStatus = "checking" | "ok" | "error";

function StatusDot({ status }: { status: HealthStatus }) {
  const color =
    status === "ok"
      ? "bg-emerald-500"
      : status === "error"
        ? "bg-red-400"
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
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
        <p className="font-mono text-2xs text-srapi-text-tertiary">{t("setup.checking")}</p>
      </div>
    );
  }

  return (
    <div className="relative flex min-h-dvh flex-col">
      <AmbientCanvas />
      <main className="mx-auto flex w-full max-w-lg flex-1 flex-col justify-center px-6 py-10">
        <div className="animate-bloom">
          <div className="mb-8 flex items-center gap-3 font-mono text-2xs uppercase tracking-[0.18em] text-srapi-text-tertiary">
            <span className="h-px w-7 bg-srapi-primary" />
            {t("setup.eyebrow")}
          </div>

          {/* Step indicators */}
          <div className="mb-6 flex items-center gap-2 font-mono text-2xs text-srapi-text-tertiary">
            <span className={step === "health" ? "text-srapi-primary" : "text-srapi-text-tertiary"}>
              {t("setup.stepHealth")}
            </span>
            <span className="h-px w-4 bg-srapi-border" />
            <span className={step === "account" ? "text-srapi-primary" : "text-srapi-text-tertiary"}>
              {t("setup.stepAccount")}
            </span>
          </div>

          {step === "health" && (
            <div>
              <h1 className="font-serif text-4xl font-medium leading-tight text-srapi-text-primary">
                {t("setup.healthTitle")}
              </h1>
              <p className="mt-4 text-md leading-relaxed text-srapi-text-secondary">
                {t("setup.healthSubtitle")}
              </p>

              <div className="mt-8 space-y-3 rounded-lg border border-srapi-border p-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-srapi-text-secondary">{t("setup.apiServer")}</span>
                  <div className="flex items-center gap-2">
                    <StatusDot status={apiHealth} />
                    <span className="font-mono text-2xs text-srapi-text-tertiary">
                      {apiHealth === "ok" ? t("setup.connected") : apiHealth === "error" ? t("setup.unreachable") : t("setup.checking")}
                    </span>
                  </div>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-srapi-text-secondary">{t("setup.database")}</span>
                  <div className="flex items-center gap-2">
                    <StatusDot status={dbHealth} />
                    <span className="font-mono text-2xs text-srapi-text-tertiary">
                      {dbHealth === "ok" ? t("setup.connected") : dbHealth === "error" ? t("setup.unreachable") : t("setup.checking")}
                    </span>
                  </div>
                </div>
              </div>

              <div className="mt-6 flex gap-3">
                <Button variant="ghost" onClick={() => void checkHealth()} className="flex-1">
                  {t("setup.recheckHealth")}
                </Button>
                <Button
                  variant="primary"
                  onClick={() => setStep("account")}
                  disabled={apiHealth !== "ok"}
                  className="flex-1"
                >
                  {t("setup.continueToAccount")}
                </Button>
              </div>
            </div>
          )}

          {step === "account" && (
            <div>
              <h1 className="font-serif text-4xl font-medium leading-tight text-srapi-text-primary">
                {t("setup.title")}
              </h1>
              <p className="mt-4 text-md leading-relaxed text-srapi-text-secondary">
                {t("setup.subtitle")}
              </p>
              <form onSubmit={onSubmit} className="mt-8 space-y-4">
                <div>
                  <Label htmlFor="setup-name">{t("setup.name")}</Label>
                  <Input id="setup-name" value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" required />
                </div>
                <div>
                  <Label htmlFor="setup-email">{t("setup.email")}</Label>
                  <Input id="setup-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="username" required />
                </div>
                <div>
                  <Label htmlFor="setup-password">{t("setup.password")}</Label>
                  <Input
                    id="setup-password"
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoComplete="new-password"
                    required
                  />
                  <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("setup.passwordHint")}</p>
                  {password.length > 0 && <PasswordStrength password={password} />}
                </div>
                {error && (
                  <p role="alert" className="text-sm text-srapi-error">
                    {error}
                  </p>
                )}
                <div className="flex gap-3">
                  <Button type="button" variant="ghost" onClick={() => setStep("health")} className="flex-1">
                    {t("setup.back")}
                  </Button>
                  <Button type="submit" variant="primary" loading={submitting} className="flex-1">
                    {t("setup.submit")}
                  </Button>
                </div>
              </form>
            </div>
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
