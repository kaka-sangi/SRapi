"use client";

import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageQueryState } from "@/components/layout/page-query-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import {
  useUserAttributeValues,
  useSetUserAttributeValue,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import type { UserAttributeValue } from "@/lib/sdk-types";

// Attribute admin had values storage + an endpoint for years (PUT
// /admin/users/{id}/attributes/{definitionId}) but no UI surface — the
// definitions page only configured the schema, never assigned values.
// This dialog plugs the per-user save flow into the existing row actions.
//
// Field controls map by data_type:
//   boolean → Switch
//   options → Select (the definition's `options[]` are the allowed values)
//   number  → numeric Input (still serialized as string per the API)
//   anything else → plain text Input
export function UserAttributeValuesDialog({
  userId,
  userLabel,
  onClose,
}: {
  userId: string;
  userLabel: string;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const query = useUserAttributeValues(userId);
  const setMut = useSetUserAttributeValue(userId);

  // Edits are tracked per definition_id so the admin sees pending changes
  // before saving them.
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const busy = setMut.isPending;

  // Seed drafts from the latest fetch — the eslint-disable matches the in-effect
  // server-data-seed pattern used elsewhere in the codebase (see stat-card).
  useEffect(() => {
    if (!query.data) return;
    const next: Record<string, string> = {};
    for (const row of query.data.data) {
      next[String(row.definition_id)] = row.value ?? "";
    }
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDrafts(next);
  }, [query.data]);

  function setDraft(definitionId: string, value: string) {
    setDrafts((prev) => ({ ...prev, [definitionId]: value }));
  }

  async function saveOne(row: UserAttributeValue) {
    setError(null);
    const next = drafts[String(row.definition_id)] ?? "";
    if (row.required && next.trim() === "") {
      setError(t("adminUserAttributeValues.required", { name: row.name }));
      return;
    }
    try {
      await setMut.mutateAsync({
        definitionId: String(row.definition_id),
        body: { value: next },
      });
      toast({ title: t("feedback.updated"), tone: "success" });
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("adminUserAttributeValues.title")}</DialogTitle>
          <DialogDescription>
            {t("adminUserAttributeValues.subtitle", { user: userLabel })}
          </DialogDescription>
        </DialogHeader>
        <div className="mt-2 min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain pr-1">
          <PageQueryState query={query} skeleton={<DialogListSkeleton rows={3} />}>
            {(list) =>
              list.data.length === 0 ? (
                <p className="text-sm text-srapi-text-tertiary">
                  {t("adminUserAttributeValues.empty")}
                </p>
              ) : (
                <div className="space-y-2.5">
                  {list.data.map((row) => {
                    const id = String(row.definition_id);
                    const current = drafts[id] ?? row.value ?? "";
                    const isDirty = current !== (row.value ?? "");
                    return (
                      <div
                        key={id}
                        className="rounded-lg border border-srapi-border p-3"
                      >
                        <div className="mb-2 flex items-baseline gap-2">
                          <Label
                            htmlFor={`uav-${id}`}
                            className="mb-0 font-mono text-2xs uppercase text-srapi-text-tertiary"
                          >
                            {row.name}
                          </Label>
                          <span className="font-mono text-2xs text-srapi-text-tertiary">
                            {row.key}
                          </span>
                          {row.required ? (
                            <span className="font-mono text-2xs text-srapi-error">*</span>
                          ) : null}
                        </div>
                        <div className="flex items-center gap-2">
                          <AttributeInput
                            row={row}
                            id={`uav-${id}`}
                            value={current}
                            disabled={busy}
                            onChange={(v) => setDraft(id, v)}
                          />
                          <Button
                            type="button"
                            size="sm"
                            variant={isDirty ? "primary" : "ghost"}
                            disabled={busy || !isDirty}
                            loading={setMut.isPending}
                            onClick={() => void saveOne(row)}
                          >
                            {t("common.save")}
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )
            }
          </PageQueryState>
          {error ? (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function AttributeInput({
  row,
  id,
  value,
  disabled,
  onChange,
}: {
  row: UserAttributeValue;
  id: string;
  value: string;
  disabled: boolean;
  onChange: (v: string) => void;
}) {
  if (row.data_type === "boolean") {
    return (
      <Switch
        id={id}
        checked={value === "true"}
        disabled={disabled}
        onCheckedChange={(checked) => onChange(checked ? "true" : "false")}
      />
    );
  }
  if (row.options && row.options.length > 0) {
    return (
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue placeholder="—" />
        </SelectTrigger>
        <SelectContent>
          {row.options.map((opt) => (
            <SelectItem key={opt} value={opt}>
              {opt}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }
  return (
    <Input
      id={id}
      value={value}
      inputMode={row.data_type === "number" ? "decimal" : undefined}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}
