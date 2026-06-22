"use client";

import { useState, useEffect } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { CopyButton } from "@/components/ui/copy-button";
import {
  useModelAliases,
  useModelMappings,
  useDeleteModelAlias,
  useDeleteModelMapping,
  useAdminAccounts,
} from "@/hooks/admin-queries";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/query-keys";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type { Model, ModelAlias, ModelProviderMapping } from "@/lib/sdk-types";

// ModelDetailDialog lists a model's aliases and provider mappings — the only place
// they are visible — and lets an admin remove individual ones (delete with an
// inline confirm to avoid a nested modal). "Add" hands back to the parent so the
// existing create dialogs open after this one closes.
export function ModelDetailDialog({
  model,
  providerLabels,
  onClose,
  onAddAlias,
  onAddMapping,
  onEditAlias,
  onEditMapping,
}: {
  model: Model;
  providerLabels: Map<string, string>;
  onClose: () => void;
  onAddAlias: () => void;
  onAddMapping: () => void;
  // Optional — when present, each alias/mapping row gets an Edit pencil that
  // pops the row's edit dialog in the parent. Kept optional so existing
  // callers (and tests) don't have to wire it.
  onEditAlias?: (alias: ModelAlias) => void;
  onEditMapping?: (mapping: ModelProviderMapping) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const aliases = useModelAliases(model.id);
  const mappings = useModelMappings(model.id);
  const deleteAlias = useDeleteModelAlias();
  const deleteMapping = useDeleteModelMapping();
  // The row pending inline-confirm, keyed as "alias:<id>" / "mapping:<id>".
  const [confirmKey, setConfirmKey] = useState<string | null>(null);

  // Refetch accounts on mount to ensure fresh data when dialog reopens
  useEffect(() => {
    void queryClient.refetchQueries({
      queryKey: queryKeys.admin.accounts({ page: 1, page_size: 200, status: "active" }),
    });
  }, [queryClient]);

  const aliasRows = aliases.data?.data ?? [];
  const mappingRows = mappings.data?.data ?? [];

  // Count active upstream accounts per provider so each mapping can show whether
  // anything can actually serve it. A model can be enabled and mapped yet have
  // zero accounts on its provider — in which case requests fail at the gateway
  // with no obvious cause. Surfacing the count (and a warning at zero) closes
  // that gap.
  const accounts = useAdminAccounts({ page: 1, page_size: 200, status: "active" });
  const activeAccountsByProvider = new Map<string, number>();
  for (const account of accounts.data?.data ?? []) {
    const providerKey = String(account.provider_id);
    activeAccountsByProvider.set(providerKey, (activeAccountsByProvider.get(providerKey) ?? 0) + 1);
  }

  async function removeAlias(aliasId: string) {
    try {
      await deleteAlias.mutateAsync({ id: model.id, aliasId });
      toast({ title: t("feedback.deleted"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    } finally {
      setConfirmKey(null);
    }
  }

  async function removeMapping(mappingId: string) {
    try {
      await deleteMapping.mutateAsync({ id: model.id, mappingId });
      toast({ title: t("feedback.deleted"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    } finally {
      setConfirmKey(null);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("adminModels.manageTitle")}</DialogTitle>
        </DialogHeader>
        <p className="-mt-1 flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
          <span className="font-mono">{model.canonical_name}</span>
          <CopyButton value={model.canonical_name} size="inline" />
        </p>

        <div className="mt-5 max-h-[60vh] space-y-6 overflow-y-auto pr-1">
          <section>
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminModels.aliasesSection")}
              </h3>
              <Button variant="ghost" size="sm" onClick={onAddAlias}>
                <Plus className="size-3.5" /> {t("adminModels.addAlias")}
              </Button>
            </div>
            {aliases.isLoading ? (
              <p className="py-3 text-xs text-srapi-text-tertiary">{t("common.loading")}</p>
            ) : aliasRows.length === 0 ? (
              <p className="py-3 text-xs text-srapi-text-tertiary">{t("adminModels.aliasesEmpty")}</p>
            ) : (
              <ul className="divide-y divide-srapi-border/70">
                {aliasRows.map((a) => {
                  const key = `alias:${a.id}`;
                  return (
                    <li key={a.id} className="flex items-center justify-between gap-3 py-3">
                      <span className="truncate font-mono text-xs text-srapi-text-primary">{a.alias}</span>
                      <div className="flex shrink-0 items-center gap-2">
                        <QuietBadge status={quietStatusFor(a.status)} label={statusLabel(t, a.status)} />
                        {onEditAlias && confirmKey !== key ? (
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t("common.edit")}
                            onClick={() => onEditAlias(a)}
                            className="text-srapi-text-tertiary hover:text-srapi-text-primary"
                          >
                            <Pencil className="size-4" />
                          </Button>
                        ) : null}
                        <RowRemove
                          confirming={confirmKey === key}
                          pending={deleteAlias.isPending}
                          onAsk={() => setConfirmKey(key)}
                          onCancel={() => setConfirmKey(null)}
                          onConfirm={() => void removeAlias(a.id)}
                        />
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          <section>
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminModels.mappingsSection")}
              </h3>
              <Button variant="ghost" size="sm" onClick={onAddMapping}>
                <Plus className="size-3.5" /> {t("adminModels.addMapping")}
              </Button>
            </div>
            {mappings.isLoading ? (
              <p className="py-3 text-xs text-srapi-text-tertiary">{t("common.loading")}</p>
            ) : mappingRows.length === 0 ? (
              <p className="py-3 text-xs text-srapi-text-tertiary">{t("adminModels.mappingsEmpty")}</p>
            ) : (
              <ul className="divide-y divide-srapi-border/70">
                {mappingRows.map((m) => {
                  const key = `mapping:${m.id}`;
                  const provider = providerLabels.get(String(m.provider_id)) ?? `#${m.provider_id}`;
                  const serving = activeAccountsByProvider.get(String(m.provider_id)) ?? 0;
                  return (
                    <li key={m.id} className="flex items-center justify-between gap-3 py-3">
                      <span className="min-w-0 truncate text-xs text-srapi-text-primary">
                        <span className="font-medium text-srapi-text-secondary">{provider}</span>
                        <span className="text-srapi-text-tertiary"> · </span>
                        <span className="font-mono">{m.upstream_model_name}</span>
                        {!accounts.isLoading ? (
                          serving > 0 ? (
                            <span className="text-srapi-text-tertiary">
                              {" · "}
                              {t("adminModels.servingAccounts", { count: serving })}
                            </span>
                          ) : (
                            <span className="text-srapi-error">
                              {" · "}
                              {t("adminModels.noServingAccount")}
                            </span>
                          )
                        ) : null}
                      </span>
                      <div className="flex shrink-0 items-center gap-2">
                        <QuietBadge status={quietStatusFor(m.status)} label={statusLabel(t, m.status)} />
                        {onEditMapping && confirmKey !== key ? (
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={t("common.edit")}
                            onClick={() => onEditMapping(m)}
                            className="text-srapi-text-tertiary hover:text-srapi-text-primary"
                          >
                            <Pencil className="size-4" />
                          </Button>
                        ) : null}
                        <RowRemove
                          confirming={confirmKey === key}
                          pending={deleteMapping.isPending}
                          onAsk={() => setConfirmKey(key)}
                          onCancel={() => setConfirmKey(null)}
                          onConfirm={() => void removeMapping(m.id)}
                        />
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>
        </div>

        <DialogFooter className="mt-6">
          <Button variant="ghost" onClick={onClose}>
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RowRemove({
  confirming,
  pending,
  onAsk,
  onCancel,
  onConfirm,
}: {
  confirming: boolean;
  pending: boolean;
  onAsk: () => void;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useLanguage();
  if (confirming) {
    return (
      <span className="flex items-center gap-1.5">
        <Button variant="ghost" size="sm" onClick={onCancel} disabled={pending}>
          {t("common.cancel")}
        </Button>
        <Button variant="danger" size="sm" onClick={onConfirm} disabled={pending}>
          {t("common.delete")}
        </Button>
      </span>
    );
  }
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={t("common.delete")}
      onClick={onAsk}
      className="text-srapi-text-tertiary hover:text-srapi-error"
    >
      <Trash2 className="size-4" />
    </Button>
  );
}
