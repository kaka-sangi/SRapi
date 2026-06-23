"use client";

import { useState, useEffect } from "react";
import { Pencil, Plus, Trash2, Tags, Network, AlertTriangle, ServerCog } from "lucide-react";
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
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DataPill } from "@/components/ui/data-pill";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
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
  const [tab, setTab] = useState<"aliases" | "mappings">("aliases");

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

  // Roll-up: how many mappings can be served right now? Drives the header
  // DataTooltip so the operator sees coverage at a glance.
  const mappingsServed = mappingRows.filter(
    (m) => (activeAccountsByProvider.get(String(m.provider_id)) ?? 0) > 0,
  ).length;
  const mappingsUnserved = mappingRows.length - mappingsServed;

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
        <div className="-mt-1 flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
            <span className="font-mono">{model.canonical_name}</span>
            <CopyButton value={model.canonical_name} size="inline" />
          </span>
          <DataTooltip
            title={t("adminModels.aliasesSection")}
            primary={<span className="tabular">{aliasRows.length}</span>}
            rows={[
              {
                label: t("adminModels.aliasesSection"),
                value: String(aliasRows.length),
              },
              {
                label: t("adminModels.mappingsSection"),
                value: String(mappingRows.length),
              },
            ]}
          >
            <DataPill tone="accent" size="sm" className="metric-tertiary cursor-help">
              <Tags className="size-3" /> {aliasRows.length}
            </DataPill>
          </DataTooltip>
          <DataTooltip
            title={t("adminModels.mappingsSection")}
            primary={
              <span className="tabular">
                {mappingsServed}
                <span className="ml-1 text-xs font-normal text-srapi-text-tertiary">
                  / {mappingRows.length}
                </span>
              </span>
            }
            rows={[
              {
                label: t("adminModels.servingAccounts", { count: mappingsServed }).replace(
                  /\(\d+\)|\d+/,
                  "",
                ).trim() || "served",
                value: String(mappingsServed),
                tone: "success",
              },
              ...(mappingsUnserved > 0
                ? [
                    {
                      label: t("adminModels.noServingAccount"),
                      value: String(mappingsUnserved),
                      tone: "error" as const,
                    },
                  ]
                : []),
            ]}
          >
            <DataPill
              tone={mappingsUnserved > 0 ? "warning" : "success"}
              size="sm"
              className="metric-tertiary cursor-help"
            >
              <Network className="size-3" /> {mappingsServed}/{mappingRows.length}
            </DataPill>
          </DataTooltip>
        </div>

        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as "aliases" | "mappings")}
          className="mt-5"
        >
          <TabsList>
            <TabsTrigger value="aliases">
              <Tags className="mr-1.5 size-3.5" />
              {t("adminModels.aliasesSection")}
              <span className="ml-1.5 text-[11px] text-srapi-text-tertiary tabular">
                {aliasRows.length}
              </span>
            </TabsTrigger>
            <TabsTrigger value="mappings">
              <Network className="mr-1.5 size-3.5" />
              {t("adminModels.mappingsSection")}
              <span className="ml-1.5 text-[11px] text-srapi-text-tertiary tabular">
                {mappingRows.length}
              </span>
            </TabsTrigger>
          </TabsList>

          <TabsContent value="aliases" className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1">
            <div className="mb-2 flex items-center justify-end">
              <Button variant="ghost" size="sm" onClick={onAddAlias}>
                <Plus className="size-3.5" /> {t("adminModels.addAlias")}
              </Button>
            </div>
            {aliases.isLoading ? (
              <p className="py-3 text-xs text-srapi-text-tertiary">{t("common.loading")}</p>
            ) : aliasRows.length === 0 ? (
              <IllustratedEmptyState
                illust="search"
                title={t("adminModels.aliasesEmpty")}
                action={
                  <Button variant="primary" size="sm" onClick={onAddAlias}>
                    <Plus className="size-3.5" /> {t("adminModels.addAlias")}
                  </Button>
                }
              />
            ) : (
              <ul className="divide-y divide-srapi-border/70">
                {aliasRows.map((a) => {
                  const key = `alias:${a.id}`;
                  return (
                    <li key={a.id} className="flex items-center justify-between gap-3 py-3">
                      <div className="min-w-0 flex flex-1 items-center gap-2">
                        <span className="truncate font-mono text-xs text-srapi-text-primary">
                          {a.alias}
                        </span>
                        <CopyButton value={a.alias} size="inline" />
                      </div>
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
          </TabsContent>

          <TabsContent value="mappings" className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1">
            <div className="mb-2 flex items-center justify-end">
              <Button variant="ghost" size="sm" onClick={onAddMapping}>
                <Plus className="size-3.5" /> {t("adminModels.addMapping")}
              </Button>
            </div>
            {mappings.isLoading ? (
              <p className="py-3 text-xs text-srapi-text-tertiary">{t("common.loading")}</p>
            ) : mappingRows.length === 0 ? (
              <IllustratedEmptyState
                illust="accounts"
                title={t("adminModels.mappingsEmpty")}
                action={
                  <Button variant="primary" size="sm" onClick={onAddMapping}>
                    <Plus className="size-3.5" /> {t("adminModels.addMapping")}
                  </Button>
                }
              />
            ) : (
              <ul className="divide-y divide-srapi-border/70">
                {mappingRows.map((m) => {
                  const key = `mapping:${m.id}`;
                  const provider = providerLabels.get(String(m.provider_id)) ?? `#${m.provider_id}`;
                  const serving = activeAccountsByProvider.get(String(m.provider_id)) ?? 0;
                  return (
                    <li key={m.id} className="flex items-center justify-between gap-3 py-3">
                      <div className="min-w-0 flex-1 text-xs text-srapi-text-primary">
                        <div className="flex items-center gap-1.5">
                          <span className="font-medium text-srapi-text-secondary">{provider}</span>
                          <span className="text-srapi-text-tertiary">·</span>
                          <span className="truncate font-mono">{m.upstream_model_name}</span>
                        </div>
                        {!accounts.isLoading ? (
                          <div className="mt-1">
                            <DataTooltip
                              title={t("adminModels.mappingsSection")}
                              primary={
                                <span className="tabular">
                                  {serving} {provider}
                                </span>
                              }
                              rows={[
                                {
                                  label: t("adminModels.servingAccounts", { count: serving }),
                                  value: String(serving),
                                  tone: serving > 0 ? "success" : "error",
                                },
                                {
                                  label: t("adminCommon.status"),
                                  value: statusLabel(t, m.status),
                                  tone: "muted",
                                },
                              ]}
                              footer={
                                serving === 0 ? t("adminModels.noServingAccount") : undefined
                              }
                            >
                              {serving > 0 ? (
                                <DataPill tone="success" size="sm" className="cursor-help">
                                  <ServerCog className="size-3" />
                                  {t("adminModels.servingAccounts", { count: serving })}
                                </DataPill>
                              ) : (
                                <DataPill tone="error" size="sm" className="cursor-help">
                                  <AlertTriangle className="size-3" />
                                  {t("adminModels.noServingAccount")}
                                </DataPill>
                              )}
                            </DataTooltip>
                          </div>
                        ) : null}
                      </div>
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
          </TabsContent>
        </Tabs>

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
