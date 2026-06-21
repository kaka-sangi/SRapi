"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { CurrentUserAttribute, apiService } from "@/lib/api";
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
import type { SiteConfig } from "@/lib/api/types";

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
        <Link href="/" className="font-serif text-2xl leading-none text-srapi-text-primary">
          {site?.site_name || "SRapi"}
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
            <p className="mt-1.5 text-sm text-srapi-text-secondary">
              {site?.site_subtitle?.trim() || t("authRegister.subtitle")}
            </p>
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
              {attributes.map((item) => (
                <div key={item.definition_id}>
                  <Label htmlFor={`reg-attr-${item.definition_id}`}>
                    {item.name}{item.required ? " *" : ""}
                  </Label>
                  {item.data_type === "boolean" ? (
                    <select
                      id={`reg-attr-${item.definition_id}`}
                      className="mt-1 h-10 w-full rounded-lg border border-srapi-border bg-srapi-card px-3 text-sm"
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
                      className="mt-1 h-10 w-full rounded-lg border border-srapi-border bg-srapi-card px-3 text-sm"
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
                loading={submitting}
                disabled={submitting || (captcha.required && !captcha.token)}
              >
                {submitting ? t("authRegister.submitting") : t("authRegister.cta")}
              </Button>
            </form>
            {(site?.user_agreement || site?.privacy_policy) ? (
              <div className="mt-4 flex justify-center gap-3 text-2xs text-srapi-text-tertiary">
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
              <div className="mt-4 flex justify-center gap-3 text-2xs text-srapi-text-tertiary">
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
