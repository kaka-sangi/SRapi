"use client";

import { useMemo, useState } from "react";
import { Mail, Send } from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { FloatingInput } from "@/components/ui/floating-input";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
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

function severityTone(s: OpsAlertSeverity): "warning" | "error" | "accent" {
  if (s === "critical") return "error";
  if (s === "warning") return "warning";
  return "accent";
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
  const { toast } = useToast();
  const createMut = useCreateOpsNotificationChannel();
  const updateMut = useUpdateOpsNotificationChannel();
  const mode = target ? "edit" : "create";

  const [form, setForm] = useState<FormState>(() =>
    target ? formFromChannel(target) : emptyForm(),
  );
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const recipients = useMemo(() => parseRecipients(form.emailRecipients), [form.emailRecipients]);
  const nameError = form.name.trim().length === 0 ? t("adminCommon.required") : undefined;
  const recipientsError =
    recipients.length === 0 ? t("adminOps.notificationChannels.recipientsRequired") : undefined;
  const hasError = Boolean(nameError || recipientsError);

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
  const tone = severityTone(form.minSeverity);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t(
              mode === "create"
                ? "adminOps.notificationChannels.create"
                : "adminOps.notificationChannels.edit",
            )}
          </DialogTitle>
          <DialogDescription className="sr-only">
            {t("adminOps.notificationChannels.subtitle")}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} noValidate className="space-y-5">
          {/* Severity-aware preview */}
          <div className="flex items-start gap-3 rounded-2xl border border-srapi-border bg-srapi-card-muted/60 p-4">
            <IconBubble tone={tone} size="md">
              <Mail />
            </IconBubble>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-1.5">
                <DataPill tone={tone} size="sm">
                  ≥ {form.minSeverity}
                </DataPill>
                <DataPill tone={form.status === "active" ? "success" : "neutral"} size="sm">
                  {form.status === "active"
                    ? t("adminOps.notificationChannels.active")
                    : t("common.disabled")}
                </DataPill>
                <DataPill tone="neutral" size="sm">
                  <Send className="size-3" />
                  {form.sendResolved
                    ? t("adminOps.notificationChannels.sendsResolved")
                    : t("adminOps.notificationChannels.firingOnly")}
                </DataPill>
              </div>
              <p className="mt-1.5 break-words text-sm font-semibold tracking-tight text-srapi-text-primary">
                {form.name.trim() || t("adminOps.notificationChannels.name")}
              </p>
              <p className="mt-1 text-xs text-srapi-text-tertiary">
                <span className="metric-secondary tabular">{recipients.length}</span>{" "}
                {t("adminOps.notificationChannels.recipients").toLowerCase()}
              </p>
            </div>
          </div>

          {/* Name */}
          <FloatingInput
            label={t("adminOps.notificationChannels.name")}
            value={form.name}
            onChange={(v) => update("name", v)}
            error={nameError && form.name !== "" ? nameError : undefined}
            required
            disabled={busy}
          />

          {/* Status */}
          <div>
            <Label className="mb-1.5 block">{t("adminOps.notificationChannels.status")}</Label>
            <SegmentedControl<OpsNotificationChannelStatus>
              value={form.status}
              onChange={(value) => update("status", value)}
              options={CHANNEL_STATUSES.map((s) => ({
                value: s,
                label: s === "active"
                  ? t("adminOps.notificationChannels.active")
                  : t("common.disabled"),
              }))}
              ariaLabel={t("adminOps.notificationChannels.status")}
              size="sm"
            />
          </div>

          {/* Min severity */}
          <div>
            <Label className="mb-1.5 block">{t("adminOps.notificationChannels.minSeverity")}</Label>
            <SegmentedControl<OpsAlertSeverity>
              value={form.minSeverity}
              onChange={(value) => update("minSeverity", value)}
              options={SEVERITIES.map((s) => ({ value: s, label: s }))}
              ariaLabel={t("adminOps.notificationChannels.minSeverity")}
              size="sm"
            />
          </div>

          {/* Recipients */}
          <div>
            <div className="mb-1.5 flex items-center justify-between gap-2">
              <Label htmlFor="nc-recipients" className="mb-0">
                {t("adminOps.notificationChannels.recipients")}
              </Label>
              <DataPill tone={recipients.length > 0 ? "success" : "neutral"} size="sm">
                <span className="tabular">{recipients.length}</span>
              </DataPill>
            </div>
            <Textarea
              id="nc-recipients"
              value={form.emailRecipients}
              disabled={busy}
              aria-invalid={recipientsError && form.emailRecipients !== "" ? true : undefined}
              placeholder="ops@example.com&#10;oncall@example.com"
              className="min-h-24 font-mono text-xs"
              onChange={(e) => update("emailRecipients", e.target.value)}
            />
            <p className="mt-1 text-[11px] text-srapi-text-tertiary">
              {t("adminOps.notificationChannels.recipientsHint")}
            </p>
            {recipientsError && form.emailRecipients !== "" ? (
              <p role="alert" className="mt-1 text-[11px] text-srapi-error">
                {recipientsError}
              </p>
            ) : null}
          </div>

          {/* Send resolved switch */}
          <div className="flex items-center justify-between gap-4 rounded-xl border border-srapi-border bg-srapi-card px-4 py-3">
            <div className="min-w-0">
              <Label htmlFor="nc-resolved" className="mb-0">
                {t("adminOps.notificationChannels.sendResolved")}
              </Label>
            </div>
            <Switch
              id="nc-resolved"
              checked={form.sendResolved}
              disabled={busy}
              onCheckedChange={(checked) => update("sendResolved", checked)}
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
