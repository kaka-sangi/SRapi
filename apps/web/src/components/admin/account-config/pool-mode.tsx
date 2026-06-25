"use client";

import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useLanguage } from "@/context/LanguageContext";

interface PoolModeProps {
  extra: Record<string, unknown>;
  onExtraChange: (extra: Record<string, unknown>) => void;
  disabled?: boolean;
}

export function PoolMode({ extra, onExtraChange, disabled }: PoolModeProps) {
  const { t } = useLanguage();
  const [enabled, setEnabled] = useState(Boolean(extra.pool_mode));

  function update(key: string, value: unknown) {
    const next = { ...extra };
    if (value === null || value === undefined || value === "") {
      delete next[key];
    } else {
      next[key] = value;
    }
    onExtraChange(next);
  }

  if (!enabled) {
    return (
      <div className="flex items-center justify-between rounded-lg border border-srapi-border p-3">
        <div>
          <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.poolMode.title")}</span>
          <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.poolMode.hint")}</p>
        </div>
        <Switch checked={false} onCheckedChange={() => { setEnabled(true); update("pool_mode", true); }} disabled={disabled} />
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-srapi-border p-3.5 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.poolMode.title")}</span>
        <Switch checked onCheckedChange={() => { setEnabled(false); update("pool_mode", false); }} disabled={disabled} />
      </div>
      <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.poolMode.enabledHint")}</p>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label className="text-xs">{t("adminAccounts.poolMode.retryCount")}</Label>
          <Input
            type="number"
            min={0}
            max={10}
            value={String(extra.pool_mode_retry_count ?? 3)}
            disabled={disabled}
            onChange={(e) => update("pool_mode_retry_count", e.target.value ? Number(e.target.value) : 3)}
          />
        </div>
        <div>
          <Label className="text-xs">{t("adminAccounts.poolMode.retryCodes")}</Label>
          <Input
            placeholder="401, 403, 429"
            value={String(extra.pool_mode_retry_status_codes ?? "")}
            disabled={disabled}
            onChange={(e) => update("pool_mode_retry_status_codes", e.target.value.trim())}
          />
        </div>
      </div>
    </div>
  );
}
