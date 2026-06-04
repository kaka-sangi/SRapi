"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { apiService } from "@/lib/api";
import { meErrorMessage } from "@/lib/me-api";
import { USER_HOME_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { useCaptcha } from "@/components/auth/captcha";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";

/**
 * Self-service sign-up. Registration is gated server-side by
 * Security.RegistrationEnabled — when the admin hasn't enabled it the submit
 * surfaces the backend's "registration disabled" message inline.
 */
export default function RegisterPage() {
  const { t } = useLanguage();
  const router = useRouter();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const captcha = useCaptcha();

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!name.trim() || !email || password.length < 8) {
      setError(t("authRegister.invalid"));
      return;
    }
    setSubmitting(true);
    try {
      await apiService.register(email, name.trim(), password, captcha.token);
      router.replace(USER_HOME_ROUTE);
    } catch (err) {
      setError(meErrorMessage(err));
      setSubmitting(false);
    }
  }

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
          <Card className="card-raised p-7 sm:p-8">
            <h1 className="font-serif text-2xl text-srapi-text-primary">{t("authRegister.title")}</h1>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">{t("authRegister.subtitle")}</p>
            <form onSubmit={handleSubmit} noValidate className="mt-7 space-y-5">
              <div>
                <Label htmlFor="reg-name">{t("authRegister.name")}</Label>
                <Input
                  id="reg-name"
                  autoComplete="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="reg-email">{t("login.email")}</Label>
                <Input
                  id="reg-email"
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="you@example.com"
                />
              </div>
              <div>
                <Label htmlFor="reg-password">{t("login.password")}</Label>
                <div className="relative">
                  <Input
                    id="reg-password"
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
                <p className="mt-1 text-2xs text-srapi-text-tertiary">
                  {t("authRegister.passwordHint")}
                </p>
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
                {submitting ? t("authRegister.submitting") : t("authRegister.cta")}
              </Button>
            </form>
            <p className="mt-6 text-center text-sm text-srapi-text-secondary">
              {t("authRegister.haveAccount")}{" "}
              <Link href="/" className="text-srapi-primary underline-offset-4 hover:underline">
                {t("authRegister.signIn")}
              </Link>
            </p>
          </Card>
        </div>
      </main>
    </div>
  );
}
