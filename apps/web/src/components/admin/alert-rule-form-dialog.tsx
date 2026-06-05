"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { useCreateOpsAlertRule, useUpdateOpsAlertRule } from "@/hooks/admin-queries";
import type { OpsAlertRule, OpsAlertMetricType, OpsAlertOperator, OpsAlertSeverity } from "@/lib/sdk-types";

const METRIC_TYPES: OpsAlertMetricType[] = ["error_rate", "success_rate", "latency_p95", "request_count"];
const OPERATORS: OpsAlertOperator[] = ["gt", "gte", "lt", "lte"];
const SEVERITIES: OpsAlertSeverity[] = ["warning", "critical", "ticket"];

type FormState = {
  name: string;
  metricType: OpsAlertMetricType;
  operator: OpsAlertOperator;
  threshold: string;
  severity: OpsAlertSeverity;
  enabled: boolean;
  windowSeconds: string;
  cooldownSeconds: string;
  minRequestCount: string;
  sourceEndpoint: string;
  model: string;
  providerId: string;
};

function emptyForm(): FormState {
  return {
    name: "",
    metricType: "error_rate",
    operator: "gt",
    threshold: "0.05",
    severity: "warning",
    enabled: true,
    windowSeconds: "3600",
    cooldownSeconds: "0",
    minRequestCount: "0",
    sourceEndpoint: "",
    model: "",
    providerId: "",
  };
}

function formFromRule(rule: OpsAlertRule): FormState {
  return {
    name: rule.name,
    metricType: rule.metric_type,
    operator: rule.operator,
    threshold: String(rule.threshold),
    severity: rule.severity,
    enabled: rule.enabled,
    windowSeconds: String(rule.window_seconds),
    cooldownSeconds: String(rule.cooldown_seconds),
    minRequestCount: String(rule.min_request_count),
    sourceEndpoint: rule.scope.source_endpoint ?? "",
    model: rule.scope.model ?? "",
    providerId: rule.scope.provider_id ?? "",
  };
}

function buildScope(form: FormState) {
  const scope: { source_endpoint: string; model: string; provider_id?: string } = {
    source_endpoint: form.sourceEndpoint.trim(),
    model: form.model.trim(),
  };
  if (form.providerId.trim()) scope.provider_id = form.providerId.trim();
  return scope;
}

