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
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
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
import { useCreateOpsSlo, useUpdateOpsSlo } from "@/hooks/admin-queries";
import {
  OPS_SLI_TYPES,
  OPS_SLO_STATUSES,
  OPS_ERROR_OWNERS,
  emptyOpsSloForm,
  opsSloFormFromDefinition,
  buildCreateOpsSloBody,
  buildUpdateOpsSloBody,
  toggleErrorOwner,
  type OpsSloFormState,
} from "@/lib/admin-ops-slo-form";
import type { OpsSloDefinition } from "@/lib/sdk-types";

/** Create / edit an Ops SLO definition (multi-window burn-rate alerting). */
export function SloFormDialog({
  open,
  onOpenChange,
  target,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  target: OpsSloDefinition | null;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const createMut = useCreateOpsSlo();
  const updateMut = useUpdateOpsSlo();
  const mode = target ? "edit" : "create";

  const [form, setForm] = useState<OpsSloFormState>(() =>
    target ? opsSloFormFromDefinition(target) : emptyOpsSloForm(),
  );
  const [error, setError] = useState<string | null>(null);
  const busy = createMut.isPending || updateMut.isPending;

  function set<K extends keyof OpsSloFormState>(key: K, value: OpsSloFormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    try {
      if (mode === "edit" && target) {
        await updateMut.mutateAsync({ id: target.id, body: buildUpdateOpsSloBody(form) });
      } else {
        await createMut.mutateAsync(buildCreateOpsSloBody(form));
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
            <DialogTitle>{t(mode === "create" ? "adminOps.createSlo" : "adminOps.editSlo")}</DialogTitle>
            {target ? <DialogDescription>{target.name}</DialogDescription> : null}
          </DialogHeader>

          <div className="mt-4 max-h-[62vh] space-y-4 overflow-y-auto pr-1">
            <div>
              <Label htmlFor="slo-name">{t("adminCommon.name")}</Label>
              <Input id="slo-name" value={form.name} disabled={busy || mode === "edit"} onChange={(e) => set("name", e.target.value)} />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="slo-sli">{t("adminOps.sliType")}</Label>
                <Select value={form.sliType} onValueChange={(v) => set("sliType", v as OpsSloFormState["sliType"])} disabled={busy || mode === "edit"}>
                  <SelectTrigger id="slo-sli">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {OPS_SLI_TYPES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="slo-status">{t("adminCommon.status")}</Label>
                <Select value={form.status} onValueChange={(v) => set("status", v as OpsSloFormState["status"])} disabled={busy}>
                  <SelectTrigger id="slo-status">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {OPS_SLO_STATUSES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="slo-objective">{t("adminOps.target")}</Label>
                <Input id="slo-objective" type="number" step="0.01" value={form.objective} disabled={busy} onChange={(e) => set("objective", e.target.value)} />
              </div>
              <div>
                <Label htmlFor="slo-window">{t("adminOps.windowDays")}</Label>
                <Input id="slo-window" type="number" value={form.windowDays} disabled={busy} onChange={(e) => set("windowDays", e.target.value)} />
              </div>
            </div>

            <div>
              <Label htmlFor="slo-endpoint">{t("usage.endpoint")}</Label>
              <Input id="slo-endpoint" value={form.sourceEndpoint} placeholder="/v1/chat/completions" disabled={busy} onChange={(e) => set("sourceEndpoint", e.target.value)} />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="slo-model">{t("adminOps.model")}</Label>
                <Input id="slo-model" value={form.model} disabled={busy} onChange={(e) => set("model", e.target.value)} />
              </div>
              <div>
                <Label htmlFor="slo-provider">{t("adminAccounts.provider")}</Label>
                <Input id="slo-provider" value={form.providerId} disabled={busy} onChange={(e) => set("providerId", e.target.value)} />
              </div>
            </div>

            <div>
              <Label>{t("adminRisk.severity")}</Label>
              <div className="mt-1 flex flex-wrap gap-3">
                {OPS_ERROR_OWNERS.map((owner) => (
                  <label key={owner} className="flex items-center gap-1.5 text-2xs text-srapi-text-secondary">
                    <Checkbox
                      checked={form.errorOwnerExclude.includes(owner)}
                      disabled={busy}
                      onChange={(e) => set("errorOwnerExclude", toggleErrorOwner(form.errorOwnerExclude, owner, e.target.checked))}
                    />
                    {owner}
                  </label>
                ))}
              </div>
            </div>

            <div>
              <Label htmlFor="slo-thresholds">{t("adminOps.alerts")} (JSON)</Label>
              <Textarea
                id="slo-thresholds"
                spellCheck={false}
                className="min-h-32 font-mono text-xs"
                value={form.thresholdsJson}
                disabled={busy}
                onChange={(e) => set("thresholdsJson", e.target.value)}
              />
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
