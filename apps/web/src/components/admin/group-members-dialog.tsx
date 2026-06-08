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
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
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
  const [toAdd, setToAdd] = useState("");

  const accountList = accounts.data?.data ?? [];
  const memberRows = members.data?.data ?? [];
  const accountName = (id: string) => accountList.find((a) => a.id === id)?.name ?? id;
  const memberIds = new Set(memberRows.map((m) => m.account_id));
  const addable = accountList.filter((a) => !memberIds.has(a.id));

  async function add() {
    if (!toAdd) return;
    try {
      await addMut.mutateAsync({ accountId: toAdd, groupId: group.id });
      toast({ title: t("adminGroups.memberAdded"), tone: "success" });
      setToAdd("");
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
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
              <Select
                value={toAdd}
                onValueChange={setToAdd}
                disabled={addMut.isPending || addable.length === 0}
              >
                <SelectTrigger>
                  <SelectValue placeholder={t("adminGroups.selectAccount")} />
                </SelectTrigger>
                <SelectContent>
                  {addable.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      {a.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button
              type="button"
              variant="primary"
              loading={addMut.isPending}
              disabled={!toAdd}
              onClick={() => void add()}
            >
              {t("adminGroups.addMember")}
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
