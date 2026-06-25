"use client";

import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";

const TIMEZONES = [
  "UTC", "Asia/Shanghai", "Asia/Tokyo", "Asia/Seoul", "Asia/Singapore",
  "Europe/London", "Europe/Paris", "Europe/Berlin",
  "America/New_York", "America/Chicago", "America/Los_Angeles",
  "Australia/Sydney", "Pacific/Auckland",
];

interface QuotaControlProps {
  extra: Record<string, unknown>;
  onExtraChange: (extra: Record<string, unknown>) => void;
  disabled?: boolean;
}

export function QuotaControl({ extra, onExtraChange, disabled }: QuotaControlProps) {
  const { t } = useLanguage();
  const [enabled, setEnabled] = useState(
    Boolean(extra.quota_limit || extra.quota_daily_limit || extra.quota_weekly_limit),
  );

  function update(key: string, value: unknown) {
    const next = { ...extra };
    if (value === null || value === undefined || value === "" || value === 0) {
      delete next[key];
    } else {
      next[key] = value;
    }
    onExtraChange(next);
  }

  function numVal(key: string): string {
    const v = extra[key];
    if (v === null || v === undefined) return "";
    return String(v);
  }

  if (!enabled) {
    return (
      <div className="flex items-center justify-between rounded-lg border border-srapi-border p-3">
        <div>
          <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.quota.title")}</span>
          <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.quota.hint")}</p>
        </div>
        <Switch checked={false} onCheckedChange={() => setEnabled(true)} disabled={disabled} />
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-srapi-border p-3.5 space-y-4">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.quota.title")}</span>
        <Switch checked onCheckedChange={() => { setEnabled(false); onExtraChange({}); }} disabled={disabled} />
      </div>

      <div className="grid grid-cols-3 gap-3">
        <div>
          <Label className="text-xs">{t("adminAccounts.quota.total")}</Label>
          <Input
            type="number"
            min={0}
            placeholder="0"
            value={numVal("quota_limit")}
            disabled={disabled}
            onChange={(e) => update("quota_limit", e.target.value ? Number(e.target.value) : null)}
          />
        </div>
        <div>
          <Label className="text-xs">{t("adminAccounts.quota.daily")}</Label>
          <Input
            type="number"
            min={0}
            placeholder="0"
            value={numVal("quota_daily_limit")}
            disabled={disabled}
            onChange={(e) => update("quota_daily_limit", e.target.value ? Number(e.target.value) : null)}
          />
        </div>
        <div>
          <Label className="text-xs">{t("adminAccounts.quota.weekly")}</Label>
          <Input
            type="number"
            min={0}
            placeholder="0"
            value={numVal("quota_weekly_limit")}
            disabled={disabled}
            onChange={(e) => update("quota_weekly_limit", e.target.value ? Number(e.target.value) : null)}
          />
        </div>
      </div>

      <div>
        <Label className="text-xs">{t("adminAccounts.quota.resetTimezone")}</Label>
        <Select
          value={(extra.quota_reset_timezone as string) ?? "UTC"}
          onValueChange={(v) => update("quota_reset_timezone", v)}
          disabled={disabled}
        >
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            {TIMEZONES.map((tz) => <SelectItem key={tz} value={tz}>{tz}</SelectItem>)}
          </SelectContent>
        </Select>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label className="text-xs">{t("adminAccounts.quota.windowCost")}</Label>
          <Input
            type="number"
            min={0}
            step={0.01}
            placeholder="0"
            value={numVal("window_cost_limit")}
            disabled={disabled}
            onChange={(e) => update("window_cost_limit", e.target.value ? Number(e.target.value) : null)}
          />
          <p className="mt-0.5 text-[10px] text-srapi-text-tertiary">{t("adminAccounts.quota.windowCostHint")}</p>
        </div>
        <div>
          <Label className="text-xs">{t("adminAccounts.quota.maxSessions")}</Label>
          <Input
            type="number"
            min={0}
            placeholder="0"
            value={numVal("max_sessions")}
            disabled={disabled}
            onChange={(e) => update("max_sessions", e.target.value ? Number(e.target.value) : null)}
          />
        </div>
      </div>
    </div>
  );
}
