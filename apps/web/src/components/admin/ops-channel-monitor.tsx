"use client";

import { useMemo, useState } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import { Gauge, Radar, FileText } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { StatCard } from "@/components/ui/stat-card";
import { Button } from "@/components/ui/button";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import {
  useAccountsAvailabilityWindows,
  AVAILABILITY_WINDOWS,
  type AccountAvailabilityWindows,
} from "@/hooks/use-accounts-availability-windows";
import {
  useAccountsAvailability,
  useChannelMonitors,
  useCreateChannelMonitor,
  useUpdateChannelMonitor,
  useDeleteChannelMonitor,
  useChannelMonitorTemplates,
  useCreateChannelMonitorTemplate,
  useUpdateChannelMonitorTemplate,
  useDeleteChannelMonitorTemplate,
  useApplyChannelMonitorTemplate,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatPercent, formatDateTime, formatLatency } from "@/lib/admin-format";
import { adminErrorMessage, type AdminListResult } from "@/lib/admin-api";
import { cn } from "@/lib/cn";
import {
  CHANNEL_MONITOR_SCOPES,
  CHANNEL_MONITOR_METHODS,
  emptyChannelMonitorForm,
  channelMonitorFormFromDefinition,
  buildChannelMonitorBody,
  emptyChannelMonitorTemplateForm,
  channelMonitorTemplateFormFromTemplate,
  buildChannelMonitorTemplateBody,
  type ChannelMonitorFormState,
  type ChannelMonitorTemplateFormState,
} from "@/lib/admin-channel-monitor-form";
import { ChannelMonitorRunDialog } from "@/components/features/channel-monitor-run-dialog";
import type {
  AccountAvailabilitySummary,
  ChannelMonitor,
  ChannelMonitorTemplate,
} from "@/lib/sdk-types";

const WINDOW_OPTIONS = [7, 14, 30, 90];

export function MonitorContent() {
  const { t } = useLanguage();
  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminMonitor.title")}
        description={t("adminMonitor.subtitle")}
      />
      <Tabs defaultValue="monitors">
        <TabsList>
          <TabsTrigger value="monitors">{t("adminMonitor.tabMonitors")}</TabsTrigger>
          <TabsTrigger value="templates">{t("adminMonitor.tabTemplates")}</TabsTrigger>
          <TabsTrigger value="availability">{t("adminMonitor.tabAvailability")}</TabsTrigger>
        </TabsList>
        <TabsContent value="monitors" className="mt-4">
          <MonitorsTab />
        </TabsContent>
        <TabsContent value="templates" className="mt-4">
          <TemplatesTab />
        </TabsContent>
        <TabsContent value="availability" className="mt-4">
          <AvailabilityTab />
        </TabsContent>
      </Tabs>
    </>
  );
}

// ---- Monitors tab ----

