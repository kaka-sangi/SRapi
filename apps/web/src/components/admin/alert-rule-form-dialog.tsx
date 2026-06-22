"use client";

import { useMemo, useState } from "react";
import { AlertTriangle, Bell, ShieldAlert } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { FloatingInput } from "@/components/ui/floating-input";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { SectionTitle } from "@/components/ui/section-title";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { useCreateOpsAlertRule, useUpdateOpsAlertRule } from "@/hooks/admin-queries";
import type {
  OpsAlertRule,
  OpsAlertMetricType,
  OpsAlertOperator,
  OpsAlertSeverity,
} from "@/lib/sdk-types";

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
  errorClass: string;
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
    errorClass: "",
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
    errorClass: rule.scope.error_class ?? "",
    providerId: rule.scope.provider_id ?? "",
  };
}

function buildScope(form: FormState) {
  const scope: { source_endpoint: string; model: string; error_class: string; provider_id?: string } = {
    source_endpoint: form.sourceEndpoint.trim(),
    model: form.model.trim(),
    error_class: form.errorClass.trim(),
  };
  if (form.providerId.trim()) scope.provider_id = form.providerId.trim();
  return scope;
}

function buildBody(form: FormState) {
  return {
    name: form.name.trim(),
    metric_type: form.metricType,
    operator: form.operator,
    threshold: Number(form.threshold),
    severity: form.severity,
    enabled: form.enabled,
    window_seconds: Number(form.windowSeconds),
    cooldown_seconds: Number(form.cooldownSeconds),
    min_request_count: Number(form.minRequestCount),
    scope: buildScope(form),
  };
}

function severityTone(severity: OpsAlertSeverity): "warning" | "error" | "accent" {
  switch (severity) {
    case "critical":
      return "error";
    case "warning":
      return "warning";
    default:
      return "accent";
  }
}

function severityBubbleTone(severity: OpsAlertSeverity): "warning" | "error" | "accent" {
  return severityTone(severity);
}

function severityIcon(severity: OpsAlertSeverity) {
  switch (severity) {
    case "critical":
      return <ShieldAlert />;
    case "warning":
      return <AlertTriangle />;
    default:
      return <Bell />;
  }
}

function formatMetricValue(metric: OpsAlertMetricType, threshold: string): string {
  const n = Number(threshold);
  if (!Number.isFinite(n)) return threshold || "—";
  if (metric === "error_rate" || metric === "success_rate") {
    return `${(n * 100).toFixed(n >= 0.1 ? 0 : 1)}%`;
  }
  if (metric === "latency_p95") return `${n.toFixed(0)} ms`;
  return String(Math.round(n));
}

