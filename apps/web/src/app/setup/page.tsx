"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { apiService } from "@/lib/api";
import { useLanguage } from "@/context/LanguageContext";
import { AmbientCanvas } from "@/components/visual/ambient-canvas";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export default function SetupPage() {
  const { t } = useLanguage();
  const router = useRouter();
  const [checking, setChecking] = useState(true);
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
      })
      .catch(() => {
        if (active) setChecking(false);
      });
    return () => {
      active = false;
    };
  }, [router]);

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
      <main className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center px-6 py-10">
        <div className="animate-bloom">
          <div className="mb-8 flex items-center gap-3 font-mono text-2xs uppercase tracking-[0.18em] text-srapi-text-tertiary">
            <span className="h-px w-7 bg-srapi-primary" />
            {t("setup.eyebrow")}
          </div>
          <h1 className="font-serif text-4xl font-medium leading-tight text-srapi-text-primary">{t("setup.title")}</h1>
          <p className="mt-4 text-md leading-relaxed text-srapi-text-secondary">{t("setup.subtitle")}</p>
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
            </div>
            {error && (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            )}
            <Button type="submit" variant="primary" loading={submitting} className="w-full">
              {t("setup.submit")}
            </Button>
          </form>
        </div>
      </main>
    </div>
  );
}
