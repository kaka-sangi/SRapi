"use client";

import { useState } from "react";
import { Boxes } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  useAdminGroups,
  useAdminProviders,
  useAdminModels,
  useCreateGroup,
  useUpdateGroup,
  useDeleteGroup,
  useGroupRateLimits,
  useUpsertGroupRateLimit,
  useDeleteGroupRateLimit,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  ACCOUNT_GROUP_STATUSES,
  GROUP_STRATEGY_HINTS,
  emptyAccountGroupForm,
  accountGroupFormFromGroup,
  buildCreateAccountGroupBody,
  buildUpdateAccountGroupBody,
  applyProviderScopeSelection,
  applyModelScopeSelection,
  type AccountGroupFormState,
} from "@/lib/admin-group-form";
import type { AccountGroup, AccountGroupRateLimit } from "@/lib/sdk-types";
import { GroupMembersDialog } from "@/components/admin/group-members-dialog";
import { RateLimitDialog } from "@/components/admin/rate-limit-dialog";
import { rateLimitSummary } from "@/lib/rate-limit-format";

const ALL = "__all__";

export default function AdminGroupsPage() {
  return (
    <AdminShell>
      <GroupsContent />
    </AdminShell>
  );
}

function GroupsContent() {
  const { t } = useLanguage();
  const groups = useAdminGroups();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-groups", []);
  const [formTarget, setFormTarget] = useState<AccountGroup | "new" | null>(null);
  const [membersTarget, setMembersTarget] = useState<AccountGroup | null>(null);
  const [rateLimitTarget, setRateLimitTarget] = useState<AccountGroup | null>(null);
  const [groupToDelete, setGroupToDelete] = useState<AccountGroup | null>(null);
  const deleteGroup = useDeleteGroup();
  const rateLimits = useGroupRateLimits();
  const upsertRl = useUpsertGroupRateLimit();
  const deleteRl = useDeleteGroupRateLimit();
  const rateLimitByGroup = new Map<number, AccountGroupRateLimit>(
    (rateLimits.data?.data ?? []).map((rl) => [rl.account_group_id, rl]),
  );

  const columns: Column<AccountGroup>[] = [
    {
      key: "name",
      header: t("adminGroups.name"),
      pinned: true,
      sortValue: (g) => g.name,
      render: (g) => <span className="text-srapi-text-primary">{g.name}</span>,
    },
    {
      key: "description",
      header: t("adminGroups.description"),
      hideOnMobile: true,
      render: (g) => <span className="text-srapi-text-secondary">{g.description || "—"}</span>,
    },
    {
      key: "strategy",
      header: t("adminGroups.strategy"),
      hideOnMobile: true,
      render: (g) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {g.strategy_hint || "default"}
        </span>
      ),
    },
    {
      key: "ratelimit",
      header: t("adminRateLimit.column"),
      hideOnMobile: true,
      render: (g) => {
        const rl = rateLimitByGroup.get(Number(g.id));
        if (!rl) {
          return <span className="text-2xs text-srapi-text-tertiary">{t("adminRateLimit.none")}</span>;
        }
        return (
          <span className="font-mono text-2xs text-srapi-text-secondary tabular">
            {rl.enabled ? rateLimitSummary(rl) : t("adminRateLimit.off")}
          </span>
        );
      },
    },
    {
      key: "status",
      header: t("common.active"),
      sortValue: (g) => g.status,
      render: (g) => <QuietBadge status={quietStatusFor(g.status)} label={statusLabel(t, g.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminGroups.title")}
        description={t("adminGroups.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {groups.data ? (
              <ListCount total={groups.data.pagination?.total ?? groups.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminGroups.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={groups}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(g) => g.id}
        emptyIcon={Boxes}
        emptyTitle={t("adminGroups.emptyTitle")}
        emptyBody={t("adminGroups.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminGroups.create")}
          </Button>
        }
        minWidth={480}
        sort={list.sort}
        onSort={list.toggleSort}
        rowActions={(g) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(g) },
              { label: t("adminGroups.manageMembers"), onSelect: () => setMembersTarget(g) },
              { label: t("adminRateLimit.action"), onSelect: () => setRateLimitTarget(g) },
              { label: t("common.delete"), destructive: true, onSelect: () => setGroupToDelete(g) },
            ]}
          />
        )}
      />

      <ConfirmDialog
        open={groupToDelete !== null}
        onOpenChange={(open) => {
          if (!open) setGroupToDelete(null);
        }}
        title={t("adminGroups.deleteTitle")}
        body={t("adminGroups.deleteBody", { name: groupToDelete?.name ?? "" })}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteGroup.isPending}
        onConfirm={async () => {
          if (groupToDelete) await deleteGroup.mutateAsync(groupToDelete.id);
        }}
      />

      {formTarget ? (
        <GroupFormDialog target={formTarget} onClose={() => setFormTarget(null)} />
      ) : null}

      {membersTarget ? (
        <GroupMembersDialog
          open
          group={membersTarget}
          onOpenChange={(open) => {
            if (!open) setMembersTarget(null);
          }}
        />
      ) : null}

      {rateLimitTarget ? (
        <RateLimitDialog
          open
          onOpenChange={(open) => {
            if (!open) setRateLimitTarget(null);
          }}
          title={t("adminRateLimit.title", { name: rateLimitTarget.name })}
          current={rateLimitByGroup.get(Number(rateLimitTarget.id))}
          onSubmit={(values) =>
            upsertRl.mutateAsync({ account_group_id: Number(rateLimitTarget.id), ...values })
          }
          onClear={
            rateLimitByGroup.has(Number(rateLimitTarget.id))
              ? () => deleteRl.mutateAsync(rateLimitTarget.id)
              : undefined
          }
          isPending={upsertRl.isPending || deleteRl.isPending}
        />
      ) : null}
    </>
  );
}

