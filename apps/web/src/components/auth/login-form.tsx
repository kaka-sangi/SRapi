"use client";

import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { apiService } from "@/lib/api";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";

export function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const { t } = useLanguage();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!email || !password) {
      setError(t("login.errRequired"));
      return;
    }
    setSubmitting(true);
    try {
      const user = await apiService.login(email, password);
      const from = params.get("from");
      const home = user.role === "admin" ? ADMIN_HOME_ROUTE : USER_HOME_ROUTE;
      router.replace(from && from.startsWith("/") ? from : home);
    } catch {
      setError(t("login.errWrong"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Card className="p-7 sm:p-8">
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
          <Label htmlFor="password">{t("login.password")}</Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        {error && (
          <p role="alert" className="text-sm text-srapi-error">
            {error}
          </p>
        )}
        <Button type="submit" variant="primary" size="lg" className="w-full" disabled={submitting}>
          {submitting ? t("login.signingIn") : t("login.signIn")}
        </Button>
      </form>
    </Card>
  );
}
