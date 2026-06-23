import { ShieldAlert, KeyRound, Users, Globe } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { type SettingsTab, type AdminSettingsDraft } from "@/lib/admin-settings-form";
import { SPECIAL_FIELDS } from "./settings-fields";
import { SpecialFieldRow } from "./special-field-row";
import { CaptchaSettingsPanel } from "./captcha-settings-panel";

interface SecurityTabProps {
  value: Record<string, unknown>;
  draft: AdminSettingsDraft;
  onField: (key: string, v: unknown) => void;
  onSpecial: (key: keyof AdminSettingsDraft, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
  modelOptions: MultiSelectOption[];
}

export function SecurityTab({ value, draft, onField, onSpecial, onSave, pending, modelOptions }: SecurityTabProps) {
  const { t } = useLanguage();
  const registrationEnabled = value.registration_enabled === true;
  const oauthEnabled = value.oauth_enabled === true;

  return (
    <div className="space-y-6">
      {/* Admin API Key */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <KeyRound className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">
                {t("adminSettings.security.adminApiKey")}
              </h3>
              <p className="text-xs text-srapi-text-tertiary">
                {t("adminSettings.security.adminApiKeyHint")}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2 rounded-lg border border-srapi-warning/30 bg-srapi-warning/5 px-3 py-2 text-xs text-srapi-text-secondary">
            <ShieldAlert className="size-4 shrink-0 text-srapi-warning" />
            <span>{t("adminSettings.security.adminApiKeyWarning")}</span>
          </div>
          <div className="text-sm text-srapi-text-secondary">
            {(value.admin_api_key as { configured?: boolean } | undefined)?.configured
              ? t("adminSettings.security.adminApiKeyConfigured")
              : t("adminSettings.security.adminApiKeyNotConfigured")}
          </div>
          <p className="text-xs text-srapi-text-tertiary">
            {t("adminSettings.security.adminApiKeyEnvHint")}
          </p>
        </CardContent>
      </Card>

      {/* Registration */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Users className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">
                {t("adminSettings.security.registration")}
              </h3>
              <p className="text-xs text-srapi-text-tertiary">
                {t("adminSettings.security.registrationHint")}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="sec-reg" className="mb-0">
              {t("adminSettings.fields.registration_enabled")}
            </Label>
            <Switch
              id="sec-reg"
              checked={registrationEnabled}
              onCheckedChange={(v) => onField("registration_enabled", v)}
            />
          </div>

          {registrationEnabled && (
            <div className="space-y-3 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
              {(SPECIAL_FIELDS.security ?? [])
                .filter((f) => f.skip === "registration_email_suffix_allowlist")
                .map((field) => (
                  <SpecialFieldRow
                    key={String(field.key)}
                    field={field}
                    draft={draft}
                    onChange={onSpecial}
                    modelOptions={modelOptions}
                  />
                ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* OAuth / SSO */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Globe className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">
                {t("adminSettings.security.oauth")}
              </h3>
              <p className="text-xs text-srapi-text-tertiary">
                {t("adminSettings.security.oauthHint")}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="sec-oauth" className="mb-0">
              {t("adminSettings.fields.oauth_enabled")}
            </Label>
            <Switch
              id="sec-oauth"
              checked={oauthEnabled}
              onCheckedChange={(v) => onField("oauth_enabled", v)}
            />
          </div>

          {oauthEnabled && (
            <div className="space-y-4 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
              {(SPECIAL_FIELDS.security ?? [])
                .filter((f) => f.skip === "oauth_providers" || f.skip === "oauth_provider_configs")
                .map((field) => (
                  <SpecialFieldRow
                    key={String(field.key)}
                    field={field}
                    draft={draft}
                    onChange={onSpecial}
                    modelOptions={modelOptions}
                  />
                ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* CAPTCHA */}
      <CaptchaSettingsPanel />

      {/* Save */}
      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>
          {t("adminSettings.saveSection")}
        </Button>
      </div>
    </div>
  );
}