function monitorMatch(row: ChannelMonitor, term: string): boolean {
  if (!term) return true;
  return [row.name, row.scope, row.scope_ref, row.model]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const monitorCompare = (a: ChannelMonitor, b: ChannelMonitor) => a.name.localeCompare(b.name);

function MonitorsTab() {
  const { t } = useLanguage();
  const list = useAdminList();
  const all = useChannelMonitors();
  const { query, total } = useClientPagedList(all, list, {
    match: monitorMatch,
    compare: monitorCompare,
  });
  const createMut = useCreateChannelMonitor();
  const updateMut = useUpdateChannelMonitor();
  const deleteMut = useDeleteChannelMonitor();

  const colVis = useColumnVisibility("admin-channel-monitors", []);
  const [formTarget, setFormTarget] = useState<ChannelMonitor | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ChannelMonitor | null>(null);
  const [runTarget, setRunTarget] = useState<ChannelMonitor | null>(null);
  const isNew = formTarget === "new";
  const isFiltered = Boolean(list.search);

  const scopeLabel = (scope: ChannelMonitor["scope"]) =>
    t(`adminMonitor.scope.${scope}` as const);

  const fields: FieldConfig<ChannelMonitorFormState>[] = [
    { name: "name", label: t("adminMonitor.name"), required: true },
    {
      name: "scope",
      label: t("adminMonitor.scope.label"),
      type: "select",
      options: CHANNEL_MONITOR_SCOPES.map((value) => ({ value, label: scopeLabel(value) })),
    },
    {
      name: "scopeRef",
      label: t("adminMonitor.scopeRef"),
      hint: t("adminMonitor.scopeRefHint"),
    },
    { name: "enabled", label: t("adminMonitor.enabled"), type: "switch" },
    {
      name: "intervalSeconds",
      label: t("adminMonitor.interval"),
      type: "number",
      hint: t("adminMonitor.intervalHint"),
    },
    { name: "model", label: t("adminMonitor.model"), hint: t("adminMonitor.modelHint") },
    {
      name: "method",
      label: t("adminMonitor.method"),
      type: "select",
      advanced: true,
      options: CHANNEL_MONITOR_METHODS.map((value) => ({
        value,
        label: value || t("adminMonitor.methodDefault"),
      })),
    },
    {
      name: "url",
      label: t("adminMonitor.url"),
      advanced: true,
      hint: t("adminMonitor.urlHint"),
    },
    { name: "headers", label: t("adminMonitor.headers"), type: "keyvalue", advanced: true },
    {
      name: "body",
      label: t("adminMonitor.body"),
      type: "json",
      advanced: true,
      hint: t("adminMonitor.bodyHint"),
    },
    {
      name: "expectedStatusCodes",
      label: t("adminMonitor.expectedStatus"),
      type: "tags",
      advanced: true,
      placeholder: "200",
    },
    {
      name: "responseJsonPath",
      label: t("adminMonitor.responseJsonPath"),
      advanced: true,
      hint: t("adminMonitor.responseJsonPathHint"),
    },
    {
      name: "responseContains",
      label: t("adminMonitor.responseContains"),
      advanced: true,
    },
  ];

  const columns: Column<ChannelMonitor>[] = [
    {
      key: "name",
      header: t("adminMonitor.name"),
      pinned: true,
      render: (r) => <span className="text-srapi-text-primary">{r.name}</span>,
    },
    {
      key: "scope",
      header: t("adminMonitor.scope.label"),
      render: (r) => (
        <span className="text-srapi-text-secondary">
          {scopeLabel(r.scope)}
          {r.scope_ref ? (
            <span className="ml-1 font-mono text-2xs text-srapi-text-tertiary">{r.scope_ref}</span>
          ) : null}
        </span>
      ),
    },
    {
      key: "model",
      header: t("adminMonitor.model"),
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{r.model || "—"}</span>
      ),
    },
    {
      key: "interval",
      header: t("adminMonitor.interval"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {r.interval_seconds}s
        </span>
      ),
    },
    {
      key: "last_run",
      header: t("adminMonitor.lastRun"),
      hideOnMobile: true,
      render: (r) => {
        if (!r.last_run_at) {
          return <span className="text-2xs text-srapi-text-tertiary">{t("adminMonitor.neverRun")}</span>;
        }
        const isOK = r.last_run_ok ?? false;
        return (
          <div className="flex flex-col gap-0.5">
            <div className="flex items-center gap-1.5">
              <span
                className={
                  "inline-block h-1.5 w-1.5 rounded-full " +
                  (isOK ? "bg-srapi-success" : "bg-srapi-error")
                }
                aria-hidden
              />
              <span className="font-mono text-2xs text-srapi-text-tertiary">
                {formatDateTime(r.last_run_at)}
              </span>
            </div>
            {typeof r.last_run_latency_ms === "number" ? (
              <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
                {formatLatency(r.last_run_latency_ms)}
              </span>
            ) : null}
          </div>
        );
      },
    },
    {
      key: "enabled",
      header: t("adminMonitor.enabled"),
      render: (r) => (
        <QuietBadge
          status={r.enabled ? "active" : "disabled"}
          label={r.enabled ? t("common.active") : t("common.disabled")}
        />
      ),
    },
  ];

  return (
    <>
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(r) => String(r.id)}
        emptyIcon={Radar}
        emptyTitle={t("adminMonitor.emptyMonitorsTitle")}
        emptyBody={t("adminMonitor.emptyMonitorsBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminMonitor.create")}
          </Button>
        }
        minWidth={720}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminMonitor.searchPlaceholder")}
            />
            <div className="ml-auto flex items-center gap-3">
              {all.data ? <ListCount total={total} /> : null}
              <ColumnToggle columns={columns} visibility={colVis} />
              <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
                ＋ {t("adminMonitor.create")}
              </Button>
            </div>
          </ListToolbar>
        }
        columnVisibility={colVis}
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(r) => (
          <RowActionsMenu
            actions={[
              { label: t("adminMonitor.runNow"), onSelect: () => setRunTarget(r) },
              { label: t("common.edit"), onSelect: () => setFormTarget(r) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(r) },
            ]}
          />
        )}
      />

      {formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={isNew ? t("adminMonitor.create") : t("adminMonitor.edit")}
          fields={fields}
          initial={isNew ? emptyChannelMonitorForm() : channelMonitorFormFromDefinition(formTarget)}
          buildBody={buildChannelMonitorBody}
          submit={
            isNew
              ? (body) => createMut.mutateAsync(body)
              : (body) => updateMut.mutateAsync({ id: String(formTarget.id), body })
          }
          successMessage={isNew ? t("feedback.created") : t("feedback.updated")}
          isPending={createMut.isPending || updateMut.isPending}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("feedback.confirmDeleteTitle", { name: deleteTarget.name })}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("common.delete")}
          confirmPhrase={deleteTarget.name}
          onConfirm={() => deleteMut.mutateAsync(String(deleteTarget.id))}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
        />
      ) : null}

      {runTarget ? (
        <ChannelMonitorRunDialog
          open
          onOpenChange={(open) => {
            if (!open) setRunTarget(null);
          }}
          monitorId={String(runTarget.id)}
          monitorName={runTarget.name}
        />
      ) : null}
    </>
  );
}