function GroupFormDialog({
  target,
  onClose,
}: {
  target: AccountGroup | "new";
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const providers = useAdminProviders();
  const models = useAdminModels();
  const createMut = useCreateGroup();
  const updateMut = useUpdateGroup();
  const isNew = target === "new";

  const [form, setForm] = useState<AccountGroupFormState>(() =>
    isNew ? emptyAccountGroupForm() : accountGroupFormFromGroup(target),
  );
  const [error, setError] = useState<string | null>(null);

  function set<K extends keyof AccountGroupFormState>(key: K, value: AccountGroupFormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    try {
      if (isNew) {
        await createMut.mutateAsync(buildCreateAccountGroupBody(form));
        toast({ title: t("feedback.created"), tone: "success" });
      } else {
        await updateMut.mutateAsync({ id: target.id, body: buildUpdateAccountGroupBody(form) });
        toast({ title: t("feedback.updated"), tone: "success" });
      }
      onClose();
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  const pending = createMut.isPending || updateMut.isPending;

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{isNew ? t("adminGroups.create") : t("adminGroups.edit")}</DialogTitle>
          </DialogHeader>
          <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
            <div>
              <Label htmlFor="g-name">{t("adminGroups.name")}</Label>
              <Input id="g-name" value={form.name} onChange={(e) => set("name", e.target.value)} />
            </div>
            <div>
              <Label htmlFor="g-desc">{t("adminGroups.description")}</Label>
              <Textarea
                id="g-desc"
                value={form.description}
                onChange={(e) => set("description", e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="g-provider">{t("adminGroups.providerScope")}</Label>
              <Select
                value={form.selectedProviderId || ALL}
                onValueChange={(v) =>
                  setForm((prev) => applyProviderScopeSelection(prev, v === ALL ? "" : v))
                }
              >
                <SelectTrigger id="g-provider">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>{t("adminGroups.allScope")}</SelectItem>
                  {(providers.data?.data ?? []).map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.display_name ?? p.id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="g-model">{t("adminGroups.modelScope")}</Label>
              <Select
                value={form.selectedModelName || ALL}
                onValueChange={(v) =>
                  setForm((prev) => applyModelScopeSelection(prev, v === ALL ? "" : v))
                }
              >
                <SelectTrigger id="g-model">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>{t("adminGroups.allScope")}</SelectItem>
                  {(models.data?.data ?? []).map((m) => (
                    <SelectItem key={m.id} value={m.canonical_name}>
                      {m.display_name ?? m.canonical_name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="g-strategy">{t("adminGroups.strategy")}</Label>
              <Select value={form.strategyHint} onValueChange={(v) => set("strategyHint", v)}>
                <SelectTrigger id="g-strategy">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {GROUP_STRATEGY_HINTS.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="g-status">{t("adminCommon.status")}</Label>
              <Select
                value={form.status}
                onValueChange={(v) => set("status", v as AccountGroupFormState["status"])}
              >
                <SelectTrigger id="g-status">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ACCOUNT_GROUP_STATUSES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-6">
            <Button type="button" variant="ghost" disabled={pending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={pending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
