"use client";

import { useState } from "react";
import { X } from "lucide-react";
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

  const accountList = accounts.data?.data ?? [];
  const memberRows = members.data?.data ?? [];
  const accountName = (id: string) => accountList.find((a) => a.id === id)?.name ?? id;
  const memberIds = new Set(memberRows.map((m) => m.account_id));
  const addable = accountList.filter((a) => !memberIds.has(a.id));

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

          <div className="rounded-lg border border-srapi-border">
            <div className="border-b border-srapi-border px-3 py-2 font-mono text-2xs uppercase tracking-widest text-srapi-text-secondary">
              {t("adminGroups.members")}
            </div>
            {members.isLoading ? (
              <DialogListSkeleton rows={2} className="p-3" />
            ) : memberRows.length === 0 ? (
              <p className="px-3 py-6 text-center text-2xs text-srapi-text-tertiary">
                {t("adminGroups.membersEmpty")}
              </p>
            ) : (
              <ul className="divide-y divide-srapi-border">
                {memberRows.map((m) => (
                  <li key={m.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <span className="truncate text-sm text-srapi-text-primary">
                      {accountName(m.account_id)}
                    </span>
                    <button
                      type="button"
                      onClick={() => void remove(m.account_id)}
                      disabled={removeMut.isPending}
                      aria-label={t("adminGroups.removeMember")}
                      className="text-srapi-text-tertiary transition-colors hover:text-srapi-error"
                    >
                      <X className="size-4" />
                    </button>
                  </li>
                ))}
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