// ---- Templates tab ----

function templateMatch(row: ChannelMonitorTemplate, term: string): boolean {
  if (!term) return true;
  return [row.name, row.description].filter(Boolean).join(" ").toLowerCase().includes(term);
}

const templateCompare = (a: ChannelMonitorTemplate, b: ChannelMonitorTemplate) =>
  a.name.localeCompare(b.name);

function TemplatesTab() {
  const { t } = useLanguage();
  const list = useAdminList();
  const all = useChannelMonitorTemplates();
  const monitors = useChannelMonitors();
  const { query, total } = useClientPagedList(all, list, {
    match: templateMatch,
    compare: templateCompare,
  });
  const createMut = useCreateChannelMonitorTemplate();
  const updateMut = useUpdateChannelMonitorTemplate();
  const deleteMut = useDeleteChannelMonitorTemplate();
  const applyMut = useApplyChannelMonitorTemplate();

  const colVis = useColumnVisibility("admin-channel-templates", []);
  const [formTarget, setFormTarget] = useState<ChannelMonitorTemplate | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ChannelMonitorTemplate | null>(null);
  const [applyTarget, setApplyTarget] = useState<ChannelMonitorTemplate | null>(null);
  const isNew = formTarget === "new";

  const fields: FieldConfig<ChannelMonitorTemplateFormState>[] = [
    { name: "name", label: t("adminMonitor.templateName"), required: true },
    { name: "description", label: t("adminMonitor.templateDescription") },
    {
      name: "method",
      label: t("adminMonitor.method"),
      type: "select",
      options: CHANNEL_MONITOR_METHODS.map((value) => ({
        value,
        label: value || t("adminMonitor.methodDefault"),
      })),
    },
    { name: "url", label: t("adminMonitor.url"), hint: t("adminMonitor.urlHint") },
    { name: "headers", label: t("adminMonitor.headers"), type: "keyvalue" },
    { name: "body", label: t("adminMonitor.body"), type: "json", hint: t("adminMonitor.bodyHint") },
    {
      name: "expectedStatusCodes",
      label: t("adminMonitor.expectedStatus"),
      type: "tags",
      placeholder: "200",
    },
    {
      name: "responseJsonPath",
      label: t("adminMonitor.responseJsonPath"),
      hint: t("adminMonitor.responseJsonPathHint"),
    },
    { name: "responseContains", label: t("adminMonitor.responseContains") },
  ];

  const columns: Column<ChannelMonitorTemplate>[] = [
    {
      key: "name",
      header: t("adminMonitor.templateName"),
      pinned: true,
      render: (r) => <span className="text-srapi-text-primary">{r.name}</span>,
    },
    {
      key: "description",
      header: t("adminMonitor.templateDescription"),
      hideOnMobile: true,
      render: (r) => (
        <span className="text-srapi-text-secondary">{r.description || "—"}</span>
      ),
    },
    {
      key: "method",
      header: t("adminMonitor.method"),
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {r.request?.method || t("adminMonitor.methodDefault")}
        </span>
      ),
    },
  ];

  return (
    <>
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(r) => String(r.id)}
        emptyIcon={FileText}
        emptyTitle={t("adminMonitor.emptyTemplatesTitle")}
        emptyBody={t("adminMonitor.emptyTemplatesBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminMonitor.createTemplate")}
          </Button>
        }
        minWidth={620}
        isFiltered={Boolean(list.search)}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminMonitor.templateSearchPlaceholder")}
            />
            <div className="ml-auto flex items-center gap-3">
              {all.data ? <ListCount total={total} /> : null}
              <ColumnToggle columns={columns} visibility={colVis} />
              <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
                ＋ {t("adminMonitor.createTemplate")}
              </Button>
            </div>
          </ListToolbar>
        }
        columnVisibility={colVis}
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(r) => (
          <RowActionsMenu
            actions={[
              { label: t("adminMonitor.apply"), onSelect: () => setApplyTarget(r) },
              { label: t("common.edit"), onSelect: () => setFormTarget(r) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(r) },
            ]}
          />
        )}
      />

      {formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={isNew ? t("adminMonitor.createTemplate") : t("adminMonitor.editTemplate")}
          fields={fields}
          initial={
            isNew
              ? emptyChannelMonitorTemplateForm()
              : channelMonitorTemplateFormFromTemplate(formTarget)
          }
          buildBody={buildChannelMonitorTemplateBody}
          submit={
            isNew
              ? (body) => createMut.mutateAsync(body)
              : (body) => updateMut.mutateAsync({ id: String(formTarget.id), body })
          }
          successMessage={isNew ? t("feedback.created") : t("feedback.updated")}
          isPending={createMut.isPending || updateMut.isPending}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("feedback.confirmDeleteTitle", { name: deleteTarget.name })}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("common.delete")}
          confirmPhrase={deleteTarget.name}
          onConfirm={() => deleteMut.mutateAsync(String(deleteTarget.id))}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
        />
      ) : null}

      {applyTarget ? (
        <ApplyTemplateDialog
          template={applyTarget}
          monitors={monitors.data?.data ?? []}
          isPending={applyMut.isPending}
          onClose={() => setApplyTarget(null)}
          onApply={(monitorIds) =>
            applyMut.mutateAsync({ id: String(applyTarget.id), monitorIds })
          }
        />
      ) : null}
    </>
  );
}

