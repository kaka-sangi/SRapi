"use client";

import { useState } from "react";
import { Copy, Check } from "lucide-react";
import {
  createApiKeySchema,
  updateApiKeySchema,
  type CreateApiKeyValues,
} from "@/lib/schemas/api-key";
import { useCreateApiKey, useUpdateApiKey } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TagInput } from "@/components/ui/tag-input";
import type { ApiKeySummary } from "@/lib/srapi-types";

/** Create entry point used by the page header. */
export function ApiKeyCreateDialog() {
  return <ApiKeyFormDialog />;
}

function limitToInput(value?: number | null): string {
  return value && value > 0 ? String(value) : "";
}

// ISO timestamp → value accepted by <input type="datetime-local"> (local time,
// minute precision), so the current expiry prefills correctly.
function isoToLocalInput(iso?: string | null): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours(),
  )}:${pad(date.getMinutes())}`;
}

export function ApiKeyFormDialog({
  editKey,
  open: controlledOpen,
  onOpenChange,
}: {
  editKey?: ApiKeySummary;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const isEdit = Boolean(editKey);
  const controlled = onOpenChange !== undefined;

  const [internalOpen, setInternalOpen] = useState(false);
  const open = controlled ? Boolean(controlledOpen) : internalOpen;

  // Initial state derives from the edited key (lazy initializers). The page
  // remounts this dialog via `key` when a different key is selected, so a fresh
  // mount always reflects the right key without a state-syncing effect.
  const [plaintext, setPlaintext] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [name, setName] = useState(() => editKey?.name ?? "");
  const [allowedModels, setAllowedModels] = useState<string[]>(() => editKey?.allowed_models ?? []);
  const [groupIds, setGroupIds] = useState<string[]>(() => editKey?.group_ids ?? []);
  const [allowedIps, setAllowedIps] = useState<string[]>(() => editKey?.allowed_ips ?? []);
  const [deniedIps, setDeniedIps] = useState<string[]>(() => editKey?.denied_ips ?? []);
  const [limit5h, setLimit5h] = useState(() => limitToInput(editKey?.request_limit_5h));
  const [limit1d, setLimit1d] = useState(() => limitToInput(editKey?.request_limit_1d));
  const [limit7d, setLimit7d] = useState(() => limitToInput(editKey?.request_limit_7d));
  const [rpmLimit, setRpmLimit] = useState(() => limitToInput(editKey?.rpm_limit));
  const [tpmLimit, setTpmLimit] = useState(() => limitToInput(editKey?.tpm_limit));
  const [concurrencyLimit, setConcurrencyLimit] = useState(() =>
    limitToInput(editKey?.concurrency_limit),
  );
  const [expiresAt, setExpiresAt] = useState(() => isoToLocalInput(editKey?.expires_at));
  const [error, setError] = useState<string | null>(null);
  const createKey = useCreateApiKey();
  const updateKey = useUpdateApiKey();
  const pending = isEdit ? updateKey.isPending : createKey.isPending;

  function parseLimit(value: string): number | undefined {
    const trimmed = value.trim();
    if (trimmed === "") return undefined;
    return Number(trimmed);
  }

  function isoOrUndefined(value?: string): string | undefined {
    if (!value) return undefined;
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const values = {
      name,
      allowedModels,
      groupIds,
      allowedIps,
      deniedIps,
      requestLimit5h: parseLimit(limit5h),
      requestLimit1d: parseLimit(limit1d),
      requestLimit7d: parseLimit(limit7d),
      rpmLimit: parseLimit(rpmLimit),
      tpmLimit: parseLimit(tpmLimit),
      concurrencyLimit: parseLimit(concurrencyLimit),
      expiresAt: expiresAt.trim() || undefined,
    } satisfies CreateApiKeyValues;
    const parsed = (isEdit ? updateApiKeySchema : createApiKeySchema).safeParse(values);
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? "Invalid input");
      return;
    }
    const payload = {
      name: parsed.data.name,
      allowedModels: parsed.data.allowedModels,
      groupIds: parsed.data.groupIds,
      allowedIps: parsed.data.allowedIps,
      deniedIps: parsed.data.deniedIps,
      requestLimit5h: parsed.data.requestLimit5h,
      requestLimit1d: parsed.data.requestLimit1d,
      requestLimit7d: parsed.data.requestLimit7d,
      rpmLimit: parsed.data.rpmLimit,
      tpmLimit: parsed.data.tpmLimit,
      concurrencyLimit: parsed.data.concurrencyLimit,
      expiresAt: isoOrUndefined(parsed.data.expiresAt),
    };
    try {
      if (isEdit && editKey) {
        await updateKey.mutateAsync({ id: editKey.id, ...payload });
        toast({ title: t("feedback.saved"), tone: "success" });
        setOpen(false);
      } else {
        const created = await createKey.mutateAsync(payload);
        setPlaintext(created.plaintextKey ?? null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("feedback.failed"));
    }
  }

  async function copyKey() {
    if (!plaintext) return;
    await navigator.clipboard.writeText(plaintext);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  function setOpen(next: boolean) {
    if (controlled) onOpenChange?.(next);
    else setInternalOpen(next);
    if (!next) {
      setName("");
      setAllowedModels([]);
      setGroupIds([]);
      setAllowedIps([]);
      setDeniedIps([]);
      setLimit5h("");
      setLimit1d("");
      setLimit7d("");
      setRpmLimit("");
      setTpmLimit("");
      setConcurrencyLimit("");
      setExpiresAt("");
      setError(null);
      setPlaintext(null);
      setCopied(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      {controlled ? null : (
        <DialogTrigger asChild>
          <Button variant="primary">＋ {t("apiKeys.create")}</Button>
        </DialogTrigger>
      )}
      <DialogContent>
        {plaintext ? (
          <>
            <DialogHeader>
              <DialogTitle>{t("apiKeys.createdToast")}</DialogTitle>
              <DialogDescription>{t("apiKeys.revealOnce")}</DialogDescription>
            </DialogHeader>
            <div className="flex items-center gap-2 rounded-xl border border-srapi-border bg-srapi-card-muted px-3 py-2.5">
              <code className="flex-1 truncate font-mono text-sm">{plaintext}</code>
              <Button variant="outline" size="icon" onClick={copyKey} aria-label={t("apiKeys.copyKey")}>
                {copied ? <Check className="size-4 text-srapi-success" /> : <Copy className="size-4" />}
              </Button>
            </div>
            <DialogFooter>
              <Button variant="primary" onClick={() => setOpen(false)}>
                {t("common.close")}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={onSubmit}>
            <DialogHeader>
              <DialogTitle>{isEdit ? t("apiKeys.edit") : t("apiKeys.create")}</DialogTitle>
              <DialogDescription>{t("apiKeys.subtitle")}</DialogDescription>
            </DialogHeader>
            <div className="mt-4 space-y-4">
              <div>
                <Label htmlFor="key-name">{t("apiKeys.name")}</Label>
                <Input id="key-name" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div>
                <Label htmlFor="key-models">{t("apiKeys.allowedModels")}</Label>
                <TagInput
                  id="key-models"
                  value={allowedModels}
                  onChange={setAllowedModels}
                  disabled={pending}
                  placeholder="claude-3-7-sonnet, gpt-4o"
                />
                <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("apiKeys.modelsHint")}</p>
              </div>
              <div>
                <Label htmlFor="key-groups">{t("apiKeys.groups")}</Label>
                <TagInput
                  id="key-groups"
                  value={groupIds}
                  onChange={setGroupIds}
                  disabled={pending}
                  placeholder="default"
                />
                <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("apiKeys.groupsHint")}</p>
              </div>
              <div className="border-t border-srapi-border pt-4">
                <p className="text-2xs font-medium uppercase tracking-wide text-srapi-text-tertiary">
                  {t("apiKeys.accessControl")}
                </p>
              </div>
              <div>
                <Label htmlFor="key-allowed-ips">{t("apiKeys.allowedIps")}</Label>
                <TagInput
                  id="key-allowed-ips"
                  value={allowedIps}
                  onChange={setAllowedIps}
                  disabled={pending}
                  placeholder="10.0.0.0/8, 203.0.113.7"
                />
                <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("apiKeys.allowedIpsHint")}</p>
              </div>
              <div>
                <Label htmlFor="key-denied-ips">{t("apiKeys.deniedIps")}</Label>
                <TagInput
                  id="key-denied-ips"
                  value={deniedIps}
                  onChange={setDeniedIps}
                  disabled={pending}
                  placeholder="198.51.100.0/24"
                />
                <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("apiKeys.deniedIpsHint")}</p>
              </div>
              <div>
                <Label>{t("apiKeys.windowLimits")}</Label>
                <div className="mt-1 grid grid-cols-3 gap-2">
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    aria-label={t("apiKeys.windowLimit5h")}
                    placeholder={t("apiKeys.windowLimit5h")}
                    value={limit5h}
                    onChange={(e) => setLimit5h(e.target.value)}
                    disabled={pending}
                  />
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    aria-label={t("apiKeys.windowLimit1d")}
                    placeholder={t("apiKeys.windowLimit1d")}
                    value={limit1d}
                    onChange={(e) => setLimit1d(e.target.value)}
                    disabled={pending}
                  />
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    aria-label={t("apiKeys.windowLimit7d")}
                    placeholder={t("apiKeys.windowLimit7d")}
                    value={limit7d}
                    onChange={(e) => setLimit7d(e.target.value)}
                    disabled={pending}
                  />
                </div>
                <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("apiKeys.windowLimitsHint")}</p>
              </div>
              <div>
                <Label>{t("apiKeys.throughputLimits")}</Label>
                <div className="mt-1 grid grid-cols-3 gap-2">
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    aria-label={t("apiKeys.rpm")}
                    placeholder={t("apiKeys.rpm")}
                    value={rpmLimit}
                    onChange={(e) => setRpmLimit(e.target.value)}
                    disabled={pending}
                  />
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    aria-label={t("apiKeys.tpm")}
                    placeholder={t("apiKeys.tpm")}
                    value={tpmLimit}
                    onChange={(e) => setTpmLimit(e.target.value)}
                    disabled={pending}
                  />
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    aria-label={t("apiKeys.concurrency")}
                    placeholder={t("apiKeys.concurrency")}
                    value={concurrencyLimit}
                    onChange={(e) => setConcurrencyLimit(e.target.value)}
                    disabled={pending}
                  />
                </div>
                <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("apiKeys.throughputLimitsHint")}</p>
              </div>
              <div>
                <Label htmlFor="key-expires">{t("apiKeys.expiresAt")}</Label>
                <Input
                  id="key-expires"
                  type="datetime-local"
                  value={expiresAt}
                  onChange={(e) => setExpiresAt(e.target.value)}
                  disabled={pending}
                />
                <p className="mt-1 text-2xs text-srapi-text-tertiary">
                  {isEdit ? t("apiKeys.expiresAtEditHint") : t("apiKeys.expiresAtHint")}
                </p>
              </div>
              {error && (
                <p role="alert" className="text-sm text-srapi-error">
                  {error}
                </p>
              )}
            </div>
            <DialogFooter className="mt-6">
              <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" variant="primary" loading={pending}>
                {isEdit ? t("common.save") : t("common.create")}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