function formatWindow(seconds: string): string {
  const n = Number(seconds);
  if (!Number.isFinite(n) || n <= 0) return "—";
  if (n % 3600 === 0) return `${n / 3600}h`;
  if (n % 60 === 0) return `${n / 60}m`;
  return `${n}s`;
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
  const [submitting, setSubmitting] = useState(false);

  const nameError = form.name.trim().length === 0 ? t("adminCommon.required") : undefined;
  const thresholdError =
    form.threshold.trim().length === 0 || !Number.isFinite(Number(form.threshold))
      ? t("adminCommon.required")
      : undefined;
  const hasError = Boolean(nameError || thresholdError);

  const scopeChips = useMemo(() => {
    const chips: { label: string; value: string }[] = [];
    if (form.sourceEndpoint.trim()) chips.push({ label: t("adminOps.alertRules.endpoint"), value: form.sourceEndpoint.trim() });
    if (form.model.trim()) chips.push({ label: t("adminOps.alertRules.model"), value: form.model.trim() });
    if (form.errorClass.trim()) chips.push({ label: t("adminOps.alertRules.errorClass"), value: form.errorClass.trim() });
    if (form.providerId.trim()) chips.push({ label: t("adminOps.alertRules.provider"), value: form.providerId.trim() });
    return chips;
  }, [form.sourceEndpoint, form.model, form.errorClass, form.providerId, t]);

  function update<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    if (hasError) return;
    setError(null);
    setSubmitting(true);
    try {
      const body = buildBody(form);
      if (mode === "edit" && target) {
        await updateMut.mutateAsync({ id: target.id, body });
      } else {
        await createMut.mutateAsync(body);
      }
      toast({ title: t(mode === "create" ? "feedback.created" : "feedback.updated"), tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setSubmitting(false);
    }
  }

  const busy = submitting || createMut.isPending || updateMut.isPending;
  const tone = severityTone(form.severity);
  const bubbleTone = severityBubbleTone(form.severity);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t(mode === "create" ? "adminOps.alertRules.create" : "adminOps.alertRules.edit")}
          </DialogTitle>
          <DialogDescription className="sr-only">
            {t("adminOps.alertRules.subtitle")}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} noValidate className="space-y-5">
          {/* Severity-aware preview */}
          <div className="flex items-start gap-3 rounded-xl border border-srapi-border bg-srapi-card-muted/60 p-4">
            <IconBubble tone={bubbleTone} size="md">
              {severityIcon(form.severity)}
            </IconBubble>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-1.5">
                <DataPill tone={tone} size="sm">{form.severity}</DataPill>
                <span className="text-[11px] uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {form.enabled ? t("adminOps.alertRules.enabled") : t("common.disabled")}
                </span>
              </div>
              <p className="mt-1.5 break-words text-sm font-semibold tracking-tight text-srapi-text-primary">
                {form.name.trim() || t("adminOps.alertRules.name")}
              </p>
              <p className="mt-1 text-xs text-srapi-text-secondary">
                {t(`adminOps.alertRules.metricType.${form.metricType}`)}{" "}
                <span className="text-srapi-text-tertiary">{t(`adminOps.alertRules.operators.${form.operator}`)}</span>{" "}
                <span className="font-mono text-srapi-text-primary tabular">
                  {formatMetricValue(form.metricType, form.threshold)}
                </span>{" "}
                <span className="text-srapi-text-tertiary">·</span>{" "}
                <span className="tabular">{formatWindow(form.windowSeconds)}</span>
              </p>
              {scopeChips.length > 0 ? (
                <div className="mt-2 flex flex-wrap gap-1">
                  {scopeChips.map((chip) => (
                    <DataPill key={chip.label} tone="neutral" size="sm" className="font-mono">
                      <span className="text-srapi-text-tertiary">{chip.label}:</span>
                      <span className="ml-1 text-srapi-text-primary">{chip.value}</span>
                    </DataPill>
                  ))}
                </div>
              ) : (
                <p className="mt-2 text-[11px] text-srapi-text-tertiary">{t("adminOps.alertRules.globalScope")}</p>
              )}
            </div>
          </div>

          {/* Severity segmented control */}
          <div>
            <Label className="mb-1.5 block">{t("adminOps.alertRules.severity")}</Label>
            <SegmentedControl<OpsAlertSeverity>
              value={form.severity}
              onChange={(value) => update("severity", value)}
              options={SEVERITIES.map((s) => ({ value: s, label: s }))}
              ariaLabel={t("adminOps.alertRules.severity")}
              size="sm"
            />
          </div>

          {/* Name */}
          <FloatingInput
            label={t("adminOps.alertRules.name")}
            value={form.name}
            onChange={(v) => update("name", v)}
            error={nameError && form.name !== "" ? nameError : undefined}
            required
            disabled={busy}
          />

          {/* Metric / Operator / Threshold */}
          <div className="grid gap-3 sm:grid-cols-[1.2fr_0.9fr_1fr]">
            <div>
              <Label htmlFor="ar-metric" className="mb-1.5 block">{t("adminOps.alertRules.metric")}</Label>
              <Select
                value={form.metricType}
                onValueChange={(v) => update("metricType", v as OpsAlertMetricType)}
                disabled={busy}
              >
                <SelectTrigger id="ar-metric">
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
              <Label htmlFor="ar-op" className="mb-1.5 block">{t("adminOps.alertRules.operator")}</Label>
              <Select
                value={form.operator}
                onValueChange={(v) => update("operator", v as OpsAlertOperator)}
                disabled={busy}
              >
                <SelectTrigger id="ar-op">
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
            <div>
              <Label htmlFor="ar-thr" className="mb-1.5 block">{t("adminOps.alertRules.threshold")}</Label>
              <Input
                id="ar-thr"
                type="number"
                value={form.threshold}
                disabled={busy}
                aria-invalid={thresholdError ? true : undefined}
                onChange={(e) => update("threshold", e.target.value)}
              />
              <p className="mt-1 text-[11px] text-srapi-text-tertiary">{t("adminOps.alertRules.thresholdHint")}</p>
            </div>
          </div>

          {/* Window / Cooldown / Min requests */}
          <div className="grid gap-3 sm:grid-cols-3">
            <div>
              <Label htmlFor="ar-win" className="mb-1.5 block">{t("adminOps.alertRules.window")}</Label>
              <Input
                id="ar-win"
                type="number"
                value={form.windowSeconds}
                disabled={busy}
                onChange={(e) => update("windowSeconds", e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="ar-cd" className="mb-1.5 block">{t("adminOps.alertRules.cooldown")}</Label>
              <Input
                id="ar-cd"
                type="number"
                value={form.cooldownSeconds}
                disabled={busy}
                onChange={(e) => update("cooldownSeconds", e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="ar-min" className="mb-1.5 block">{t("adminOps.alertRules.minRequests")}</Label>
              <Input
                id="ar-min"
                type="number"
                value={form.minRequestCount}
                disabled={busy}
                onChange={(e) => update("minRequestCount", e.target.value)}
              />
            </div>
          </div>

          {/* Scope section */}
          <div className="space-y-3 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
            <SectionTitle
              label={t("adminOps.alertRules.endpoint")}
              action={
                scopeChips.length === 0 ? (
                  <span className="text-[11px] text-srapi-text-tertiary">
                    {t("adminOps.alertRules.globalScope")}
                  </span>
                ) : null
              }
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="ar-ep" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.alertRules.endpoint")}
                </Label>
                <Input
                  id="ar-ep"
                  value={form.sourceEndpoint}
                  placeholder="/v1/chat/completions"
                  disabled={busy}
                  onChange={(e) => update("sourceEndpoint", e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="ar-model" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.alertRules.model")}
                </Label>
                <Input
                  id="ar-model"
                  value={form.model}
                  disabled={busy}
                  onChange={(e) => update("model", e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="ar-ec" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.alertRules.errorClass")}
                </Label>
                <Input
                  id="ar-ec"
                  value={form.errorClass}
                  placeholder="provider_5xx"
                  disabled={busy}
                  onChange={(e) => update("errorClass", e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="ar-prov" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.alertRules.provider")}
                </Label>
                <Input
                  id="ar-prov"
                  value={form.providerId}
                  disabled={busy}
                  onChange={(e) => update("providerId", e.target.value)}
                />
              </div>
            </div>
          </div>

          {/* Enabled switch */}
          <div className="flex items-center justify-between gap-4 rounded-xl border border-srapi-border bg-srapi-card px-4 py-3">
            <div className="min-w-0">
              <Label htmlFor="ar-enabled" className="mb-0">{t("adminOps.alertRules.enabled")}</Label>
            </div>
            <Switch
              id="ar-enabled"
              checked={form.enabled}
              disabled={busy}
              onCheckedChange={(checked) => update("enabled", checked)}
            />
          </div>

          {error ? (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={busy} disabled={hasError}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