function ApplyTemplateDialog({
  template,
  monitors,
  isPending,
  onClose,
  onApply,
}: {
  template: ChannelMonitorTemplate;
  monitors: ChannelMonitor[];
  isPending: boolean;
  onClose: () => void;
  onApply: (monitorIds: number[]) => Promise<unknown>;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const [selected, setSelected] = useState<number[]>([]);
  const toggle = (id: number) =>
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));

  async function handleApply() {
    try {
      await onApply(selected);
      toast({ title: t("feedback.updated"), tone: "success" });
      onClose();
    } catch (err) {
      toast({ title: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("adminMonitor.applyTitle", { name: template.name })}</DialogTitle>
          <DialogDescription>{t("adminMonitor.applyBody")}</DialogDescription>
        </DialogHeader>
        {monitors.length === 0 ? (
          <p className="text-sm text-srapi-text-secondary">{t("adminMonitor.applyNoMonitors")}</p>
        ) : (
          <div className="max-h-56 space-y-1 overflow-y-auto rounded-lg border border-srapi-border p-2">
            {monitors.map((monitor) => (
              <label
                key={monitor.id}
                className="flex cursor-pointer items-center gap-2 rounded px-1.5 py-1 hover:bg-srapi-card-muted"
              >
                <input
                  type="checkbox"
                  checked={selected.includes(Number(monitor.id))}
                  onChange={() => toggle(Number(monitor.id))}
                />
                <span className="text-sm text-srapi-text-primary">{monitor.name}</span>
              </label>
            ))}
          </div>
        )}
        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={handleApply}
            loading={isPending}
            disabled={selected.length === 0}
          >
            {t("adminMonitor.apply")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ---- Availability tab ----
//
// Two views share this tab. The multi-window view (default) fans out parallel
// availability queries across every window in WINDOW_OPTIONS and renders one
// uptime column per window, so an operator compares 7/14/30/90d side by side
// without re-querying. The single-window view is the original rollup, kept as a
// simpler fallback behind a small mode toggle.

const UPTIME_WARN_THRESHOLD = 0.95;

function uptimeTone(uptime: number | undefined): string {
  if (uptime == null) return "text-srapi-text-tertiary";
  return uptime < UPTIME_WARN_THRESHOLD ? "text-srapi-error" : "text-srapi-text-secondary";
}

type AvailabilityMode = "windows" | "single";

function AvailabilityTab() {
  const { t } = useLanguage();
  // The toggle lives at the tab level so both views unmount cleanly when the
  // operator switches; each view owns its own list/search/pagination state.
  const [mode, setMode] = useState<AvailabilityMode>("windows");

  // Labels compose from existing translation keys so we don't introduce new
  // i18n entries here: "All · Window" vs "Window".
  const toggle = (
    <div className="flex items-center gap-1 rounded-lg border border-srapi-border p-0.5">
      <Button
        variant={mode === "windows" ? "outline" : "ghost"}
        size="sm"
        onClick={() => setMode("windows")}
      >
        {t("common.all")} · {t("adminMonitor.window")}
      </Button>
      <Button
        variant={mode === "single" ? "outline" : "ghost"}
        size="sm"
        onClick={() => setMode("single")}
      >
        {t("adminMonitor.window")}
      </Button>
    </div>
  );

  return mode === "windows" ? (
    <MultiWindowAvailabilityTab modeToggle={toggle} />
  ) : (
    <SingleWindowAvailabilityTab modeToggle={toggle} />
  );
}

// ---- Multi-window view (parallel windows, simultaneous uptime columns) ----

function windowsMatch(row: AccountAvailabilityWindows, term: string): boolean {
  if (!term) return true;
  return [row.account_name, row.status].filter(Boolean).join(" ").toLowerCase().includes(term);
}

/** Worst-first by the widest resolved window (falling back to any window). */
function worstUptime(row: AccountAvailabilityWindows): number {
  for (let i = AVAILABILITY_WINDOWS.length - 1; i >= 0; i--) {
    const u = row.uptime[AVAILABILITY_WINDOWS[i]];
    if (u != null) return u;
  }
  return 1;
}

function windowsCompare(a: AccountAvailabilityWindows, b: AccountAvailabilityWindows): number {
  return worstUptime(a) - worstUptime(b) || a.account_name.localeCompare(b.account_name);
}

function MultiWindowAvailabilityTab({ modeToggle }: { modeToggle: React.ReactNode }) {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-channel-availability-windows", []);
  const [detail, setDetail] = useState<AccountAvailabilityWindows | null>(null);

  const availability = useAccountsAvailabilityWindows();

  // Adapt the merged multi-query result into the UseQueryResult shape AdminListView
  // expects, so loading/error/empty/skeleton + the table chrome stay identical to
  // every other admin list — only the data source differs.
  const query = useMemo(
    () =>
      ({
        data: { data: availability.rows } as AdminListResult<AccountAvailabilityWindows>,
        isLoading: availability.isPending,
        isError: availability.error != null,
        isPending: availability.isPending,
        isFetching: availability.isFetching,
        error: availability.error,
        refetch: availability.refetch,
      }) as unknown as UseQueryResult<AdminListResult<AccountAvailabilityWindows>>,
    [availability],
  );

  const { query: pagedQuery, total } = useClientPagedList(query, list, {
    match: windowsMatch,
    compare: windowsCompare,
  });

  const columns: Column<AccountAvailabilityWindows>[] = [
    {
      key: "account",
      header: t("adminMonitor.account"),
      pinned: true,
      render: (r) => (
        <button
          type="button"
          onClick={() => setDetail(r)}
          className="text-left text-srapi-text-primary underline-offset-4 hover:underline"
        >
          {r.account_name}
        </button>
      ),
    },
    {
      key: "status",
      header: t("adminMonitor.status"),
      render: (r) => (
        <QuietBadge status={quietStatusFor(r.status)} label={statusLabel(t, r.status)} />
      ),
    },
    // One uptime column per window, rendered simultaneously.
    ...AVAILABILITY_WINDOWS.map(
      (d): Column<AccountAvailabilityWindows> => ({
        key: `uptime-${d}`,
        header: t("adminMonitor.windowDays", { days: d }),
        align: "right",
        render: (r) => (
          <span className={cn("font-mono tabular", uptimeTone(r.uptime[d]))}>
            {r.uptime[d] == null ? "—" : formatPercent(r.uptime[d])}
          </span>
        ),
      }),
    ),
    {
      key: "checked",
      header: t("adminMonitor.lastChecked"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.last_checked_at)}
        </span>
      ),
    },
  ];

  return (
    <>
      <AdminListView
        query={pagedQuery}
        columns={columns}
        getRowId={(r) => String(r.account_id)}
        emptyIcon={Gauge}
        emptyTitle={t("adminMonitor.emptyTitle")}
        emptyBody={t("adminMonitor.emptyBody")}
        minWidth={720}
        columnVisibility={colVis}
        isFiltered={Boolean(list.search)}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminMonitor.searchPlaceholder")}
            />
            <div className="ml-auto flex items-center gap-3">
              {availability.isPending ? null : <ListCount total={total} />}
              <ColumnToggle columns={columns} visibility={colVis} />
              {modeToggle}
            </div>
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(r) => (
          <RowActionsMenu
            actions={[
              { label: `${t("common.show")} · ${t("adminMonitor.uptime")}`, onSelect: () => setDetail(r) },
            ]}
          />
        )}
      />

      {detail ? (
        <AvailabilityDetailDialog account={detail} onClose={() => setDetail(null)} />
      ) : null}
    </>
  );
}

function AvailabilityDetailDialog({
  account,
  onClose,
}: {
  account: AccountAvailabilityWindows;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{account.account_name}</DialogTitle>
          <DialogDescription className="flex items-center gap-2">
            <QuietBadge
              status={quietStatusFor(account.status)}
              label={statusLabel(t, account.status)}
            />
            <span className="font-mono text-2xs text-srapi-text-tertiary">
              {formatDateTime(account.last_checked_at)}
            </span>
          </DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {AVAILABILITY_WINDOWS.map((d) => {
            const uptime = account.uptime[d];
            return (
              <StatCard
                key={d}
                label={t("adminMonitor.windowDays", { days: d })}
                value={uptime == null ? "—" : formatPercent(uptime)}
                className={uptime != null && uptime < UPTIME_WARN_THRESHOLD ? "ring-1 ring-srapi-error/40" : undefined}
              />
            );
          })}
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ---- Single-window view (the original rollup, preserved as a fallback) ----

function availabilityMatch(row: AccountAvailabilitySummary, term: string): boolean {
  if (!term) return true;
  return [row.account_name, row.status].filter(Boolean).join(" ").toLowerCase().includes(term);
}

const availabilityCompare = (a: AccountAvailabilitySummary, b: AccountAvailabilitySummary) =>
  a.overall_uptime - b.overall_uptime || a.account_name.localeCompare(b.account_name);

function SingleWindowAvailabilityTab({ modeToggle }: { modeToggle: React.ReactNode }) {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-channel-availability", []);
  const [days, setDays] = useState(7);
  const all = useAccountsAvailability(days);
  const { query, total } = useClientPagedList(all, list, {
    match: availabilityMatch,
    compare: availabilityCompare,
  });

  const columns: Column<AccountAvailabilitySummary>[] = [
    {
      key: "account",
      header: t("adminMonitor.account"),
      pinned: true,
      render: (r) => <span className="text-srapi-text-primary">{r.account_name}</span>,
    },
    {
      key: "status",
      header: t("adminMonitor.status"),
      render: (r) => <QuietBadge status={quietStatusFor(r.status)} label={statusLabel(t, r.status)} />,
    },
    {
      key: "uptime",
      header: t("adminMonitor.uptime"),
      align: "right",
      render: (r) => (
        <span
          className={cn(
            "font-mono tabular",
            r.overall_uptime < 0.95 ? "text-srapi-error" : "text-srapi-text-secondary",
          )}
        >
          {formatPercent(r.overall_uptime)}
        </span>
      ),
    },
    {
      key: "checked",
      header: t("adminMonitor.lastChecked"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.last_checked_at)}
        </span>
      ),
    },
  ];

  return (
    <AdminListView
      query={query}
      columns={columns}
      getRowId={(r) => String(r.account_id)}
      emptyIcon={Gauge}
      emptyTitle={t("adminMonitor.emptyTitle")}
      emptyBody={t("adminMonitor.emptyBody")}
      minWidth={640}
      columnVisibility={colVis}
      isFiltered={Boolean(list.search)}
      onClearFilters={list.clearFilters}
      toolbar={
        <ListToolbar>
          <SearchInput
            value={list.searchInput}
            onChange={list.setSearchInput}
            placeholder={t("adminMonitor.searchPlaceholder")}
          />
          <div className="ml-auto flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle columns={columns} visibility={colVis} />
            <Select
              value={String(days)}
              onValueChange={(v) => {
                setDays(Number(v));
                list.setPage(1);
              }}
            >
              <SelectTrigger className="h-9 w-auto min-w-[7rem] gap-2 rounded-lg">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WINDOW_OPTIONS.map((d) => (
                  <SelectItem key={d} value={String(d)}>
                    {t("adminMonitor.windowDays", { days: d })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {modeToggle}
          </div>
        </ListToolbar>
      }
      pagination={{
        page: list.page,
        pageSize: list.pageSize,
        total,
        onPageChange: list.setPage,
      }}
    />
  );
}
