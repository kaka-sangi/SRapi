import { Mail, Bell, Server } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { type MultiSelectOption } from "@/components/ui/multi-select";
import { type AdminSettingsDraft } from "@/lib/admin-settings-form";
import { SPECIAL_FIELDS } from "./settings-fields";
import { SpecialFieldRow } from "./special-field-row";
import { EmailTestPanel } from "./email-test-panel";

interface Props {
  value: Record<string, unknown>;
  draft: AdminSettingsDraft;
  onField: (key: string, v: unknown) => void;
  onSpecial: (key: keyof AdminSettingsDraft, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
  modelOptions: MultiSelectOption[];
}

export function EmailTab({ value, draft, onField, onSpecial, onSave, pending, modelOptions }: Props) {
  const { t } = useLanguage();
  const str = (key: string) => (value[key] == null ? "" : String(value[key]));
  const num = (key: string) => (typeof value[key] === "number" ? value[key] as number : 0);
  const smtpConfigured = value.smtp_configured === true;

  return (
    <div className="space-y-6">
      {/* SMTP Configuration */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Server className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.email.smtp")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.email.smtpHint")}</p>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="em-host">{t("adminSettings.fields.smtp_host")}</Label>
              <Input id="em-host" value={str("smtp_host")} placeholder="smtp.example.com"
                onChange={(e) => onField("smtp_host", e.target.value)} />
            </div>
            <div>
              <Label htmlFor="em-port">{t("adminSettings.fields.smtp_port")}</Label>
              <Input id="em-port" type="number" min={0} value={String(num("smtp_port"))}
                onChange={(e) => onField("smtp_port", e.target.value === "" ? 0 : Number(e.target.value))} />
            </div>
            <div>
              <Label htmlFor="em-user">{t("adminSettings.fields.smtp_username")}</Label>
              <Input id="em-user" value={str("smtp_username")} onChange={(e) => onField("smtp_username", e.target.value)} />
            </div>
            <div className="flex items-center gap-2 self-end">
              <Switch id="em-tls" checked={value.smtp_use_tls === true}
                onCheckedChange={(v) => onField("smtp_use_tls", v)} />
              <Label htmlFor="em-tls" className="mb-0">{t("adminSettings.fields.smtp_use_tls")}</Label>
            </div>
            <div>
              <Label htmlFor="em-from">{t("adminSettings.fields.smtp_from")}</Label>
              <Input id="em-from" value={str("smtp_from")} placeholder="noreply@example.com"
                onChange={(e) => onField("smtp_from", e.target.value)} />
            </div>
            <div>
              <Label htmlFor="em-fromname">{t("adminSettings.fields.smtp_from_name")}</Label>
              <Input id="em-fromname" value={str("smtp_from_name")} placeholder="SRapi"
                onChange={(e) => onField("smtp_from_name", e.target.value)} />
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="em-baseurl">{t("adminSettings.fields.public_base_url")}</Label>
              <p className="mb-1 text-xs text-srapi-text-tertiary">{t("adminSettings.email.baseUrlHint")}</p>
              <Input id="em-baseurl" value={str("public_base_url")} placeholder="https://app.example.com"
                onChange={(e) => onField("public_base_url", e.target.value)} />
            </div>
          </div>
          {smtpConfigured && <EmailTestPanel />}
        </CardContent>
      </Card>

      {/* Notification Triggers */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Bell className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.email.notifications")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.email.notificationsHint")}</p>
            </div>
          </div>

          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="em-lowbal" className="mb-0">{t("adminSettings.fields.balance_low_notify_enabled")}</Label>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.email.lowBalanceHint")}</p>
            </div>
            <Switch id="em-lowbal" checked={value.balance_low_notify_enabled === true}
              onCheckedChange={(v) => onField("balance_low_notify_enabled", v)} />
          </div>
          {value.balance_low_notify_enabled === true && (
            <div className="grid gap-4 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4 sm:grid-cols-2">
              <div>
                <Label htmlFor="em-threshold">{t("adminSettings.fields.balance_low_notify_threshold")}</Label>
                <Input id="em-threshold" value={str("balance_low_notify_threshold")} placeholder="5.00"
                  onChange={(e) => onField("balance_low_notify_threshold", e.target.value)} />
              </div>
              <div>
                <Label htmlFor="em-recharge">{t("adminSettings.fields.balance_low_notify_recharge_url")}</Label>
                <Input id="em-recharge" value={str("balance_low_notify_recharge_url")} placeholder="https://..."
                  onChange={(e) => onField("balance_low_notify_recharge_url", e.target.value)} />
              </div>
            </div>
          )}

          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="em-subexpiry" className="mb-0">{t("adminSettings.fields.subscription_expiry_notify_enabled")}</Label>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.email.subscriptionExpiryHint")}</p>
            </div>
            <Switch id="em-subexpiry" checked={value.subscription_expiry_notify_enabled === true}
              onCheckedChange={(v) => onField("subscription_expiry_notify_enabled", v)} />
          </div>

          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="em-quota" className="mb-0">{t("adminSettings.fields.account_quota_notify_enabled")}</Label>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.email.quotaAlertHint")}</p>
            </div>
            <Switch id="em-quota" checked={value.account_quota_notify_enabled === true}
              onCheckedChange={(v) => onField("account_quota_notify_enabled", v)} />
          </div>
        </CardContent>
      </Card>

      {/* Email Templates */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Mail className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.email.templates")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.email.templatesHint")}</p>
            </div>
          </div>
          {(SPECIAL_FIELDS.email ?? []).map((field) => (
            <SpecialFieldRow key={String(field.key)} field={field} draft={draft} onChange={onSpecial} modelOptions={modelOptions} />
          ))}
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>{t("adminSettings.saveSection")}</Button>
      </div>
    </div>
  );
}
