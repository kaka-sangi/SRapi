import { useMemo } from "react";
import { useLanguage } from "@/context/LanguageContext";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { type SettingsTab } from "@/lib/admin-settings-form";
import { SPECIAL_FIELDS, GATEWAY_NON_NEGATIVE_INT_FIELDS, fieldLabel } from "./settings-fields";

/** Auto-render the primitive fields of a settings section as typed inputs. */
export function SectionFields({
  section,
  value,
  onChange,
}: {
  section: SettingsTab;
  value: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}) {
  const { t } = useLanguage();
  const skip = useMemo(
    () => new Set((SPECIAL_FIELDS[section] ?? []).map((f) => f.skip)),
    [section],
  );

  const entries = Object.entries(value).filter(([key, v]) => {
    if (skip.has(key)) return false;
    const type = typeof v;
    return v === null || type === "boolean" || type === "number" || type === "string";
  });

  if (entries.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-5 sm:grid-cols-2">
      {entries.map(([key, v]) => {
        const id = `f-${section}-${key}`;
        if (typeof v === "boolean") {
          return (
            <div key={key} className="flex items-center justify-between gap-4 sm:col-span-2">
              <Label htmlFor={id} className="mb-0">
                {fieldLabel(key, t)}
              </Label>
              <Switch id={id} checked={v} onCheckedChange={(checked) => onChange(key, checked)} />
            </div>
          );
        }
        if (typeof v === "number") {
          const clamp = GATEWAY_NON_NEGATIVE_INT_FIELDS.has(key)
            ? (n: number) => (Number.isFinite(n) ? Math.max(0, Math.trunc(n)) : 0)
            : (n: number) => n;
          return (
            <div key={key}>
              <Label htmlFor={id}>{fieldLabel(key, t)}</Label>
              <Input
                id={id}
                type="number"
                min={GATEWAY_NON_NEGATIVE_INT_FIELDS.has(key) ? 0 : undefined}
                value={String(v)}
                onChange={(e) =>
                  onChange(key, e.target.value === "" ? 0 : clamp(Number(e.target.value)))
                }
              />
            </div>
          );
        }
        return (
          <div key={key}>
            <Label htmlFor={id}>{fieldLabel(key, t)}</Label>
            <Input
              id={id}
              value={v == null ? "" : String(v)}
              onChange={(e) => onChange(key, e.target.value)}
            />
          </div>
        );
      })}
    </div>
  );
}