/** Create / edit a configurable generic-metric alert rule. */
export function AlertRuleFormDialog({
  open,
  onOpenChange,
  target,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  target: OpsAlertRule | null;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const createMut = useCreateOpsAlertRule();
  const updateMut = useUpdateOpsAlertRule();
  const mode = target ? "edit" : "create";

  const [form, setForm] = useState<FormState>(() => (target ? formFromRule(target) : emptyForm()));
  const [error, setError] = useState<string | null>(null);
  const busy = createMut.isPending || updateMut.isPending;

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const scope = buildScope(form);
    try {
      if (mode === "edit" && target) {
        await updateMut.mutateAsync({
          id: target.id,
          body: {
            name: form.name.trim(),
            metric_type: form.metricType,
            operator: form.operator,
            threshold: Number(form.threshold),
            severity: form.severity,
            enabled: form.enabled,
            window_seconds: Number(form.windowSeconds),
            cooldown_seconds: Number(form.cooldownSeconds),
            min_request_count: Number(form.minRequestCount),
            scope,
          },
        });
      } else {
        await createMut.mutateAsync({
          name: form.name.trim(),
          metric_type: form.metricType,
          operator: form.operator,
          threshold: Number(form.threshold),
          severity: form.severity,
          enabled: form.enabled,
          window_seconds: Number(form.windowSeconds),
          cooldown_seconds: Number(form.cooldownSeconds),
          min_request_count: Number(form.minRequestCount),
          scope,
        });
      }
      toast({ title: t(mode === "create" ? "feedback.created" : "feedback.updated"), tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={onSubmit}>
          <DialogHeader>
            <DialogTitle>
              {t(mode === "create" ? "adminOps.alertRules.create" : "adminOps.alertRules.edit")}
            </DialogTitle>
            {target ? <DialogDescription>{target.name}</DialogDescription> : null}
          </DialogHeader>

          <div className="mt-4 max-h-[62vh] space-y-4 overflow-y-auto pr-1">
            <div>
              <Label htmlFor="rule-name">{t("adminOps.alertRules.name")}</Label>
              <Input id="rule-name" value={form.name} disabled={busy} onChange={(e) => set("name", e.target.value)} />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="rule-metric">{t("adminOps.alertRules.metric")}</Label>
                <Select value={form.metricType} onValueChange={(v) => set("metricType", v as OpsAlertMetricType)} disabled={busy}>
                  <SelectTrigger id="rule-metric">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {METRIC_TYPES.map((m) => (
                      <SelectItem key={m} value={m}>
                        {t(`adminOps.alertRules.metricType.${m}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="rule-operator">{t("adminOps.alertRules.operator")}</Label>
                <Select value={form.operator} onValueChange={(v) => set("operator", v as OpsAlertOperator)} disabled={busy}>
                  <SelectTrigger id="rule-operator">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {OPERATORS.map((o) => (
                      <SelectItem key={o} value={o}>
                        {t(`adminOps.alertRules.operators.${o}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="rule-threshold">{t("adminOps.alertRules.threshold")}</Label>
                <Input id="rule-threshold" type="number" step="any" value={form.threshold} disabled={busy} onChange={(e) => set("threshold", e.target.value)} />
              </div>
              <div>
                <Label htmlFor="rule-severity">{t("adminOps.alertRules.severity")}</Label>
                <Select value={form.severity} onValueChange={(v) => set("severity", v as OpsAlertSeverity)} disabled={busy}>
                  <SelectTrigger id="rule-severity">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SEVERITIES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <p className="text-2xs text-srapi-text-tertiary">{t("adminOps.alertRules.thresholdHint")}</p>

            <div className="grid grid-cols-3 gap-3">
              <div>
                <Label htmlFor="rule-window">{t("adminOps.alertRules.window")}</Label>
                <Input id="rule-window" type="number" value={form.windowSeconds} disabled={busy} onChange={(e) => set("windowSeconds", e.target.value)} />
              </div>
              <div>
                <Label htmlFor="rule-cooldown">{t("adminOps.alertRules.cooldown")}</Label>
                <Input id="rule-cooldown" type="number" value={form.cooldownSeconds} disabled={busy} onChange={(e) => set("cooldownSeconds", e.target.value)} />
              </div>
              <div>
                <Label htmlFor="rule-min">{t("adminOps.alertRules.minRequests")}</Label>
                <Input id="rule-min" type="number" value={form.minRequestCount} disabled={busy} onChange={(e) => set("minRequestCount", e.target.value)} />
              </div>
            </div>

            <div>
              <Label htmlFor="rule-endpoint">{t("adminOps.alertRules.endpoint")}</Label>
              <Input id="rule-endpoint" value={form.sourceEndpoint} placeholder="/v1/chat/completions" disabled={busy} onChange={(e) => set("sourceEndpoint", e.target.value)} />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="rule-model">{t("adminOps.alertRules.model")}</Label>
                <Input id="rule-model" value={form.model} disabled={busy} onChange={(e) => set("model", e.target.value)} />
              </div>
              <div>
                <Label htmlFor="rule-provider">{t("adminOps.alertRules.provider")}</Label>
                <Input id="rule-provider" value={form.providerId} disabled={busy} onChange={(e) => set("providerId", e.target.value)} />
              </div>
            </div>

            <label className="flex items-center justify-between">
              <span className="text-sm text-srapi-text-secondary">{t("adminOps.alertRules.enabled")}</span>
              <Switch checked={form.enabled} disabled={busy} onCheckedChange={(checked) => set("enabled", checked)} />
            </label>

            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>

          <DialogFooter className="mt-6">
            <Button type="button" variant="ghost" disabled={busy} onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={busy}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
