"use client";

import { Switch } from "@/components/ui/switch";
import { useLanguage } from "@/context/LanguageContext";

interface AntigravityConfigProps {
  extra: Record<string, unknown>;
  onExtraChange: (extra: Record<string, unknown>) => void;
  disabled?: boolean;
}

export function AntigravityConfig({ extra, onExtraChange, disabled }: AntigravityConfigProps) {
  const { t } = useLanguage();

  function toggle(key: string, value: boolean) {
    const updated = { ...extra };
    if (!value) {
      delete updated[key];
    } else {
      updated[key] = true;
    }
    onExtraChange(updated);
  }

  return (
    <div className="rounded-lg border border-srapi-border p-3.5 space-y-3">
      <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.antigravity.title")}</span>

      <div className="flex items-center justify-between">
        <div>
          <span className="text-sm text-srapi-text-primary">{t("adminAccounts.antigravity.mixedScheduling")}</span>
          <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.antigravity.mixedSchedulingHint")}</p>
        </div>
        <Switch
          checked={Boolean(extra.mixed_scheduling)}
          onCheckedChange={(v) => toggle("mixed_scheduling", v)}
          disabled={disabled}
        />
      </div>

      <div className="flex items-center justify-between">
        <div>
          <span className="text-sm text-srapi-text-primary">{t("adminAccounts.antigravity.allowOverages")}</span>
          <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.antigravity.allowOveragesHint")}</p>
        </div>
        <Switch
          checked={Boolean(extra.allow_overages)}
          onCheckedChange={(v) => toggle("allow_overages", v)}
          disabled={disabled}
        />
      </div>
    </div>
  );
}
