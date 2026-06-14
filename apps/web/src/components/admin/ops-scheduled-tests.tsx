"use client";

import { useState } from "react";
import { CalendarClock } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import {
  useScheduledTestPlans,
  useScheduledTestPlanRuns,
  useCreateScheduledTestPlan,
  useUpdateScheduledTestPlan,
  useDeleteScheduledTestPlan,
  useRunScheduledTestPlan,
} from "@/hooks/admin-queries";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatDateTime } from "@/lib/admin-format";
import { useLanguage } from "@/context/LanguageContext";
import {
  SCHEDULED_TEST_SCOPES,
  emptyScheduledTestForm,
  scheduledTestFormFromPlan,
  buildScheduledTestBody,
  type ScheduledTestFormState,
} from "@/lib/admin-scheduled-test-form";
import type { ScheduledTestPlan } from "@/lib/sdk-types";

const STATUS_TONE: Record<string, QuietStatus> = {
  ok: "active",
  warning: "limited",
  partial: "limited",
  failed: "error",
};

function planMatch(
  plan: ScheduledTestPlan,
  term: string,
  filters: Record<string, string>,
): boolean {
  if (filters.scope_type && plan.scope_type !== filters.scope_type) return false;
  if (!term) return true;
  return [plan.name, plan.scope_type, plan.cron_expression, plan.probe_model]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const planCompare = (a: ScheduledTestPlan, b: ScheduledTestPlan) =>
  a.name.localeCompare(b.name) || a.id - b.id;

export function ScheduledTestsContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-scheduled-tests", ["lastRun"]);
  const all = useScheduledTestPlans();
  const { query, total } = useClientPagedList(all, list, { match: planMatch, compare: planCompare });

  const createMut = useCreateScheduledTestPlan();
  const updateMut = useUpdateScheduledTestPlan();
  const deleteMut = useDeleteScheduledTestPlan();
  const runMut = useRunScheduledTestPlan();

  const [formTarget, setFormTarget] = useState<ScheduledTestPlan | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ScheduledTestPlan | null>(null);
  const [historyTarget, setHistoryTarget] = useState<ScheduledTestPlan | null>(null);
  const isNew = formTarget === "new";
  const isFiltered = Boolean(list.search || list.filters.scope_type);

  const scopeLabel = (scope: ScheduledTestPlan["scope_type"]) =>
    t(`adminScheduledTests.scope_${scope}`);

  const statusLabel = (status: string) =>
    status ? t(`adminScheduledTests.status_${status}`) : t("adminScheduledTests.never");

  async function runNow(plan: ScheduledTestPlan) {
    try {
      await runMut.mutateAsync(String(plan.id));
      toast({ title: t("adminScheduledTests.runStarted"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const fields: FieldConfig<ScheduledTestFormState>[] = [
    { name: "name", label: t("adminScheduledTests.name") },
    {
      name: "scopeType",
      label: t("adminScheduledTests.scope"),
      type: "select",
      options: SCHEDULED_TEST_SCOPES.map((value) => ({ value, label: scopeLabel(value) })),
      hint: t("adminScheduledTests.scopeHint"),
    },
    {
      name: "scopeId",
      label: t("adminScheduledTests.scopeId"),
      type: "number",
      hint: t("adminScheduledTests.scopeIdHint"),
    },
    {
      name: "intervalSeconds",
      label: t("adminScheduledTests.interval"),
      type: "number",
      hint: t("adminScheduledTests.intervalHint"),
    },
    {
      name: "cronExpression",
      label: t("adminScheduledTests.cron"),
      placeholder: "0 * * * *",
      hint: t("adminScheduledTests.cronHint"),
    },
    {
      name: "probeModel",
      label: t("adminScheduledTests.probeModel"),
      placeholder: "gpt-4o-mini",
      hint: t("adminScheduledTests.probeModelHint"),
    },
    {
      name: "maxResults",
      label: t("adminScheduledTests.maxResults"),
      type: "number",
      hint: t("adminScheduledTests.maxResultsHint"),
    },
    {
      name: "autoRecover",
      label: t("adminScheduledTests.autoRecover"),
      type: "switch",
      hint: t("adminScheduledTests.autoRecoverHint"),
    },
    { name: "enabled", label: t("adminScheduledTests.enabled"), type: "switch" },
  ];

  const columns: Column<ScheduledTestPlan>[] = [
    {
      key: "name",
      header: t("adminScheduledTests.name"),
      pinned: true,
      render: (p) => <span className="text-srapi-text-primary">{p.name}</span>,
    },
    {
      key: "scope",
      header: t("adminScheduledTests.scope"),
      render: (p) => (
        <span className="text-srapi-text-secondary">
          {scopeLabel(p.scope_type)}
          {p.scope_id != null ? (
            <span className="ml-1 font-mono text-2xs text-srapi-text-tertiary">#{p.scope_id}</span>
          ) : null}
        </span>
      ),
    },
    {
      key: "interval",
      header: t("adminScheduledTests.interval"),
      align: "right",
      hideOnMobile: true,
      render: (p) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {p.interval_seconds}s
        </span>
      ),
    },
    {
      key: "probeModel",
      header: t("adminScheduledTests.probeModel"),
      hideOnMobile: true,
      render: (p) =>
        p.probe_model ? (
          <span className="font-mono text-2xs text-srapi-text-secondary">{p.probe_model}</span>
        ) : (
          <QuietBadge status="limited" label={t("adminScheduledTests.metadataProbeModel")} />
        ),
    },
    {
      key: "autoRecover",
      header: t("adminScheduledTests.autoRecover"),
      hideOnMobile: true,
      render: (p) =>
        p.auto_recover ? (
          <QuietBadge status="active" label={t("common.active")} />
        ) : (
          <span className="text-srapi-text-tertiary">—</span>
        ),
    },
    {
      key: "lastRun",
      header: t("adminScheduledTests.lastRun"),
      hideOnMobile: true,
      render: (p) =>
        p.last_run_at ? (
          <span className="text-2xs text-srapi-text-tertiary">{formatDateTime(p.last_run_at)}</span>
        ) : (
          <span className="text-srapi-text-tertiary">{t("adminScheduledTests.never")}</span>
        ),
    },
    {
      key: "status",
      header: t("adminScheduledTests.lastStatus"),
      render: (p) =>
        p.last_status ? (
          <QuietBadge status={STATUS_TONE[p.last_status] ?? "disabled"} label={statusLabel(p.last_status)} />
        ) : (
          // No run yet — the "Last status" column must say "never ran", not the
          // test's enabled/disabled config state.
          <QuietBadge status="disabled" label={t("adminScheduledTests.never")} />
        ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminScheduledTests.title")}
        description={t("adminScheduledTests.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminScheduledTests.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => String(p.id)}
        emptyIcon={CalendarClock}
        emptyTitle={t("adminScheduledTests.emptyTitle")}
        emptyBody={t("adminScheduledTests.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminScheduledTests.create")}
          </Button>
        }
        minWidth={760}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminScheduledTests.searchPlaceholder")}
            />
            <FilterSelect
              value={list.filters.scope_type}
              onChange={(v) => list.setFilter("scope_type", v)}
              options={SCHEDULED_TEST_SCOPES.map((value) => ({ value, label: scopeLabel(value) }))}
              allLabel={t("adminScheduledTests.scope")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              {
                label: t("adminScheduledTests.runNow"),
                onSelect: () => void runNow(p),
              },
              {
                label: t("adminScheduledTests.runHistory"),
                onSelect: () => setHistoryTarget(p),
              },
              { label: t("common.edit"), onSelect: () => setFormTarget(p) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(p) },
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
          title={isNew ? t("adminScheduledTests.create") : t("adminScheduledTests.edit")}
          fields={fields}
          initial={isNew ? emptyScheduledTestForm() : scheduledTestFormFromPlan(formTarget)}
          buildBody={buildScheduledTestBody}
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

      {historyTarget ? (
        <RunHistoryDialog plan={historyTarget} onClose={() => setHistoryTarget(null)} />
      ) : null}
    </>
  );
}

function RunHistoryDialog({
  plan,
  onClose,
}: {
  plan: ScheduledTestPlan;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const runs = useScheduledTestPlanRuns(String(plan.id));
  const triggerLabel = (trigger: string) =>
    t(trigger === "manual" ? "adminScheduledTests.trigger_manual" : "adminScheduledTests.trigger_schedule");
  const statusLabel = (status: string) =>
    status ? t(`adminScheduledTests.status_${status}`) : status;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("adminScheduledTests.runHistoryFor", { name: plan.name })}</DialogTitle>
          <DialogDescription>{t("adminScheduledTests.subtitle")}</DialogDescription>
        </DialogHeader>
        <div className="mt-2 max-h-[60vh] overflow-auto">
          {runs.data && runs.data.data.length > 0 ? (
            <table className="w-full text-left text-xs">
              <thead className="text-2xs uppercase tracking-wide text-srapi-text-tertiary">
                <tr>
                  <th className="py-1 pr-3">{t("adminScheduledTests.startedAt")}</th>
                  <th className="py-1 pr-3">{t("adminScheduledTests.lastStatus")}</th>
                  <th className="py-1 pr-3">{t("adminScheduledTests.summary")}</th>
                </tr>
              </thead>
              <tbody className="text-srapi-text-secondary">
                {runs.data.data.map((run) => (
                  <tr key={run.id} className="border-t border-srapi-border/60">
                    <td className="py-1.5 pr-3 align-top">
                      <div className="text-srapi-text-primary">{formatDateTime(run.started_at)}</div>
                      <div className="text-2xs text-srapi-text-tertiary">{triggerLabel(run.trigger)}</div>
                    </td>
                    <td className="py-1.5 pr-3 align-top">
                      <QuietBadge
                        status={STATUS_TONE[run.status] ?? "disabled"}
                        label={statusLabel(run.status)}
                      />
                    </td>
                    <td className="py-1.5 pr-3 align-top font-mono text-2xs text-srapi-text-tertiary">
                      {run.summary || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <div className="py-10 text-center">
              <p className="text-sm text-srapi-text-secondary">
                {t("adminScheduledTests.runsEmptyTitle")}
              </p>
              <p className="mt-1 text-xs text-srapi-text-tertiary">
                {t("adminScheduledTests.runsEmptyBody")}
              </p>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
