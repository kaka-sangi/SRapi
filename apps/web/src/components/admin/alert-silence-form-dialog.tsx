"use client";

import { useLanguage } from "@/context/LanguageContext";
import {
  ResourceFormDialog,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { useCreateOpsAlertSilence } from "@/hooks/admin-queries";
import type { OpsAlertSeverity } from "@/lib/sdk-types";

const SEVERITIES: OpsAlertSeverity[] = ["warning", "critical", "ticket"];
const ANY_SEVERITY = "any";

function defaultEnd(): string {
  const end = new Date(Date.now() + 24 * 60 * 60 * 1000);
  return end.toISOString().slice(0, 16);
}

type FormState = {
  comment: string;
  ruleId: string;
  severity: typeof ANY_SEVERITY | OpsAlertSeverity;
  sourceEndpoint: string;
  model: string;
  errorClass: string;
  providerId: string;
  startsAt: string;
  endsAt: string;
};

function emptyForm(): FormState {
  return {
    comment: "",
    ruleId: "",
    severity: ANY_SEVERITY,
    sourceEndpoint: "",
    model: "",
    errorClass: "",
    providerId: "",
    startsAt: "",
    endsAt: defaultEnd(),
  };
}

function buildBody(form: FormState) {
  const matcher: {
    rule_id?: string;
    severity?: OpsAlertSeverity;
    source_endpoint?: string;
    model?: string;
    error_class?: string;
    provider_id?: string;
  } = {};
  if (form.ruleId.trim()) matcher.rule_id = form.ruleId.trim();
  if (form.severity !== ANY_SEVERITY) matcher.severity = form.severity;
  if (form.sourceEndpoint.trim()) matcher.source_endpoint = form.sourceEndpoint.trim();
  if (form.model.trim()) matcher.model = form.model.trim();
  if (form.errorClass.trim()) matcher.error_class = form.errorClass.trim();
  if (form.providerId.trim()) matcher.provider_id = form.providerId.trim();
  return {
    comment: form.comment.trim() || undefined,
    matcher,
    starts_at: form.startsAt ? new Date(form.startsAt).toISOString() : undefined,
    ends_at: new Date(form.endsAt).toISOString(),
  };
}

/** Create an alert silence window that suppresses matching alert events. */
export function AlertSilenceFormDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const createMut = useCreateOpsAlertSilence();

  const fields: FieldConfig<FormState>[] = [
    { name: "comment", label: t("adminOps.silences.comment") },
    { name: "ruleId", label: t("adminOps.silences.ruleId"), placeholder: "rule.1" },
    {
      name: "severity",
      label: t("adminOps.silences.severity"),
      type: "select",
      options: [
        { value: ANY_SEVERITY, label: t("adminOps.silences.anyMatcher") },
        ...SEVERITIES.map((s) => ({ value: s, label: s })),
      ],
    },
    {
      name: "sourceEndpoint",
      label: t("adminOps.silences.endpoint"),
      placeholder: "/v1/chat/completions",
    },
    { name: "model", label: t("adminOps.silences.model") },
    { name: "errorClass", label: t("adminOps.silences.errorClass"), placeholder: "provider_5xx" },
    { name: "providerId", label: t("adminOps.silences.provider") },
    { name: "startsAt", label: t("adminOps.silences.startsAt"), type: "datetime" },
    { name: "endsAt", label: t("adminOps.silences.endsAt"), type: "datetime" },
  ];

  return (
    <ResourceFormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("adminOps.silences.create")}
      fields={fields}
      initial={emptyForm()}
      buildBody={buildBody}
      submit={(body) => createMut.mutateAsync(body)}
      successMessage={t("feedback.created")}
      isPending={createMut.isPending}
    />
  );
}
