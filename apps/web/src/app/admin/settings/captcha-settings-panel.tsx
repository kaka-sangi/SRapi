"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, CircleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { useAdminCaptchaSettings, useUpdateCaptchaSettings } from "@/hooks/admin-queries";
import { adminErrorMessage } from "@/lib/admin-api";
import type {
  CaptchaSettings,
  CaptchaSettingsWritable,
} from "../../../../../../packages/sdk/typescript/src/types.gen";

type CaptchaProvider = CaptchaSettings["provider"];

const PROVIDERS: Array<{ value: CaptchaProvider; label: string }> = [
  { value: "turnstile", label: "Cloudflare Turnstile" },
  { value: "hcaptcha", label: "hCaptcha" },
  { value: "recaptcha", label: "reCAPTCHA" },
];

interface CaptchaDraft {
  managed: boolean;
  enabled: boolean;
  provider: CaptchaProvider;
  siteKey: string;
  secretKey: string;
  secretKeyConfigured: boolean;
  verifyURL: string;
}

export function CaptchaSettingsPanel() {
  const { t } = useLanguage();
  const query = useAdminCaptchaSettings();

  return (
    <PageQueryState
      query={query}
      skeleton={
        <div className="border-t border-srapi-border pt-5 text-sm text-srapi-text-tertiary">
          {t("common.loading")}
        </div>
      }
    >
      {(settings) => <CaptchaSettingsEditor initial={settings} />}
    </PageQueryState>
  );
}

