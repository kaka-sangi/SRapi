"use client";

import { useState } from "react";
import { KeyRound, MoreHorizontal } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useApiKeys, useToggleApiKey } from "@/hooks/queries";
import type { ApiKeySummary } from "@/lib/srapi-types";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card } from "@/components/ui/card";
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { ApiKeyCreateDialog, ApiKeyFormDialog } from "@/components/features/api-key-create-dialog";
import { ApiKeyUsageDialog } from "@/components/features/api-key-usage-dialog";

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
  const [editKey, setEditKey] = useState<ApiKeySummary | null>(null);
  const [usageKey, setUsageKey] = useState<ApiKeySummary | null>(null);

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
                        <TableCell className="font-mono text-srapi-text-tertiary">
                          {key.prefix}
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
                        <TableCell className="font-mono text-srapi-text-tertiary tabular">
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
    </>
  );
}

function TableSkeleton() {
  return (
    <div className="space-y-2 p-5">
      <Skeleton className="h-9" />
      <Skeleton className="h-9" />
      <Skeleton className="h-9 w-2/3" />
    </div>
  );
}
