"use client";

import { useState } from "react";
import { KeyRound, MoreHorizontal, Copy, Check } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useApiKeys, useToggleApiKey, useDeleteApiKey } from "@/hooks/queries";
import type { ApiKeySummary } from "@/lib/srapi-types";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card } from "@/components/ui/card";
import { writeClipboard } from "@/components/ui/copy-button";
import {
  Table,
  TableScroll,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { EmptyState } from "@/components/ui/empty-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { ApiKeyCreateDialog, ApiKeyFormDialog } from "@/components/features/api-key-create-dialog";
import { ApiKeyUsageDialog } from "@/components/features/api-key-usage-dialog";
import { ApiKeyOnboarding } from "@/components/features/api-key-onboarding";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";

export default function ApiKeysPage() {
  return (
    <AppShell allowedRole="user">
      <ApiKeysContent />
    </AppShell>
  );
}

function ApiKeysContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const apiKeys = useApiKeys();
  const toggle = useToggleApiKey();
  const deleteKey = useDeleteApiKey();
  const [editKey, setEditKey] = useState<ApiKeySummary | null>(null);
  const [usageKey, setUsageKey] = useState<ApiKeySummary | null>(null);
  const [connectKey, setConnectKey] = useState<ApiKeySummary | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ApiKeySummary | null>(null);

  async function runToggle(id: string, status: "active" | "disabled") {
    try {
      await toggle.mutateAsync({ id, status });
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch {
      toast({ title: t("feedback.failed"), tone: "error" });
    }
  }

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionWorkspace")}
        title={t("apiKeys.title")}
        description={t("apiKeys.subtitle")}
        actions={<ApiKeyCreateDialog />}
      />

      <Card className="anim-rise-sm">
        <PageQueryState
          query={apiKeys}
          isEmpty={(d) => d.length === 0}
          skeleton={<TableSkeleton />}
        >
          {(data) =>
            data.length === 0 ? (
              <EmptyState
                icon={KeyRound}
                title={t("apiKeys.emptyTitle")}
                description={t("apiKeys.emptyBody")}
              />
            ) : (
              <TableScroll minWidth={560}>
                <Table>
                  <TableHeader>
                    <tr>
                      <TableHead>{t("apiKeys.name")}</TableHead>
                      <TableHead>{t("apiKeys.prefix")}</TableHead>
                      <TableHead>{t("apiKeys.status")}</TableHead>
                      <TableHead>{t("apiKeys.allowedModels")}</TableHead>
                      <TableHead>{t("apiKeys.created")}</TableHead>
                      <TableHead aria-label="actions" />
                    </tr>
                  </TableHeader>
                  <TableBody>
                    {data.map((key) => (
                      <TableRow key={key.id} className={key.status === "disabled" ? "opacity-50" : ""}>
                        <TableCell className="text-srapi-text-primary">{key.name}</TableCell>
                        <TableCell>
                          <KeyPrefixCopy prefix={key.prefix} />
                        </TableCell>
                        <TableCell>
                          <QuietBadge
                            status={key.status === "active" ? "active" : "disabled"}
                            label={key.status === "active" ? t("common.active") : t("common.disabled")}
                          />
                        </TableCell>
                        <TableCell className="text-srapi-text-secondary">
                          {key.allowed_models.length === 0 ? t("common.all") : key.allowed_models.join(" · ")}
                        </TableCell>
                        <TableCell className="text-[12px] text-srapi-text-tertiary tabular">
                          {key.created_at.slice(0, 10)}
                        </TableCell>
                        <TableCell className="text-right">
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="icon" aria-label="actions">
                                <MoreHorizontal className="size-4" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              <DropdownMenuItem onClick={() => setConnectKey(key)}>
                                {t("apiKeys.onboardingAction")}
                              </DropdownMenuItem>
                              <DropdownMenuItem onClick={() => setUsageKey(key)}>
                                {t("apiKeys.usageAction")}
                              </DropdownMenuItem>
                              <DropdownMenuItem onClick={() => setEditKey(key)}>
                                {t("common.edit")}
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                disabled={toggle.isPending}
                                onClick={() => void runToggle(key.id, key.status)}
                              >
                                {key.status === "active" ? t("common.disable") : t("common.enable")}
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() => setDeleteTarget(key)}
                                className="text-srapi-error focus:text-srapi-error"
                              >
                                {t("common.delete")}
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableScroll>
            )
          }
        </PageQueryState>
      </Card>

      <ApiKeyFormDialog
        key={editKey?.id ?? "none"}
        editKey={editKey ?? undefined}
        open={editKey !== null}
        onOpenChange={(next) => {
          if (!next) setEditKey(null);
        }}
      />

      <ApiKeyUsageDialog
        keyId={usageKey?.id ?? null}
        keyName={usageKey?.name ?? ""}
        variant="me"
        open={usageKey !== null}
        onOpenChange={(next) => {
          if (!next) setUsageKey(null);
        }}
      />

      <Dialog
        open={connectKey !== null}
        onOpenChange={(next) => {
          if (!next) setConnectKey(null);
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle className="font-sans text-lg font-semibold tracking-tight">
              {t("apiKeys.onboardingAction")}
            </DialogTitle>
            <DialogDescription>
              {connectKey ? `${connectKey.name} · ${connectKey.prefix}…` : null}
            </DialogDescription>
          </DialogHeader>
          {connectKey ? (
            <ApiKeyOnboarding
              key={connectKey.id}
              apiKey={null}
              allowPaste
              defaultModel={connectKey.allowed_models[0]}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(next) => {
            if (!next) setDeleteTarget(null);
          }}
          title={t("feedback.confirmDeleteTitle", { name: deleteTarget.name })}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("common.delete")}
          onConfirm={() => deleteKey.mutateAsync(deleteTarget.id)}
          successMessage={t("feedback.deleted")}
          isPending={deleteKey.isPending}
        />
      ) : null}
    </>
  );
}

function KeyPrefixCopy({ prefix }: { prefix: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      onClick={() => {
        void writeClipboard(prefix).then((ok) => {
          if (!ok) return;
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
      className="group inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-[12px] font-medium text-srapi-text-tertiary tabular transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-secondary"
      title={prefix}
    >
      <span>{prefix}...</span>
      {copied ? (
        <Check className="size-3 text-srapi-success" />
      ) : (
        <Copy className="size-3 opacity-0 transition-opacity group-hover:opacity-100" />
      )}
    </button>
  );
}

function TableSkeleton() {
  return <DialogListSkeleton rows={3} className="p-5" />;
}
