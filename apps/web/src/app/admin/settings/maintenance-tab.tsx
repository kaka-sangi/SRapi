"use client";

import { useMemo } from "react";
import { AlertTriangle, ShieldOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useLanguage } from "@/context/LanguageContext";
import type { AdminSettingsMaintenance } from "../../../../../../packages/sdk/typescript/src/types.gen";

interface MaintenanceTabProps {
  value: AdminSettingsMaintenance;
  onField: <K extends keyof AdminSettingsMaintenance>(key: K, value: AdminSettingsMaintenance[K]) => void;
  onSave: () => void;
  pending: boolean;
}

const MESSAGE_MAX_LEN = 1024;

export function MaintenanceTab({ value, onField, onSave, pending }: MaintenanceTabProps) {
  const { t } = useLanguage();
  const recoveryLocal = useMemo(() => isoToDatetimeLocal(value.expected_recovery_at), [value.expected_recovery_at]);
  const messageLen = (value.message ?? "").length;
  const messageRemaining = Math.max(0, MESSAGE_MAX_LEN - messageLen);

  return (
    <Card>
      <CardContent className="space-y-5">
        <div className="flex items-center justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="maintenance-enabled" className="mb-0 flex items-center gap-2">
              <ShieldOff className="size-4 text-srapi-warning" aria-hidden />
              {t("adminSettings.maintenance.enabledLabel")}
            </Label>
            <p className="text-xs text-srapi-text-tertiary">
              {t("adminSettings.maintenance.enabledHelp")}
            </p>
          </div>
          <Switch
            id="maintenance-enabled"
            checked={value.enabled}
            onCheckedChange={(checked) => onField("enabled", checked)}
          />
        </div>

        <div>
          <Label htmlFor="maintenance-message">{t("adminSettings.maintenance.messageLabel")}</Label>
          <Textarea
            id="maintenance-message"
            rows={4}
            maxLength={MESSAGE_MAX_LEN}
            value={value.message ?? ""}
            placeholder={t("adminSettings.maintenance.messagePlaceholder")}
            onChange={(e) => onField("message", e.target.value)}
          />
          <p className="mt-1 text-right text-xs tabular text-srapi-text-tertiary">
            {messageRemaining} / {MESSAGE_MAX_LEN}
          </p>
        </div>

        <div>
          <Label htmlFor="maintenance-recovery">{t("adminSettings.maintenance.recoveryLabel")}</Label>
          <Input
            id="maintenance-recovery"
            type="datetime-local"
            value={recoveryLocal}
            onChange={(e) =>
              onField("expected_recovery_at", datetimeLocalToISO(e.target.value) as never)
            }
          />
          <p className="mt-1 text-xs text-srapi-text-tertiary">
            {t("adminSettings.maintenance.recoveryHelp")}
          </p>
        </div>

        {value.enabled ? (
          <div className="flex items-start gap-2 rounded-xl border border-srapi-warning/40 bg-srapi-warning/10 p-3 text-xs text-srapi-text-secondary">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-srapi-warning" aria-hidden />
            <p>{t("adminSettings.maintenance.activeBanner")}</p>
          </div>
        ) : null}

        <div className="flex justify-end border-t border-srapi-border/70 pt-4">
          <Button variant="primary" loading={pending} onClick={onSave}>
            {t("adminSettings.saveSection")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

/**
 * Convert an ISO-8601 timestamp to the `YYYY-MM-DDTHH:mm` string the
 * `datetime-local` input expects. Strips the seconds and timezone offset so
 * the browser renders the operator's local clock-time.
 */
function isoToDatetimeLocal(iso: string | undefined): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/**
 * Convert the local datetime string back to a UTC ISO timestamp the API
 * accepts. Returns undefined for empty values so the field clears on save.
 */
function datetimeLocalToISO(local: string): string | undefined {
  const trimmed = local.trim();
  if (!trimmed) return undefined;
  const date = new Date(trimmed);
  if (Number.isNaN(date.getTime())) return undefined;
  return date.toISOString();
}
