"use client";

import { useLanguage } from "@/context/LanguageContext";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
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
  const createMut = useCreateOpsAlertRule();
  const updateMut = useUpdateOpsAlertRule();
  const mode = target ? "edit" : "create";

  const fields: FieldConfig<FormState>[] = [
    { name: "name", label: t("adminOps.alertRules.name") },
    {
      name: "metricType",
      label: t("adminOps.alertRules.metric"),
      type: "select",
      options: METRIC_TYPES.map((m) => ({ value: m, label: t(`adminOps.alertRules.metricType.${m}`) })),
    },
    {
      name: "operator",
      label: t("adminOps.alertRules.operator"),
      type: "select",
      options: OPERATORS.map((o) => ({ value: o, label: t(`adminOps.alertRules.operators.${o}`) })),
    },
    {
      name: "threshold",
      label: t("adminOps.alertRules.threshold"),
      type: "number",
      hint: t("adminOps.alertRules.thresholdHint"),
    },
    {
      name: "severity",
      label: t("adminOps.alertRules.severity"),
      type: "select",
      options: enumOptions(SEVERITIES),
    },
    { name: "windowSeconds", label: t("adminOps.alertRules.window"), type: "number" },
    { name: "cooldownSeconds", label: t("adminOps.alertRules.cooldown"), type: "number" },
    { name: "minRequestCount", label: t("adminOps.alertRules.minRequests"), type: "number" },
    {
      name: "sourceEndpoint",
      label: t("adminOps.alertRules.endpoint"),
      placeholder: "/v1/chat/completions",
    },
    { name: "model", label: t("adminOps.alertRules.model") },
    {
      name: "errorClass",
      label: t("adminOps.alertRules.errorClass"),
      placeholder: "provider_5xx",
    },
    { name: "providerId", label: t("adminOps.alertRules.provider") },
    { name: "enabled", label: t("adminOps.alertRules.enabled"), type: "switch" },
  ];

  return (
    <ResourceFormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t(mode === "create" ? "adminOps.alertRules.create" : "adminOps.alertRules.edit")}
      description={target ? target.name : undefined}
      fields={fields}
      initial={target ? formFromRule(target) : emptyForm()}
      buildBody={buildBody}
      submit={
        mode === "edit" && target
          ? (body) => updateMut.mutateAsync({ id: target.id, body })
          : (body) => createMut.mutateAsync(body)
      }
      successMessage={t(mode === "create" ? "feedback.created" : "feedback.updated")}
      isPending={createMut.isPending || updateMut.isPending}
    />
  );
}
