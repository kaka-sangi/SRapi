"use client";

import { useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useLanguage } from "@/context/LanguageContext";

interface TempUnschedRule {
  error_code: number | null;
  keywords: string;
  duration_minutes: number | null;
  description: string;
}

interface TempUnschedRulesProps {
  extra: Record<string, unknown>;
  onExtraChange: (extra: Record<string, unknown>) => void;
  disabled?: boolean;
}

function parseRules(extra: Record<string, unknown>): TempUnschedRule[] {
  const raw = extra.temp_unschedulable_rules;
  if (!Array.isArray(raw)) return [];
  return raw.map((r: unknown) => {
    if (!r || typeof r !== "object") return null;
    const obj = r as Record<string, unknown>;
    return {
      error_code: typeof obj.error_code === "number" ? obj.error_code : null,
      keywords: typeof obj.keywords === "string" ? obj.keywords : "",
      duration_minutes: typeof obj.duration_minutes === "number" ? obj.duration_minutes : null,
      description: typeof obj.description === "string" ? obj.description : "",
    };
  }).filter(Boolean) as TempUnschedRule[];
}

export function TempUnschedRules({ extra, onExtraChange, disabled }: TempUnschedRulesProps) {
  const { t } = useLanguage();
  const [enabled, setEnabled] = useState(parseRules(extra).length > 0);
  const [rules, setRules] = useState<TempUnschedRule[]>(() => parseRules(extra));

  function sync(next: TempUnschedRule[]) {
    setRules(next);
    const updated = { ...extra };
    if (next.length === 0) {
      delete updated.temp_unschedulable_rules;
    } else {
      updated.temp_unschedulable_rules = next.filter(
        (r) => r.error_code || r.keywords.trim(),
      );
    }
    onExtraChange(updated);
  }

  function addRule() {
    sync([...rules, { error_code: 429, keywords: "", duration_minutes: 5, description: "" }]);
  }

  function removeRule(index: number) {
    sync(rules.filter((_, i) => i !== index));
  }

  function updateRule(index: number, patch: Partial<TempUnschedRule>) {
    const next = rules.map((r, i) => (i === index ? { ...r, ...patch } : r));
    sync(next);
  }

  if (!enabled) {
    return (
      <div className="flex items-center justify-between rounded-lg border border-srapi-border p-3">
        <div>
          <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.tempUnsched.title")}</span>
          <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.tempUnsched.hint")}</p>
        </div>
        <Switch checked={false} onCheckedChange={() => { setEnabled(true); addRule(); }} disabled={disabled} />
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-srapi-border p-3.5 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.tempUnsched.title")}</span>
        <Switch checked onCheckedChange={() => { setEnabled(false); sync([]); }} disabled={disabled} />
      </div>

      {rules.map((rule, i) => (
        <div key={i} className="flex items-start gap-2 rounded border border-srapi-border p-2.5">
          <div className="grid flex-1 grid-cols-4 gap-2">
            <div>
              <Label className="text-[10px]">{t("adminAccounts.tempUnsched.errorCode")}</Label>
              <Input
                type="number"
                min={100}
                max={599}
                placeholder="429"
                value={rule.error_code ?? ""}
                disabled={disabled}
                onChange={(e) => updateRule(i, { error_code: e.target.value ? Number(e.target.value) : null })}
              />
            </div>
            <div>
              <Label className="text-[10px]">{t("adminAccounts.tempUnsched.keywords")}</Label>
              <Input
                placeholder="rate_limit"
                value={rule.keywords}
                disabled={disabled}
                onChange={(e) => updateRule(i, { keywords: e.target.value })}
              />
            </div>
            <div>
              <Label className="text-[10px]">{t("adminAccounts.tempUnsched.duration")}</Label>
              <Input
                type="number"
                min={1}
                placeholder="5"
                value={rule.duration_minutes ?? ""}
                disabled={disabled}
                onChange={(e) => updateRule(i, { duration_minutes: e.target.value ? Number(e.target.value) : null })}
              />
            </div>
            <div>
              <Label className="text-[10px]">{t("adminAccounts.tempUnsched.desc")}</Label>
              <Input
                placeholder=""
                value={rule.description}
                disabled={disabled}
                onChange={(e) => updateRule(i, { description: e.target.value })}
              />
            </div>
          </div>
          <button type="button" onClick={() => removeRule(i)} disabled={disabled} className="mt-4 text-srapi-text-tertiary hover:text-red-500">
            <Trash2 className="size-3.5" />
          </button>
        </div>
      ))}

      <Button type="button" variant="outline" size="sm" onClick={addRule} disabled={disabled}>
        <Plus className="size-3.5" />
        {t("adminAccounts.tempUnsched.addRule")}
      </Button>
    </div>
  );
}
