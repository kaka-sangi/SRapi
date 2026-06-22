"use client";

import { useMemo, useState } from "react";
import { Users, UserMinus } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { MultiSelect } from "@/components/ui/multi-select";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DataPill } from "@/components/ui/data-pill";
import { SectionTitle } from "@/components/ui/section-title";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import {
  useGroupMembers,
  useAdminAccounts,
  useAddGroupMember,
  useRemoveGroupMember,
} from "@/hooks/admin-queries";
import { adminErrorMessage } from "@/lib/admin-api";
import type { AccountGroup } from "@/lib/sdk-types";

/**
 * Add / remove provider accounts in an account group. Uses the admin
 * list-group-members endpoint plus the add/remove member mutations; account
 * names are resolved from the accounts list (falls back to the raw id).
 */
export function GroupMembersDialog({
  open,
  onOpenChange,
  group,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group: AccountGroup;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const members = useGroupMembers(group.id);
  const accounts = useAdminAccounts();
  const addMut = useAddGroupMember();
  const removeMut = useRemoveGroupMember();
  const [toAdd, setToAdd] = useState<string[]>([]);
  const [adding, setAdding] = useState(false);

  const accountList = useMemo(() => accounts.data?.data ?? [], [accounts.data]);
  const memberRows = useMemo(() => members.data?.data ?? [], [members.data]);
  const accountById = useMemo(
    () => new Map(accountList.map((a) => [a.id, a] as const)),
    [accountList],
  );
  const accountName = (id: string) => accountById.get(id)?.name ?? id;
  const memberIds = new Set(memberRows.map((m) => m.account_id));
  const addable = accountList.filter((a) => !memberIds.has(a.id));

  // Tooltip breakdown: provider mix (top-3) + total addable pool size, so the
  // operator sees what's in the group without opening each row.
  const providerBreakdown = useMemo(() => {
    const counts = new Map<string, number>();
    for (const m of memberRows) {
      const acct = accountById.get(m.account_id);
      const provider = acct?.provider_id != null ? String(acct.provider_id) : "—";
      counts.set(provider, (counts.get(provider) ?? 0) + 1);
    }
    return Array.from(counts.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 3);
  }, [memberRows, accountById]);

  // Batch add: the API only takes one account at a time, so fan the selected
  // ids out over the single-member mutation and report an aggregate result —
  // the operator picks many accounts once instead of select-then-click N times.
  async function add() {
    if (toAdd.length === 0) return;
    setAdding(true);
    try {
      const results = await Promise.allSettled(
        toAdd.map((accountId) => addMut.mutateAsync({ accountId, groupId: group.id })),
      );
      const ok = results.filter((r) => r.status === "fulfilled").length;
      const failed = results.length - ok;
      if (ok > 0) {
        toast({
          title: t("adminGroups.membersAdded", { count: ok }),
          description:
            failed > 0 ? t("adminGroups.membersAddFailed", { count: failed }) : undefined,
          tone: failed > 0 ? "warning" : "success",
        });
      } else {
        const firstErr = results.find((r) => r.status === "rejected") as
          | PromiseRejectedResult
          | undefined;
        toast({
          title: t("feedback.failed"),
          description: firstErr ? adminErrorMessage(firstErr.reason) : undefined,
          tone: "error",
        });
      }
      setToAdd([]);
    } finally {
      setAdding(false);
    }
  }

  async function remove(accountId: string) {
    try {
      await removeMut.mutateAsync({ accountId, groupId: group.id });
      toast({ title: t("adminGroups.memberRemoved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("adminGroups.manageMembers")}</DialogTitle>
          <DialogDescription>{group.name}</DialogDescription>
        </DialogHeader>

        <div className="mt-4 space-y-4">
          <div className="flex items-end gap-2">
            <div className="flex-1">
              <MultiSelect
                value={toAdd}
                onChange={setToAdd}
                options={addable.map((a) => ({ value: a.id, label: a.name }))}
                placeholder={t("adminGroups.selectAccounts")}
                searchPlaceholder={t("adminGroups.searchAccounts")}
                emptyText={t("adminGroups.allAccountsInGroup")}
                disabled={adding || addable.length === 0}
              />
            </div>
            <Button
              type="button"
              variant="primary"
              loading={adding}
              disabled={toAdd.length === 0}
              onClick={() => void add()}
            >
              {t("adminGroups.addSelected", { count: toAdd.length })}
            </Button>
          </div>

          <div className="rounded-2xl border border-srapi-border bg-srapi-card">
            <div className="border-b border-srapi-border/70 px-4 py-3">
              <SectionTitle
                icon={<Users />}
                label={t("adminGroups.members")}
                action={
                  memberRows.length > 0 ? (
                    <DataTooltip
                      title={t("adminGroups.members")}
                      primary={
                        <span className="tabular">
                          {memberRows.length}
                          <span className="ml-1 text-xs font-normal text-srapi-text-tertiary">
                            / {accountList.length}
                          </span>
                        </span>
                      }
                      rows={[
                        ...providerBreakdown.map(([provider, count]) => ({
                          label: `provider #${provider}`,
                          value: String(count),
                        })),
                        {
                          label: t("adminGroups.allAccountsInGroup")
                            .replace(/[.。]$/, "")
                            .toLowerCase(),
                          value: String(addable.length),
                          tone: "muted" as const,
                        },
                      ]}
                    >
                      <DataPill tone="accent" size="sm" className="metric-tertiary cursor-help">
                        {memberRows.length}
                      </DataPill>
                    </DataTooltip>
                  ) : undefined
                }
              />
            </div>
            {members.isLoading ? (
              <DialogListSkeleton rows={2} className="p-3" />
            ) : memberRows.length === 0 ? (
              <div className="px-3 py-6">
                <IllustratedEmptyState
                  illust="accounts"
                  title={t("adminGroups.membersEmpty")}
                />
              </div>
            ) : (
              <ul className="divide-y divide-srapi-border/70">
                {memberRows.map((m) => {
                  const acct = accountById.get(m.account_id);
                  const providerLabel =
                    acct?.provider_id != null ? `provider #${acct.provider_id}` : null;
                  return (
                    <li
                      key={m.id}
                      className="flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-srapi-card-muted/60"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm text-srapi-text-primary">
                          {accountName(m.account_id)}
                        </p>
                        {providerLabel ? (
                          <p className="mt-0.5 text-[11px] font-mono text-srapi-text-tertiary">
                            {providerLabel}
                          </p>
                        ) : null}
                      </div>
                      <button
                        type="button"
                        onClick={() => void remove(m.account_id)}
                        disabled={removeMut.isPending}
                        aria-label={t("adminGroups.removeMember")}
                        className="text-srapi-text-tertiary transition-colors hover:text-srapi-error"
                      >
                        <UserMinus className="size-4" />
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </div>

        <DialogFooter className="mt-6">
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
