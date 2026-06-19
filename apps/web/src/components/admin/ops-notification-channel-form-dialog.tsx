"use client";

import { useLanguage } from "@/context/LanguageContext";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useCreateOpsNotificationChannel,
  useUpdateOpsNotificationChannel,
} from "@/hooks/admin-queries";
import type {
  OpsAlertSeverity,
  OpsNotificationChannel,
  OpsNotificationChannelStatus,
} from "@/lib/sdk-types";

const CHANNEL_STATUSES: OpsNotificationChannelStatus[] = ["active", "disabled"];
const SEVERITIES: OpsAlertSeverity[] = ["ticket", "warning", "critical"];

type FormState = {
  name: string;
  status: OpsNotificationChannelStatus;
  minSeverity: OpsAlertSeverity;
  emailRecipients: string;
  sendResolved: boolean;
};

function emptyForm(): FormState {
  return {
    name: "",
    status: "active",
    minSeverity: "warning",
    emailRecipients: "",
    sendResolved: true,
  };
}

function formFromChannel(channel: OpsNotificationChannel): FormState {
  return {
    name: channel.name,
    status: channel.status,
    minSeverity: channel.min_severity,
    emailRecipients: channel.email_recipients.join("\n"),
    sendResolved: channel.send_resolved,
  };
}

function parseRecipients(value: string): string[] {
  return value
    .split(/[\n,;]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function buildBody(form: FormState) {
  return {
    name: form.name.trim(),
    type: "email" as const,
    status: form.status,
    min_severity: form.minSeverity,
    email_recipients: parseRecipients(form.emailRecipients),
    send_resolved: form.sendResolved,
  };
}

export function OpsNotificationChannelFormDialog({
  open,
  onOpenChange,
  target,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  target: OpsNotificationChannel | null;
}) {
  const { t } = useLanguage();
  const createMut = useCreateOpsNotificationChannel();
  const updateMut = useUpdateOpsNotificationChannel();
  const mode = target ? "edit" : "create";

  const fields: FieldConfig<FormState>[] = [
    { name: "name", label: t("adminOps.notificationChannels.name"), required: true },
    {
      name: "status",
      label: t("adminOps.notificationChannels.status"),
      type: "select",
      options: CHANNEL_STATUSES.map((status) => ({
        value: status,
        label:
          status === "active" ? t("adminOps.notificationChannels.active") : t("common.disabled"),
      })),
    },
    {
      name: "minSeverity",
      label: t("adminOps.notificationChannels.minSeverity"),
      type: "select",
      options: enumOptions(SEVERITIES),
    },
    {
      name: "emailRecipients",
      label: t("adminOps.notificationChannels.recipients"),
      type: "textarea",
      required: true,
      hint: t("adminOps.notificationChannels.recipientsHint"),
      validate: (value) =>
        parseRecipients(String(value ?? "")).length === 0
          ? t("adminOps.notificationChannels.recipientsRequired")
          : undefined,
    },
    {
      name: "sendResolved",
      label: t("adminOps.notificationChannels.sendResolved"),
      type: "switch",
    },
  ];

  return (
    <ResourceFormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t(
        mode === "create"
          ? "adminOps.notificationChannels.create"
          : "adminOps.notificationChannels.edit",
      )}
      description={target ? target.name : undefined}
      fields={fields}
      initial={target ? formFromChannel(target) : emptyForm()}
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
