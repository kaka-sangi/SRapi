import { CreditCard, Store, BadgeDollarSign } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { type AdminSettingsDraft } from "@/lib/admin-settings-form";

const PAYMENT_PROVIDER_OPTIONS: MultiSelectOption[] = [
  { value: "stripe", label: "Stripe" },
  { value: "alipay", label: "Alipay (支付宝)" },
  { value: "wechat", label: "WeChat Pay (微信支付)" },
  { value: "easypay", label: "EasyPay" },
  { value: "airwallex", label: "Airwallex" },
  { value: "linuxdo", label: "LinuxDo" },
];

interface PaymentTabProps {
  paymentValue: Record<string, unknown>;
  featuresValue: Record<string, unknown>;
  draft: AdminSettingsDraft;
  onPaymentField: (key: string, v: unknown) => void;
  onFeaturesField: (key: string, v: unknown) => void;
  onSpecial: (key: keyof AdminSettingsDraft, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
  modelOptions: MultiSelectOption[];
}

export function PaymentTab({
  paymentValue,
  featuresValue,
  draft,
  onPaymentField,
  onFeaturesField,
  onSpecial,
  onSave,
  pending,
  modelOptions,
}: PaymentTabProps) {
  const { t } = useLanguage();
  const paymentsEnabled = featuresValue.payments_enabled === true;
  const paymentSystemEnabled = paymentValue.enabled === true;
  const subscriptionPlansEnabled = paymentValue.subscription_plans_enabled === true;

  return (
    <div className="space-y-6">
      {/* Master Switch */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <BadgeDollarSign className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">
                {t("adminSettings.payment.masterSwitch")}
              </h3>
              <p className="text-xs text-srapi-text-tertiary">
                {t("adminSettings.payment.masterSwitchHint")}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="pay-master" className="mb-0">
                {t("adminSettings.fields.payments_enabled")}
              </Label>
              <p className="text-xs text-srapi-text-tertiary">
                {t("adminSettings.payment.paymentsEnabledHint")}
              </p>
            </div>
            <Switch
              id="pay-master"
              checked={paymentsEnabled}
              onCheckedChange={(v) => onFeaturesField("payments_enabled", v)}
            />
          </div>
        </CardContent>
      </Card>

      {/* Payment System & Providers — only show when master switch is on */}
      {paymentsEnabled && (
        <Card>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-2">
              <CreditCard className="size-5 text-srapi-text-tertiary" />
              <div>
                <h3 className="text-sm font-semibold text-srapi-text-primary">
                  {t("adminSettings.payment.system")}
                </h3>
                <p className="text-xs text-srapi-text-tertiary">
                  {t("adminSettings.payment.systemHint")}
                </p>
              </div>
            </div>

            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="pay-enabled" className="mb-0">
                {t("adminSettings.payment.systemEnabled")}
              </Label>
              <Switch
                id="pay-enabled"
                checked={paymentSystemEnabled}
                onCheckedChange={(v) => onPaymentField("enabled", v)}
              />
            </div>

            {paymentSystemEnabled && (
              <div className="space-y-4 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
                <div>
                  <Label>{t("adminSettings.payment.providers")}</Label>
                  <p className="mb-1 text-xs text-srapi-text-tertiary">
                    {t("adminSettings.payment.providersHint")}
                  </p>
                  <MultiSelect
                    options={PAYMENT_PROVIDER_OPTIONS}
                    value={Array.isArray(draft.paymentProviders) ? draft.paymentProviders : []}
                    onChange={(next) => onSpecial("paymentProviders", next)}
                    placeholder={t("adminSettings.payment.providersPlaceholder")}
                  />
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Subscription Plans — only show when payment system is on */}
      {paymentsEnabled && paymentSystemEnabled && (
        <Card>
          <CardContent className="space-y-4">
            <div className="flex items-center gap-2">
              <Store className="size-5 text-srapi-text-tertiary" />
              <div>
                <h3 className="text-sm font-semibold text-srapi-text-primary">
                  {t("adminSettings.payment.subscriptionPlans")}
                </h3>
                <p className="text-xs text-srapi-text-tertiary">
                  {t("adminSettings.payment.subscriptionPlansHint")}
                </p>
              </div>
            </div>

            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="pay-plans" className="mb-0">
                {t("adminSettings.fields.subscription_plans_enabled")}
              </Label>
              <Switch
                id="pay-plans"
                checked={subscriptionPlansEnabled}
                onCheckedChange={(v) => onPaymentField("subscription_plans_enabled", v)}
              />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Save */}
      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>
          {t("adminSettings.saveSection")}
        </Button>
      </div>
    </div>
  );
}
