"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { CurrentUserAttribute, apiService } from "@/lib/api";
import { meErrorMessage } from "@/lib/me-api";
import { USER_HOME_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { Card } from "@/components/ui/card";
import { FloatingInput } from "@/components/ui/floating-input";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { useCaptcha } from "@/components/auth/captcha";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";
import type { SiteConfig } from "@/lib/api/types";

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

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
  const [emailTouched, setEmailTouched] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [site, setSite] = useState<SiteConfig | null>(null);
  const [attributes, setAttributes] = useState<CurrentUserAttribute[]>([]);
  const [attributeValues, setAttributeValues] = useState<Record<number, string>>({});
  const captcha = useCaptcha();

  useEffect(() => {
    let active = true;
    Promise.allSettled([
      apiService.getSiteConfig(),
      apiService.listRegistrationAttributes(),
    ]).then(([siteResult, attrResult]) => {
      if (!active) return;
      setSite(siteResult.status === "fulfilled" ? siteResult.value : null);
      const nextAttributes = attrResult.status === "fulfilled" ? attrResult.value : [];
      setAttributes(nextAttributes);
      setAttributeValues(Object.fromEntries(nextAttributes.map((item) => [item.definition_id, ""])));
    });
    return () => {
      active = false;
    };
  }, []);

  const emailLooksValid = EMAIL_RE.test(email.trim());
  const emailError = emailTouched && email.length > 0 && !emailLooksValid
    ? t("authRegister.invalid")
    : undefined;
  const passwordLongEnough = password.length >= 8;
  const passwordError = password.length > 0 && !passwordLongEnough
    ? t("authRegister.passwordHint")
    : undefined;

  const requiredAttrsFilled = attributes.every(
    (item) => !item.required || (attributeValues[item.definition_id] || "").trim().length > 0,
  );

  const formValid = name.trim().length > 0 && emailLooksValid && passwordLongEnough && requiredAttrsFilled && !(captcha.required && !captcha.token);
  const formDirty = name.length > 0 || email.length > 0 || password.length > 0;

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!name.trim() || !email || password.length < 8) {
      setError(t("authRegister.invalid"));
      return;
    }
    const values = attributes.map((item) => ({
      definition_id: item.definition_id,
      value: attributeValues[item.definition_id] || "",
    }));
    if (attributes.some((item) => item.required && !attributeValues[item.definition_id]?.trim())) {
      setError(t("authRegister.invalid"));
      return;
    }
    setSubmitting(true);
    try {
      // Affiliate referral: pass ?invite_code= from the share link so the
      // referral/rebate is recorded on sign-up (read at submit time — runs on
      // the client). The backend RegisterRequest applies it.
      const inviteCode = new URLSearchParams(window.location.search).get("invite_code") || "";
      await apiService.register(email, name.trim(), password, captcha.token, values, inviteCode || undefined);
      router.replace(USER_HOME_ROUTE);
    } catch (err) {
      setError(meErrorMessage(err));
      setSubmitting(false);
    }
  }

  return (
    <div className="relative flex min-h-dvh flex-col">
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <Link href="/" className="text-2xl font-semibold tracking-tight leading-none text-srapi-text-primary">
          {site?.site_name || "SRapi"}
        </Link>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>
      <main className="mx-auto flex w-full max-w-6xl flex-1 items-center justify-center px-6 py-10">
        <div className="w-full max-w-sm">
          <Card className="p-7 sm:p-8">
            <h1 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">{t("authRegister.title")}</h1>
            <p className="mt-1.5 text-sm text-srapi-text-secondary">
              {site?.site_subtitle?.trim() || t("authRegister.subtitle")}
            </p>
            <form onSubmit={handleSubmit} noValidate className="mt-7 space-y-5">
              <FloatingInput
                id="reg-name"
                label={t("authRegister.name")}
                autoComplete="name"
                value={name}
                onChange={setName}
              />
              <div onBlur={() => setEmailTouched(true)}>
                <FloatingInput
                  id="reg-email"
                  label={t("login.email")}
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={setEmail}
                  error={emailError}
                />
              </div>
              <div>
                <div className="relative">
                  <FloatingInput
                    id="reg-password"
                    label={t("login.password")}
                    type={showPassword ? "text" : "password"}
                    autoComplete="new-password"
                    value={password}
                    onChange={setPassword}
                    error={passwordError}
                    hint={password.length === 0 ? t("authRegister.passwordHint") : undefined}
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
              {attributes.map((item) => (
                <div key={item.definition_id}>
                  <Label htmlFor={`reg-attr-${item.definition_id}`}>
                    {item.name}{item.required ? " *" : ""}
                  </Label>
                  {item.data_type === "boolean" ? (
                    <select
                      id={`reg-attr-${item.definition_id}`}
                      className="mt-1.5 h-10 w-full rounded-xl border border-srapi-border bg-srapi-card px-3 text-sm"
                      value={attributeValues[item.definition_id] || ""}
                      onChange={(event) => setAttributeValues((prev) => ({ ...prev, [item.definition_id]: event.target.value }))}
                    >
                      <option value="">{t("account.attrUnset")}</option>
                      <option value="true">{t("account.attrTrue")}</option>
                      <option value="false">{t("account.attrFalse")}</option>
                    </select>
                  ) : item.data_type === "select" ? (
                    <select
                      id={`reg-attr-${item.definition_id}`}
                      className="mt-1.5 h-10 w-full rounded-xl border border-srapi-border bg-srapi-card px-3 text-sm"
                      value={attributeValues[item.definition_id] || ""}
                      onChange={(event) => setAttributeValues((prev) => ({ ...prev, [item.definition_id]: event.target.value }))}
                    >
                      <option value="">{t("account.attrUnset")}</option>
                      {(item.options || []).map((option) => (
                        <option key={option} value={option}>{option}</option>
                      ))}
                    </select>
                  ) : (
                    <Input
                      id={`reg-attr-${item.definition_id}`}
                      type={item.data_type === "number" ? "number" : "text"}
                      value={attributeValues[item.definition_id] || ""}
                      onChange={(event) => setAttributeValues((prev) => ({ ...prev, [item.definition_id]: event.target.value }))}
                    />
                  )}
                </div>
              ))}
              {error && (
                <p role="alert" className="anim-shake rounded-xl bg-srapi-error/10 px-3 py-2 text-sm text-srapi-error">
                  {error}
                </p>
              )}
              {captcha.node}
              <Button
                type="submit"
                variant="primary"
                size="lg"
                className="h-11 w-full rounded-xl"
                loading={submitting}
                disabled={submitting || (captcha.required && !captcha.token)}
              >
                <span className="inline-flex items-center gap-2">
                  {submitting ? t("authRegister.submitting") : t("authRegister.cta")}
                  {formValid && formDirty && !submitting ? (
                    <Kbd className="border-white/20 bg-white/10 text-white/90 shadow-none">↵</Kbd>
                  ) : null}
                </span>
              </Button>
            </form>
            {(site?.user_agreement || site?.privacy_policy) ? (
              <div className="mt-4 flex justify-center gap-3 text-xs text-srapi-text-tertiary">
                {site.user_agreement ? (
                  <a href={site.user_agreement} className="underline-offset-4 hover:underline">
                    {t("authRegister.userAgreement")}
                  </a>
                ) : null}
                {site.privacy_policy ? (
                  <a href={site.privacy_policy} className="underline-offset-4 hover:underline">
                    {t("authRegister.privacyPolicy")}
                  </a>
                ) : null}
              </div>
            ) : null}
            <p className="mt-6 text-center text-sm text-srapi-text-secondary">
              {t("authRegister.haveAccount")}{" "}
              <Link href="/" className="text-srapi-primary underline-offset-4 hover:underline">
                {t("authRegister.signIn")}
              </Link>
            </p>
            {(site?.doc_url || site?.contact_info) ? (
              <div className="mt-4 flex justify-center gap-3 text-xs text-srapi-text-tertiary">
                {site.doc_url ? (
                  <a href={site.doc_url} className="underline-offset-4 hover:underline">
                    {t("login.docsLink")}
                  </a>
                ) : null}
                {site.contact_info ? <span>{site.contact_info}</span> : null}
              </div>
            ) : null}
          </Card>
        </div>
      </main>
    </div>
  );
}

/**
 * 4-segment strength bar mirroring the one in /setup. Lives co-located so each
 * auth form can render it without a cross-feature import; the scoring is light
 * (length + character-class buckets) and intentionally identical so the user
 * sees the same signal across sign-up, set-password, and admin setup.
 */
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
