"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import { useCreateOpsAlertSilence } from "@/hooks/admin-queries";
import type { OpsAlertSeverity } from "@/lib/sdk-types";

const SEVERITIES: OpsAlertSeverity[] = ["warning", "critical", "ticket"];

function defaultEnd(): string {
  const end = new Date(Date.now() + 24 * 60 * 60 * 1000);
  return end.toISOString().slice(0, 16);
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

  const [comment, setComment] = useState("");
  const [ruleId, setRuleId] = useState("");
  const [severity, setSeverity] = useState<"" | OpsAlertSeverity>("");
  const [sourceEndpoint, setSourceEndpoint] = useState("");
  const [model, setModel] = useState("");
  const [providerId, setProviderId] = useState("");
  const [startsAt, setStartsAt] = useState("");
  const [endsAt, setEndsAt] = useState(defaultEnd());
  const [error, setError] = useState<string | null>(null);
  const busy = createMut.isPending;

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const matcher: {
      rule_id?: string;
      severity?: OpsAlertSeverity;
      source_endpoint?: string;
      model?: string;
      provider_id?: string;
    } = {};
    if (ruleId.trim()) matcher.rule_id = ruleId.trim();
    if (severity) matcher.severity = severity;
    if (sourceEndpoint.trim()) matcher.source_endpoint = sourceEndpoint.trim();
    if (model.trim()) matcher.model = model.trim();
    if (providerId.trim()) matcher.provider_id = providerId.trim();
    try {
      await createMut.mutateAsync({
        comment: comment.trim() || undefined,
        matcher,
        starts_at: startsAt ? new Date(startsAt).toISOString() : undefined,
        ends_at: new Date(endsAt).toISOString(),
      });
      toast({ title: t("feedback.created"), tone: "success" });
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
            <DialogTitle>{t("adminOps.silences.create")}</DialogTitle>
          </DialogHeader>

          <div className="mt-4 max-h-[62vh] space-y-4 overflow-y-auto pr-1">
            <div>
              <Label htmlFor="silence-comment">{t("adminOps.silences.comment")}</Label>
              <Input id="silence-comment" value={comment} disabled={busy} onChange={(e) => setComment(e.target.value)} />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="silence-rule">{t("adminOps.silences.ruleId")}</Label>
                <Input id="silence-rule" value={ruleId} placeholder="rule.1" disabled={busy} onChange={(e) => setRuleId(e.target.value)} />
              </div>
              <div>
                <Label htmlFor="silence-severity">{t("adminOps.silences.severity")}</Label>
                <Select value={severity || "any"} onValueChange={(v) => setSeverity(v === "any" ? "" : (v as OpsAlertSeverity))} disabled={busy}>
                  <SelectTrigger id="silence-severity">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="any">{t("adminOps.silences.anyMatcher")}</SelectItem>
                    {SEVERITIES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div>
              <Label htmlFor="silence-endpoint">{t("adminOps.silences.endpoint")}</Label>
              <Input id="silence-endpoint" value={sourceEndpoint} placeholder="/v1/chat/completions" disabled={busy} onChange={(e) => setSourceEndpoint(e.target.value)} />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="silence-model">{t("adminOps.silences.model")}</Label>
                <Input id="silence-model" value={model} disabled={busy} onChange={(e) => setModel(e.target.value)} />
              </div>
              <div>
                <Label htmlFor="silence-provider">{t("adminOps.silences.provider")}</Label>
                <Input id="silence-provider" value={providerId} disabled={busy} onChange={(e) => setProviderId(e.target.value)} />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="silence-start">{t("adminOps.silences.startsAt")}</Label>
                <Input id="silence-start" type="datetime-local" value={startsAt} disabled={busy} onChange={(e) => setStartsAt(e.target.value)} />
              </div>
              <div>
                <Label htmlFor="silence-end">{t("adminOps.silences.endsAt")}</Label>
                <Input id="silence-end" type="datetime-local" value={endsAt} disabled={busy} onChange={(e) => setEndsAt(e.target.value)} />
              </div>
            </div>

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
