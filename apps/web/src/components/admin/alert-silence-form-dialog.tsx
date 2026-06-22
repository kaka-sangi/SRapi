"use client";

import { useMemo, useState } from "react";
import { BellOff, Clock } from "lucide-react";
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
import { Input } from "@/components/ui/input";
import { FloatingInput } from "@/components/ui/floating-input";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { SectionTitle } from "@/components/ui/section-title";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { useCreateOpsAlertSilence } from "@/hooks/admin-queries";
import type { OpsAlertSeverity } from "@/lib/sdk-types";

const SEVERITIES: OpsAlertSeverity[] = ["warning", "critical", "ticket"];
const ANY_SEVERITY = "any";
type SeverityChoice = typeof ANY_SEVERITY | OpsAlertSeverity;

function defaultEnd(): string {
  const end = new Date(Date.now() + 24 * 60 * 60 * 1000);
  return end.toISOString().slice(0, 16);
}

type FormState = {
  comment: string;
  ruleId: string;
  severity: SeverityChoice;
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

function severityTone(s: SeverityChoice): "warning" | "error" | "accent" | "neutral" {
  if (s === "critical") return "error";
  if (s === "warning") return "warning";
  if (s === "ticket") return "accent";
  return "neutral";
}

function formatDuration(startsAt: string, endsAt: string): string | null {
  if (!endsAt) return null;
  const start = startsAt ? new Date(startsAt) : new Date();
  const end = new Date(endsAt);
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return null;
  const ms = end.getTime() - start.getTime();
  if (ms <= 0) return null;
  const hours = ms / (1000 * 60 * 60);
  if (hours >= 24) return `${(hours / 24).toFixed(hours >= 72 ? 0 : 1)}d`;
  if (hours >= 1) return `${hours.toFixed(hours >= 10 ? 0 : 1)}h`;
  return `${Math.round(ms / (1000 * 60))}m`;
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
  const { toast } = useToast();
  const createMut = useCreateOpsAlertSilence();

  const [form, setForm] = useState<FormState>(() => emptyForm());
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const endsError =
    !form.endsAt || Number.isNaN(new Date(form.endsAt).getTime())
      ? t("adminCommon.required")
      : undefined;
  const hasError = Boolean(endsError);

  const matcherChips = useMemo(() => {
    const chips: { label: string; value: string }[] = [];
    if (form.ruleId.trim()) chips.push({ label: t("adminOps.silences.ruleId"), value: form.ruleId.trim() });
    if (form.severity !== ANY_SEVERITY) chips.push({ label: t("adminOps.silences.severity"), value: form.severity });
    if (form.sourceEndpoint.trim()) chips.push({ label: t("adminOps.silences.endpoint"), value: form.sourceEndpoint.trim() });
    if (form.model.trim()) chips.push({ label: t("adminOps.silences.model"), value: form.model.trim() });
    if (form.errorClass.trim()) chips.push({ label: t("adminOps.silences.errorClass"), value: form.errorClass.trim() });
    if (form.providerId.trim()) chips.push({ label: t("adminOps.silences.provider"), value: form.providerId.trim() });
    return chips;
  }, [form.ruleId, form.severity, form.sourceEndpoint, form.model, form.errorClass, form.providerId, t]);

  const duration = formatDuration(form.startsAt, form.endsAt);

  function update<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    if (hasError) return;
    setError(null);
    setSubmitting(true);
    try {
      await createMut.mutateAsync(buildBody(form));
      toast({ title: t("feedback.created"), tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setSubmitting(false);
    }
  }

  const busy = submitting || createMut.isPending;
  const tone = severityTone(form.severity);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t("adminOps.silences.create")}
          </DialogTitle>
          <DialogDescription className="sr-only">
            {t("adminOps.silences.subtitle")}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} noValidate className="space-y-5">
          {/* Severity-aware preview */}
          <div className="flex items-start gap-3 rounded-2xl border border-srapi-border bg-srapi-card-muted/60 p-4">
            <IconBubble tone={tone} size="md">
              <BellOff />
            </IconBubble>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-1.5">
                <DataPill tone={tone} size="sm">
                  {form.severity === ANY_SEVERITY ? t("adminOps.silences.anyMatcher") : form.severity}
                </DataPill>
                {duration ? (
                  <DataPill tone="neutral" size="sm">
                    <Clock className="size-3" />
                    <span className="tabular">{duration}</span>
                  </DataPill>
                ) : null}
              </div>
              <p className="mt-1.5 break-words text-sm font-semibold tracking-tight text-srapi-text-primary">
                {form.comment.trim() || t("adminOps.silences.comment")}
              </p>
              {matcherChips.length > 0 ? (
                <div className="mt-2 flex flex-wrap gap-1">
                  {matcherChips.map((chip) => (
                    <DataPill key={chip.label} tone="neutral" size="sm" className="font-mono">
                      <span className="text-srapi-text-tertiary">{chip.label}:</span>
                      <span className="ml-1 text-srapi-text-primary">{chip.value}</span>
                    </DataPill>
                  ))}
                </div>
              ) : (
                <p className="mt-2 text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.anyMatcher")}
                </p>
              )}
            </div>
          </div>

          {/* Severity SegmentedControl */}
          <div>
            <Label className="mb-1.5 block">{t("adminOps.silences.severity")}</Label>
            <SegmentedControl<SeverityChoice>
              value={form.severity}
              onChange={(value) => update("severity", value)}
              options={[
                { value: ANY_SEVERITY, label: t("adminOps.silences.anyMatcher") },
                ...SEVERITIES.map((s) => ({ value: s as SeverityChoice, label: s })),
              ]}
              ariaLabel={t("adminOps.silences.severity")}
              size="sm"
            />
          </div>

          {/* Comment */}
          <FloatingInput
            label={t("adminOps.silences.comment")}
            value={form.comment}
            onChange={(v) => update("comment", v)}
            disabled={busy}
          />

          {/* Time window */}
          <div className="space-y-3 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
            <SectionTitle
              label={t("adminOps.silences.window")}
              action={
                duration ? (
                  <DataPill tone="neutral" size="sm">
                    <Clock className="size-3" />
                    <span className="tabular">{duration}</span>
                  </DataPill>
                ) : null
              }
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="as-start" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.startsAt")}
                </Label>
                <Input
                  id="as-start"
                  type="datetime-local"
                  value={form.startsAt}
                  disabled={busy}
                  onChange={(e) => update("startsAt", e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="as-end" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.endsAt")}
                </Label>
                <Input
                  id="as-end"
                  type="datetime-local"
                  value={form.endsAt}
                  disabled={busy}
                  aria-invalid={endsError ? true : undefined}
                  onChange={(e) => update("endsAt", e.target.value)}
                />
              </div>
            </div>
          </div>

          {/* Matcher section */}
          <div className="space-y-3 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
            <SectionTitle
              label={t("adminOps.silences.matcher")}
              action={
                matcherChips.length === 0 ? (
                  <span className="text-[11px] text-srapi-text-tertiary">
                    {t("adminOps.silences.anyMatcher")}
                  </span>
                ) : null
              }
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="as-rule" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.ruleId")}
                </Label>
                <Input
                  id="as-rule"
                  value={form.ruleId}
                  placeholder="rule.1"
                  disabled={busy}
                  onChange={(e) => update("ruleId", e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="as-ep" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.endpoint")}
                </Label>
                <Input
                  id="as-ep"
                  value={form.sourceEndpoint}
                  placeholder="/v1/chat/completions"
                  disabled={busy}
                  onChange={(e) => update("sourceEndpoint", e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="as-model" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.model")}
                </Label>
                <Input
                  id="as-model"
                  value={form.model}
                  disabled={busy}
                  onChange={(e) => update("model", e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="as-ec" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.errorClass")}
                </Label>
                <Input
                  id="as-ec"
                  value={form.errorClass}
                  placeholder="provider_5xx"
                  disabled={busy}
                  onChange={(e) => update("errorClass", e.target.value)}
                />
              </div>
              <div className="sm:col-span-2">
                <Label htmlFor="as-prov" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                  {t("adminOps.silences.provider")}
                </Label>
                <Input
                  id="as-prov"
                  value={form.providerId}
                  disabled={busy}
                  onChange={(e) => update("providerId", e.target.value)}
                />
              </div>
            </div>
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
