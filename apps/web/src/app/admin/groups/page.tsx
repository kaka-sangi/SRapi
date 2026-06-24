"use client";

import { useState } from "react";
import { Boxes } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ADMIN_ROUTES } from "@/lib/routes";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { useClientPagedList } from "@/hooks/use-client-list";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
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
  useBatchSetGroupRateMultipliers,
  useBatchSetGroupRpmOverrides,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { DataPill } from "@/components/ui/data-pill";
import { Button } from "@/components/ui/button";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
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

function groupMatch(group: AccountGroup, term: string, filters: Record<string, string>): boolean {
  // Status segmented-control filter sits client-side because /admin/groups is
  // already paged in-memory via useClientPagedList — adding a server filter
  // would require an API change and we have the whole list locally.
  if (filters.status && group.status !== filters.status) return false;
  if (!term) return true;
  return [group.name, group.description, group.strategy_hint]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

// Summarise provider_scope / model_scope JSON for the inline expand row.
// The scope shape varies (empty -> "all", { provider: "foo" } -> "foo", { items: [...] } -> "n items"),
// so we coerce to a readable string and flag empty as muted so the operator
// instantly sees "this group covers everything" vs "this group is narrowed".
function scopeSummary(scope: unknown): { label: string; tone: "default" | "muted" } {
  if (!scope || typeof scope !== "object") return { label: "all", tone: "muted" };
  const obj = scope as Record<string, unknown>;
  const keys = Object.keys(obj);
  if (keys.length === 0) return { label: "all", tone: "muted" };
  const values: string[] = [];
  for (const key of keys) {
    const value = obj[key];
    if (Array.isArray(value)) {
      values.push(...value.map(String));
    } else if (value != null) {
      values.push(String(value));
    }
  }
  if (values.length === 0) return { label: "all", tone: "muted" };
  const shown = values.slice(0, 4).join(", ");
  const extra = values.length - 4;
  return { label: extra > 0 ? `${shown} +${extra}` : shown, tone: "default" };
}

const groupCompare = (a: AccountGroup, b: AccountGroup) => a.name.localeCompare(b.name);

function GroupsContent() {
  const { t } = useLanguage();
  const all = useAdminGroups();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-groups", []);
  const { query: groups, total } = useClientPagedList(all, list, {
    match: groupMatch,
    compare: groupCompare,
  });
  const isFiltered = Boolean(list.search || list.filters.status);
  const [formTarget, setFormTarget] = useState<AccountGroup | "new" | null>(null);
  const [membersTarget, setMembersTarget] = useState<AccountGroup | null>(null);
  const [rateLimitTarget, setRateLimitTarget] = useState<AccountGroup | null>(null);
  const [groupToDelete, setGroupToDelete] = useState<AccountGroup | null>(null);
  const deleteGroup = useDeleteGroup();
  const rateLimits = useGroupRateLimits();
  const upsertRl = useUpsertGroupRateLimit();
  const deleteRl = useDeleteGroupRateLimit();
  const batchMultipliers = useBatchSetGroupRateMultipliers();
  const batchRpm = useBatchSetGroupRpmOverrides();
  const [bulkMultiplierOpen, setBulkMultiplierOpen] = useState(false);
  const [bulkRpmOpen, setBulkRpmOpen] = useState(false);
  const { toast } = useToast();
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
        <DataPill tone="neutral" size="sm">{g.strategy_hint || "default"}</DataPill>
      ),
    },
    {
      key: "rate-multiplier",
      header: t("adminGroups.rateMultiplier"),
      hideOnMobile: true,
      align: "right",
      render: (g) => {
        const rl = rateLimitByGroup.get(Number(g.id));
        return (
          <DataTooltip
            title={t("adminGroups.rateMultiplier")}
            primary={`${g.rate_multiplier || "1.00000000"}×`}
            rows={[
              { label: t("adminGroups.strategy"), value: g.strategy_hint || "default", tone: "muted" },
              { label: t("adminRateLimit.column"), value: rl ? (rl.enabled ? rateLimitSummary(rl) : t("adminRateLimit.off")) : t("adminRateLimit.none"), tone: rl?.enabled ? "default" : "muted" },
            ]}
          >
            <span className="text-xs text-srapi-text-tertiary tabular">
              {g.rate_multiplier || "1.00000000"}×
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "ratelimit",
      header: t("adminRateLimit.column"),
      hideOnMobile: true,
      render: (g) => {
        const rl = rateLimitByGroup.get(Number(g.id));
        if (!rl) {
          return <span className="text-xs text-srapi-text-tertiary">{t("adminRateLimit.none")}</span>;
        }
        return (
          <span className="text-xs text-srapi-text-secondary tabular">
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
      <SectionHero
        eyebrow={t("hero.eyebrowGatewayGroups")}
        title={t("adminGroups.title")}
        description={t("adminGroups.subtitle")}
        metrics={
          all.data
            ? [{ label: t("adminCommon.total"), value: String(total) }]
            : undefined
        }
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
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
          <div className="flex gap-2">
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminGroups.create")}
            </Button>
            <Button variant="outline" size="sm" asChild>
              <a href={ADMIN_ROUTES.quickSetup}>{t("adminGroups.emptyQuickSetup")}</a>
            </Button>
          </div>
        }
        minWidth={480}
        sort={list.sort}
        onSort={list.toggleSort}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        enableKeyboardNav
        rowSeverity={(g) => {
          // Inactive groups get an info stripe so the operator can scan a
          // long list and notice deactivated ones without scrolling to the
          // status column. Active stays unstriped (the default visual).
          if (g.status !== "active") return "info";
          return undefined;
        }}
        expandRow={(g) => {
          const rl = rateLimitByGroup.get(Number(g.id));
          const providerScope = scopeSummary(g.provider_scope);
          const modelScope = scopeSummary(g.model_scope);
          return (
            <InlineDetailGrid
              sections={[
                {
                  title: t("adminGroups.description"),
                  rows: [
                    { label: t("adminGroups.description"), value: g.description || "—", tone: g.description ? "default" : "muted" },
                    { label: t("adminGroups.strategy"), value: g.strategy_hint || "default" },
                    { label: t("adminCommon.status"), value: statusLabel(t, g.status) },
                  ],
                },
                {
                  title: t("adminGroups.providerScope"),
                  rows: [
                    { label: t("adminGroups.providerScope"), value: providerScope.label, tone: providerScope.tone },
                    { label: t("adminGroups.modelScope"), value: modelScope.label, tone: modelScope.tone },
                  ],
                },
                {
                  title: t("adminRateLimit.column"),
                  rows: [
                    { label: t("adminGroups.rateMultiplier"), value: `${g.rate_multiplier || "1.00000000"}×` },
                    { label: t("adminRateLimit.column"), value: rl ? (rl.enabled ? rateLimitSummary(rl) : t("adminRateLimit.off")) : t("adminRateLimit.none"), tone: rl?.enabled ? "default" : "muted" },
                  ],
                },
              ]}
              actions={
                <>
                  <Button variant="outline" size="sm" onClick={() => setMembersTarget(g)}>
                    {t("adminGroups.manageMembers")}
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => setRateLimitTarget(g)}>
                    {t("adminRateLimit.action")}
                  </Button>
                </>
              }
            />
          );
        }}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminGroups.searchPlaceholder")}
            />
            <SegmentedControl<string>
              value={(list.filters.status as string) || "__all__"}
              onChange={(v) => list.setFilter("status", v === "__all__" ? undefined : v)}
              ariaLabel={t("adminCommon.status")}
              size="sm"
              options={[
                { value: "__all__", label: t("adminCommon.allStatuses") },
                ...ACCOUNT_GROUP_STATUSES.map((s) => ({ value: s, label: s })),
              ]}
            />
          </ListToolbar>
        }
        selection={{
          selected: list.selected,
          onToggle: list.toggle,
          onTogglePage: list.togglePage,
          bulkActions: (
            <>
              <Button
                variant="outline"
                size="sm"
                disabled={list.selected.size === 0}
                loading={batchMultipliers.isPending}
                onClick={() => setBulkMultiplierOpen(true)}
              >
                {t("adminGroups.bulkSetMultiplier")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={list.selected.size === 0}
                loading={batchRpm.isPending}
                onClick={() => setBulkRpmOpen(true)}
              >
                {t("adminGroups.bulkSetRpmOverride")}
              </Button>
            </>
          ),
        }}
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

      {bulkMultiplierOpen ? (
        <BulkGroupMultiplierDialog
          count={list.selected.size}
          isPending={batchMultipliers.isPending}
          onSubmit={async (multiplier) => {
            const ids = [...list.selected];
            try {
              const result = await batchMultipliers.mutateAsync(
                ids.map((id) => ({ group_id: id, multiplier })),
              );
              list.clearSelection();
              if (result.errors.length > 0) {
                toast({
                  title: t("feedback.batchPartial", {
                    succeeded: result.updated_count,
                    failed: result.errors.length,
                  }),
                  tone: "warning",
                });
              } else {
                toast({
                  title: t("feedback.batchAllSucceeded", { count: result.updated_count }),
                  tone: "success",
                });
              }
            } catch (err) {
              toast({
                title: t("feedback.failed"),
                description: adminErrorMessage(err),
                tone: "error",
              });
            }
            setBulkMultiplierOpen(false);
          }}
          onClose={() => setBulkMultiplierOpen(false)}
        />
      ) : null}

      {bulkRpmOpen ? (
        <BulkGroupRpmDialog
          count={list.selected.size}
          isPending={batchRpm.isPending}
          onSubmit={async (rpm) => {
            const ids = [...list.selected];
            try {
              const result = await batchRpm.mutateAsync(
                ids.map((id) => ({ group_id: id, rpm_override: rpm })),
              );
              list.clearSelection();
              if (result.errors.length > 0) {
                toast({
                  title: t("feedback.batchPartial", {
                    succeeded: result.updated_count,
                    failed: result.errors.length,
                  }),
                  tone: "warning",
                });
              } else {
                toast({
                  title: t("feedback.batchAllSucceeded", { count: result.updated_count }),
                  tone: "success",
                });
              }
            } catch (err) {
              toast({
                title: t("feedback.failed"),
                description: adminErrorMessage(err),
                tone: "error",
              });
            }
            setBulkRpmOpen(false);
          }}
          onClose={() => setBulkRpmOpen(false)}
        />
      ) : null}
    </>
  );
}

// Small focused dialog: decimal-string multiplier input.
function BulkGroupMultiplierDialog({
  count,
  isPending,
  onSubmit,
  onClose,
}: {
  count: number;
  isPending: boolean;
  onSubmit: (multiplier: string) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [value, setValue] = useState("1.0");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const trimmed = value.trim();
    const num = Number.parseFloat(trimmed);
    if (!Number.isFinite(num) || num <= 0) {
      setError(t("adminGroups.bulkSetMultiplierHint"));
      return;
    }
    void onSubmit(trimmed);
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t("adminGroups.bulkSetMultiplierTitle", { count })}</DialogTitle>
          </DialogHeader>
          <div className="mt-4 space-y-3">
            <div>
              <Label htmlFor="bulk-multiplier">{t("adminGroups.bulkSetMultiplier")}</Label>
              <p className="mb-1.5 text-xs text-srapi-text-tertiary">
                {t("adminGroups.bulkSetMultiplierHint")}
              </p>
              <Input
                id="bulk-multiplier"
                inputMode="decimal"
                placeholder="1.00000000"
                value={value}
                disabled={isPending}
                aria-invalid={Boolean(error)}
                onChange={(e) => setValue(e.target.value)}
              />
            </div>
            {error ? (
              <p role="alert" className="text-xs text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("common.apply")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// Small focused dialog: integer RPM input + "clear" checkbox.
function BulkGroupRpmDialog({
  count,
  isPending,
  onSubmit,
  onClose,
}: {
  count: number;
  isPending: boolean;
  onSubmit: (rpm: number | null) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [value, setValue] = useState("60");
  const [clear, setClear] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (clear) {
      void onSubmit(null);
      return;
    }
    const n = Number.parseInt(value.trim(), 10);
    if (!Number.isFinite(n) || n < 0) {
      setError(t("adminGroups.bulkSetRpmOverrideHint"));
      return;
    }
    void onSubmit(n);
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t("adminGroups.bulkSetRpmOverrideTitle", { count })}</DialogTitle>
          </DialogHeader>
          <div className="mt-4 space-y-3">
            <div>
              <Label htmlFor="bulk-rpm">{t("adminGroups.bulkSetRpmOverride")}</Label>
              <p className="mb-1.5 text-xs text-srapi-text-tertiary">
                {t("adminGroups.bulkSetRpmOverrideHint")}
              </p>
              <Input
                id="bulk-rpm"
                type="number"
                inputMode="numeric"
                min={0}
                disabled={isPending || clear}
                value={value}
                onChange={(e) => setValue(e.target.value)}
              />
              <label className="mt-2 flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={clear}
                  disabled={isPending}
                  onChange={(e) => setClear(e.target.checked)}
                />
                {t("adminGroups.bulkSetRpmOverrideClear")}
              </label>
            </div>
            {error ? (
              <p role="alert" className="text-xs text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("common.apply")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
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
              <p className="mb-1.5 text-xs text-srapi-text-tertiary">{t("adminGroups.providerScopeHint")}</p>
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
              <p className="mb-1.5 text-xs text-srapi-text-tertiary">{t("adminGroups.modelScopeHint")}</p>
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
              <p className="mb-1.5 text-xs text-srapi-text-tertiary">{t("adminGroups.strategyHint")}</p>
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
            <div>
              <Label htmlFor="g-rate-multiplier">{t("adminGroups.rateMultiplier")}</Label>
              <p className="mb-1.5 text-xs text-srapi-text-tertiary">
                {t("adminGroups.rateMultiplierHint")}
              </p>
              <Input
                id="g-rate-multiplier"
                inputMode="decimal"
                placeholder="1.00000000"
                value={form.rateMultiplier}
                onChange={(e) => set("rateMultiplier", e.target.value)}
              />
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