function CaptchaSettingsEditor({ initial }: { initial: CaptchaSettings }) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const updateMut = useUpdateCaptchaSettings();
  const initialSignature = JSON.stringify(initial);
  const initialSignatureRef = useRef(initialSignature);
  const [draft, setDraft] = useState<CaptchaDraft>(() => createCaptchaDraft(initial));
  const [savedDraft, setSavedDraft] = useState<CaptchaDraft>(() => createCaptchaDraft(initial));

  useEffect(() => {
    if (initialSignatureRef.current === initialSignature) return;
    initialSignatureRef.current = initialSignature;
    const next = createCaptchaDraft(initial);
    setDraft(next);
    setSavedDraft(next);
  }, [initial, initialSignature]);

  const isDirty = useMemo(
    () => JSON.stringify(draft) !== JSON.stringify(savedDraft),
    [draft, savedDraft],
  );

  function patch(next: Partial<CaptchaDraft>) {
    setDraft((current) => ({ ...current, ...next }));
  }

  async function save() {
    try {
      const saved = await updateMut.mutateAsync(materializeCaptchaDraft(draft));
      const normalized = createCaptchaDraft(saved);
      setDraft(normalized);
      setSavedDraft(normalized);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const effectiveSecretConfigured = draft.secretKey.trim() !== "" || draft.secretKeyConfigured;

  return (
    <section className="space-y-4 border-t border-srapi-border pt-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-srapi-text-primary">
            {t("adminSettings.captcha.title")}
          </h3>
          <p className="mt-1 max-w-2xl text-xs text-srapi-text-tertiary">
            {t("adminSettings.captcha.hint")}
          </p>
        </div>
        <CaptchaSaveState dirty={isDirty} pending={updateMut.isPending} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <ToggleRow
          id="captcha-managed"
          label={t("adminSettings.captcha.managed")}
          hint={t("adminSettings.captcha.managedHint")}
          checked={draft.managed}
          onChange={(checked) => patch({ managed: checked })}
        />
        <ToggleRow
          id="captcha-enabled"
          label={t("adminSettings.captcha.enabled")}
          hint={t("adminSettings.captcha.enabledHint")}
          checked={draft.enabled}
          disabled={!draft.managed}
          onChange={(checked) => patch({ enabled: checked })}
        />
        <div>
          <Label>{t("adminSettings.captcha.provider")}</Label>
          <Select
            value={draft.provider}
            disabled={!draft.managed}
            onValueChange={(value) => patch({ provider: value as CaptchaProvider })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PROVIDERS.map((provider) => (
                <SelectItem key={provider.value} value={provider.value}>
                  {provider.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="captcha-site-key">{t("adminSettings.captcha.siteKey")}</Label>
          <Input
            id="captcha-site-key"
            className="font-mono text-xs"
            value={draft.siteKey}
            disabled={!draft.managed}
            onChange={(event) => patch({ siteKey: event.target.value })}
          />
        </div>
        <div>
          <Label htmlFor="captcha-secret-key">{t("adminSettings.captcha.secretKey")}</Label>
          <Input
            id="captcha-secret-key"
            type="password"
            autoComplete="off"
            className="font-mono text-xs"
            value={draft.secretKey}
            disabled={!draft.managed}
            placeholder={
              draft.secretKeyConfigured
                ? t("adminSettings.captcha.secretConfigured")
                : t("adminSettings.captcha.secretPlaceholder")
            }
            onChange={(event) => patch({ secretKey: event.target.value })}
          />
          <p className="mt-1 text-2xs text-srapi-text-tertiary">
            {t("adminSettings.captcha.secretHint")}
          </p>
        </div>
        <div>
          <Label htmlFor="captcha-verify-url">{t("adminSettings.captcha.verifyURL")}</Label>
          <Input
            id="captcha-verify-url"
            className="font-mono text-xs"
            value={draft.verifyURL}
            disabled={!draft.managed}
            placeholder="https://challenges.cloudflare.com/turnstile/v0/siteverify"
            onChange={(event) => patch({ verifyURL: event.target.value })}
          />
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-srapi-border pt-4">
        <div className="font-mono text-2xs text-srapi-text-tertiary">
          {effectiveSecretConfigured
            ? t("adminSettings.captcha.secretStatusConfigured")
            : t("adminSettings.captcha.secretStatusMissing")}
        </div>
        <Button
          type="button"
          variant="outline"
          loading={updateMut.isPending}
          disabled={!isDirty}
          onClick={() => void save()}
        >
          {t("adminSettings.captcha.save")}
        </Button>
      </div>
    </section>
  );
}

function ToggleRow({
  id,
  label,
  hint,
  checked,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  hint: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex min-h-16 cursor-pointer items-start gap-3 rounded-md border border-srapi-border bg-srapi-card/40 px-3 py-3 text-sm has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60">
      <Checkbox
        id={id}
        aria-label={label}
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span>
        <span className="block font-medium text-srapi-text-primary">{label}</span>
        <span className="mt-1 block text-xs text-srapi-text-tertiary">{hint}</span>
      </span>
    </label>
  );
}

function CaptchaSaveState({ dirty, pending }: { dirty: boolean; pending: boolean }) {
  const { t } = useLanguage();
  const icon = dirty ? (
    <CircleAlert className="size-3.5 text-srapi-warning" aria-hidden />
  ) : (
    <CheckCircle2 className="size-3.5 text-srapi-success" aria-hidden />
  );
  const label = pending
    ? t("adminSettings.captcha.saving")
    : dirty
      ? t("adminSettings.captcha.unsaved")
      : t("adminSettings.captcha.saved");

  return (
    <span className="inline-flex items-center gap-1.5 rounded-md border border-srapi-border px-2 py-1 font-mono text-2xs text-srapi-text-tertiary">
      {pending ? null : icon}
      {label}
    </span>
  );
}

function createCaptchaDraft(settings: CaptchaSettings): CaptchaDraft {
  return {
    managed: settings.managed,
    enabled: settings.enabled,
    provider: settings.provider,
    siteKey: settings.site_key,
    secretKey: "",
    secretKeyConfigured: settings.secret_key_configured,
    verifyURL: settings.verify_url,
  };
}

function materializeCaptchaDraft(draft: CaptchaDraft): CaptchaSettingsWritable {
  const body: CaptchaSettingsWritable = {
    managed: draft.managed,
    enabled: draft.enabled,
    provider: draft.provider,
    site_key: draft.siteKey.trim(),
    verify_url: draft.verifyURL.trim(),
  };
  const secretKey = draft.secretKey.trim();
  if (secretKey) {
    body.secret_key = secretKey;
  }
  return body;
}
