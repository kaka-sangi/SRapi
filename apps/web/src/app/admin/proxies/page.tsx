"use client";

import { useState } from "react";
import { Network } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminProxies,
  useCreateProxy,
  useUpdateProxy,
  useDeleteProxy,
  useTestProxy,
  useBatchTestProxies,
  useBatchDeleteProxies,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import {
  PROXY_TYPES,
  PROXY_STATUSES,
  PROXY_FALLBACK_MODES,
  PROXY_BACKUP_NONE,
  emptyProxyForm,
  proxyFormFromProxy,
  buildCreateProxyBody,
  buildUpdateProxyBody,
  COUNTRY_NONE,
  type ProxyFormState,
} from "@/lib/admin-proxy-form";
import type { ProxyDefinition } from "@/lib/sdk-types";
import { formatDateTime, formatLatency } from "@/lib/admin-format";
import { countryOptions } from "@/lib/countries";

export default function AdminProxiesPage() {
  return (
    <AdminShell>
      <ProxiesContent />
    </AdminShell>
  );
}

function ProxiesContent() {
  const { t, language } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-proxies", []);
  const statusFilter = (list.filters.status as ProxyDefinition["status"]) || undefined;
  const countryFilter = (list.filters.country as string | undefined) || undefined;
  // Memoise the localized {value,label} pairs once per render and resolve a
  // single code → label lookup so the table and the form share the exact same
  // localized display name.
  const countrySelectOptions = countryOptions(language);
  const countryLabelByCode = new Map(countrySelectOptions.map((o) => [o.value, o.label]));
  const localizedCountryName = (code: string | null | undefined, fallback: string | null | undefined): string => {
    const c = (code ?? "").trim().toUpperCase();
    if (!c) return fallback?.trim() || "";
    const labeled = countryLabelByCode.get(c);
    if (labeled) {
      // Strip the trailing " (XX)" suffix that countryOptions adds for the
      // form picker — the table already shows the code in a separate font-mono
      // hint, so the cell stays terse.
      return labeled.replace(/\s+\([A-Z]{2}\)$/, "");
    }
    return fallback?.trim() || c;
  };
  const proxies = useAdminProxies({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
  });
  // Country filter is applied client-side because the API list endpoint takes
  // status only — adding a server-side filter would be a separate contract
  // change. Pagination keeps the server total so the operator still sees a
  // truthful row count; filtering only narrows the visible page.
  const filteredProxies = countryFilter && proxies.data
    ? {
        ...proxies,
        data: {
          ...proxies.data,
          data: proxies.data.data.filter(
            (p) => (p.country_code ?? "").trim().toUpperCase() === countryFilter.trim().toUpperCase(),
          ),
        },
      }
    : proxies;
  const createMut = useCreateProxy();
  const updateMut = useUpdateProxy();
  const deleteMut = useDeleteProxy();
  const testMut = useTestProxy();
  const batchTestMut = useBatchTestProxies();

  async function runTest(id: string) {
    try {
      const result = await testMut.mutateAsync({ id });
      if (result.ok) {
        toast({
          title: t("adminProxies.testOk", { latency: result.latency_ms }),
          description: t("adminProxies.testTarget", { target: result.target_url }),
          tone: "success",
        });
      } else {
        toast({
          title: t("adminProxies.testFailed", {
            // Show the categorised error class verbatim — useful for triage.
            reason: result.error_class,
          }),
          description: t("adminProxies.testTarget", { target: result.target_url }),
          tone: "error",
        });
      }
    } catch (err) {
      toast({
        title: t("feedback.failed"),
        description: err instanceof Error ? err.message : String(err),
        tone: "error",
      });
    }
  }
  const batchDeleteMut = useBatchDeleteProxies();
  const { toast } = useToast();

  const [formTarget, setFormTarget] = useState<ProxyDefinition | "new" | null>(null);
  const [toDelete, setToDelete] = useState<ProxyDefinition | null>(null);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);
  const isNew = formTarget === "new";
  const visibleProxyRows = proxies.data?.data ?? [];
  const backupProxyOptions = [
    { value: PROXY_BACKUP_NONE, label: t("adminProxies.backupProxyNone") },
    ...visibleProxyRows
      .filter((proxy) => formTarget === "new" || !formTarget || proxy.id !== formTarget.id)
      .map((proxy) => ({ value: proxy.id, label: proxy.name })),
  ];

  // Bulk-test the selection. The server runs the probes in parallel with its
  // own concurrency cap (see Service.BatchTestProxies) so the frontend gets
  // one HTTP call and one result list — simpler than the iter-26 client-side
  // Promise.allSettled loop and faster on big selections.
  async function bulkTest() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const rows = await batchTestMut.mutateAsync(ids);
      const okCount = rows.filter((r) => r.result.ok).length;
      const failCount = rows.length - okCount;
      if (failCount === 0) {
        toast({
          title: t("adminProxies.bulkTestOk", { count: okCount }),
          tone: "success",
        });
      } else if (okCount === 0) {
        toast({
          title: t("adminProxies.bulkTestAllFailed", { count: failCount }),
          tone: "error",
        });
      } else {
        toast({
          title: t("adminProxies.bulkTestPartial", { ok: okCount, fail: failCount }),
          tone: "warning",
        });
      }
    } catch (err) {
      toast({
        title: t("feedback.failed"),
        description: err instanceof Error ? err.message : String(err),
        tone: "error",
      });
    }
  }

  /** Atomic bulk-soft-delete via /admin/proxies/batch-delete. Per-id outcome
   * means partial failures show in the toast rather than silently rolling
   * back the whole call — same UX pattern as the accounts bulk-status flow. */
  async function applyBulkDelete() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchDeleteMut.mutateAsync(ids);
      list.clearSelection();
      setBulkDeleteOpen(false);
      const failed = result.errors.length;
      const succeeded = result.deleted_count;
      if (failed > 0 && succeeded > 0) {
        toast({ title: t("feedback.batchPartial", { succeeded, failed }), tone: "warning" });
      } else if (failed > 0) {
        toast({ title: t("feedback.batchAllFailed", { count: ids.length }), tone: "error" });
      } else {
        toast({ title: t("feedback.batchAllSucceeded", { count: succeeded }), tone: "success" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: String(err), tone: "error" });
    }
  }

  const fields: FieldConfig<ProxyFormState>[] = [
    { name: "name", label: t("adminProxies.name"), required: true },
    {
      name: "type",
      label: t("adminProxies.protocol"),
      type: "select",
      options: enumOptions(PROXY_TYPES),
    },
    {
      name: "url",
      label: t("adminProxies.url"),
      placeholder: "http://user:pass@host:port",
      hint: isNew ? undefined : t("adminProxies.urlEditHint"),
    },
    {
      // Country is a long select sourced from the canonical ISO list. A
      // leading "—" sentinel clears the field (Radix Select rejects empty
      // SelectItem values, so we use a sentinel and translate it back to ""
      // inside withCountryName at save time). The form commits the ISO code;
      // the snapshot label is captured into countryName at save time inside
      // withCountryName so the list view does not depend on the viewer's
      // locale to render a stable country column.
      name: "countryCode",
      label: t("adminProxies.countrySelectLabel"),
      type: "select",
      options: [{ value: COUNTRY_NONE, label: "— " + t("adminProxies.countrySelectPlaceholder") }, ...countrySelectOptions],
      placeholder: t("adminProxies.countrySelectPlaceholder"),
    },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(PROXY_STATUSES),
      advanced: true,
    },
    {
      name: "expiresAtLocal",
      label: t("adminProxies.expiresAt"),
      type: "datetime",
      hint: t("adminProxies.expiresAtHint"),
      advanced: true,
    },
    {
      name: "fallbackMode",
      label: t("adminProxies.fallbackMode"),
      type: "select",
      options: fallbackModeOptions(t),
      advanced: true,
    },
    {
      name: "backupProxyId",
      label: t("adminProxies.backupProxy"),
      type: "select",
      options: backupProxyOptions,
      advanced: true,
      validate: (value, draft) => {
        const selected = String(value ?? "");
        return draft.fallbackMode === "proxy" && (!selected || selected === PROXY_BACKUP_NONE)
          ? t("adminProxies.backupProxyRequired")
          : undefined;
      },
    },
    { name: "metadata", label: t("adminCommon.metadata"), help: t("adminCommon.metadataHelp"), type: "keyvalue", advanced: true },
  ];

  // Snapshot the localized country label at save time so the table shows a
  // stable name even when the operator's locale later changes. Done in a thin
  // wrapper around buildCreateProxyBody / buildUpdateProxyBody so the form
  // helper stays pure and re-usable from tests.
  // Snapshot the localized country label so the table shows a stable name
  // even when the operator's locale later changes. buildCreate/UpdateProxyBody
  // already normalises country_code (handles the sentinel, trims, uppercases);
  // this thin wrapper just attaches a matching country_name.
  const withCountryName = <T extends { country_code?: string | null; country_name?: string | null }>(body: T): T => {
    const code = (body.country_code ?? "").trim().toUpperCase();
    if (!code) {
      return { ...body, country_code: null, country_name: null };
    }
    return { ...body, country_code: code, country_name: localizedCountryName(code, code) };
  };

  const columns: Column<ProxyDefinition>[] = [
    {
      key: "name",
      header: t("adminProxies.name"),
      pinned: true,
      sortValue: (p) => p.name,
      render: (p) => <span className="text-srapi-text-primary">{p.name}</span>,
    },
    {
      key: "protocol",
      header: t("adminProxies.protocol"),
      render: (p) => (
        <span className="font-mono text-2xs uppercase text-srapi-text-secondary">{p.type}</span>
      ),
    },
    {
      key: "country",
      header: t("adminProxies.countryColumn"),
      sortValue: (p) =>
        (p.country_name ?? "") || (p.country_code ?? "") || "",
      render: (p) => {
        const code = (p.country_code ?? "").trim().toUpperCase();
        const name = localizedCountryName(code, p.country_name);
        if (!code && !name) {
          return <span className="text-srapi-text-tertiary">—</span>;
        }
        return (
          <div className="flex items-center gap-1.5">
            <span className="text-srapi-text-primary">{name || code}</span>
            {code && name ? (
              <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">{code}</span>
            ) : null}
          </div>
        );
      },
    },
    {
      key: "availability",
      header: t("adminProxies.availabilityColumn"),
      sortValue: (p) => {
        const total = (p.probe_success_count ?? 0) + (p.probe_failure_count ?? 0);
        if (total <= 0) return -1;
        return typeof p.probe_success_pct_7d === "number" ? p.probe_success_pct_7d : -1;
      },
      render: (p) => {
        const total = (p.probe_success_count ?? 0) + (p.probe_failure_count ?? 0);
        if (!p.last_probed_at || total <= 0 || typeof p.probe_success_pct_7d !== "number") {
          return (
            <span className="text-2xs text-srapi-text-tertiary" title={t("adminProxies.availabilityNeverProbed")}>
              —
            </span>
          );
        }
        return <AvailabilityBadge pct={p.probe_success_pct_7d} />;
      },
    },
    {
      key: "lifecycle",
      header: t("adminProxies.lifecycleColumn"),
      hideOnMobile: true,
      sortValue: (p) => p.expires_at ?? "",
      render: (p) => (
        <ProxyLifecycleCell
          proxy={p}
          proxies={visibleProxyRows}
          t={t}
        />
      ),
    },
    {
      key: "url",
      header: t("adminProxies.url"),
      hideOnMobile: true,
      render: (p) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {p.url_configured ? "✓" : "—"}
        </span>
      ),
    },
    {
      key: "last_test",
      header: t("adminProxies.lastTest"),
      hideOnMobile: true,
      sortValue: (p) => readLastTest(p)?.at ?? "",
      render: (p) => {
        const snap = readLastTest(p);
        if (!snap) {
          return <span className="text-2xs text-srapi-text-tertiary">{t("adminProxies.neverTested")}</span>;
        }
        return (
          <div className="flex flex-col gap-0.5">
            <div className="flex items-center gap-1.5">
              <span
                className={
                  "inline-block h-1.5 w-1.5 rounded-full " +
                  (snap.ok ? "bg-srapi-success" : "bg-srapi-error")
                }
                aria-hidden
              />
              <span className="font-mono text-2xs text-srapi-text-tertiary">
                {formatDateTime(snap.at)}
              </span>
            </div>
            <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
              {snap.ok
                ? formatLatency(snap.latency_ms)
                : snap.error_class || t("adminProxies.testFailed", { reason: "" })}
            </span>
          </div>
        );
      },
    },
    {
      key: "status",
      header: t("common.active"),
      render: (p) => <QuietBadge status={quietStatusFor(p.status)} label={statusLabel(t, p.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminProxies.title")}
        description={t("adminProxies.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {proxies.data ? (
              <ListCount total={proxies.data.pagination?.total ?? proxies.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminProxies.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={filteredProxies}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => p.id}
        emptyIcon={Network}
        emptyTitle={t("adminProxies.emptyTitle")}
        emptyBody={t("adminProxies.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminProxies.create")}
          </Button>
        }
        minWidth={520}
        isFiltered={Boolean(statusFilter) || Boolean(countryFilter)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(PROXY_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
            <FilterSelect
              value={countryFilter}
              onChange={(v) => list.setFilter("country", v)}
              options={countryFilterOptions(proxies.data?.data, localizedCountryName)}
              allLabel={t("adminProxies.countrySelectPlaceholder")}
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
                loading={batchTestMut.isPending}
                onClick={() => void bulkTest()}
              >
                {t("adminProxies.bulkTest")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                loading={batchDeleteMut.isPending}
                onClick={() => setBulkDeleteOpen(true)}
              >
                {t("common.delete")}
              </Button>
            </>
          ),
        }}
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: proxies.data?.pagination?.total ?? proxies.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(p) },
              { label: t("adminProxies.test"), onSelect: () => void runTest(p.id) },
              { label: t("common.delete"), destructive: true, onSelect: () => setToDelete(p) },
            ]}
          />
        )}
      />

      <ConfirmDialog
        open={toDelete !== null}
        onOpenChange={(open) => {
          if (!open) setToDelete(null);
        }}
        title={t("adminProxies.deleteTitle")}
        body={t("adminProxies.deleteBody", { name: toDelete?.name ?? "" })}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteMut.isPending}
        onConfirm={async () => {
          if (toDelete) await deleteMut.mutateAsync(toDelete.id);
        }}
      />

      <ConfirmDialog
        open={bulkDeleteOpen}
        onOpenChange={setBulkDeleteOpen}
        title={t("adminProxies.bulkDeleteTitle")}
        body={t("adminProxies.bulkDeleteBody", { count: list.selected.size })}
        confirmLabel={t("common.delete")}
        isPending={batchDeleteMut.isPending}
        onConfirm={applyBulkDelete}
      />

      {formTarget === "new" ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={t("adminProxies.create")}
          fields={fields}
          initial={emptyProxyForm()}
          buildBody={(form) => withCountryName(buildCreateProxyBody(form))}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={t("adminProxies.edit")}
          fields={fields}
          initial={proxyFormFromProxy(formTarget)}
          buildBody={(form) => withCountryName(buildUpdateProxyBody(form))}
          submit={(body) => updateMut.mutateAsync({ id: formTarget.id, body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}
    </>
  );
}

// AvailabilityBadge renders the rolling 7-day success percentage with three
// tone thresholds chosen to match the operator's read-at-a-glance intent:
// >=95% is green ("ship it"), 70-94% is yellow ("watch this"), <70% is red
// ("don't route through this"). Exported so the unit test can render it
// directly without bootstrapping the whole admin page.
export function AvailabilityBadge({ pct }: { pct: number }) {
  // The QuietBadge palette uses "active"=green, "limited"=yellow, "error"=red
  // — the same tone scheme the rest of the admin uses for at-a-glance status.
  const tone: "active" | "limited" | "error" = pct >= 95 ? "active" : pct >= 70 ? "limited" : "error";
  return <QuietBadge status={tone} label={`${pct}%`} />;
}

function ProxyLifecycleCell({
  proxy,
  proxies,
  t,
}: {
  proxy: ProxyDefinition;
  proxies: readonly ProxyDefinition[];
  t: (key: string, vars?: Record<string, string | number>) => string;
}) {
  const expiresAt = proxy.expires_at ? new Date(proxy.expires_at) : null;
  const expired = expiresAt ? expiresAt.getTime() <= Date.now() : false;
  const fallbackMode = proxy.fallback_mode ?? "none";
  const backupName = proxy.backup_proxy_id
    ? proxies.find((item) => item.id === proxy.backup_proxy_id)?.name ?? `#${proxy.backup_proxy_id}`
    : "";
  const fallbackLabel = fallbackModeLabel(t, fallbackMode);
  return (
    <div className="flex flex-col gap-0.5">
      <span className={expired ? "text-2xs text-srapi-error" : "font-mono text-2xs text-srapi-text-tertiary"}>
        {expiresAt ? (expired ? t("adminProxies.expired") : formatDateTime(proxy.expires_at)) : t("adminProxies.noExpiry")}
      </span>
      <span className="text-2xs text-srapi-text-tertiary">
        {fallbackMode === "proxy" && backupName ? `${fallbackLabel}: ${backupName}` : fallbackLabel}
      </span>
    </div>
  );
}

function fallbackModeOptions(t: (key: string) => string): { value: string; label: string }[] {
  return PROXY_FALLBACK_MODES.map((mode) => ({ value: mode, label: fallbackModeLabel(t, mode) }));
}

function fallbackModeLabel(t: (key: string) => string, mode: string): string {
  if (mode === "direct") return t("adminProxies.fallbackDirect");
  if (mode === "proxy") return t("adminProxies.fallbackProxy");
  return t("adminProxies.fallbackNone");
}

// countryFilterOptions returns one entry per unique country_code present in
// the visible list. Operators see only the codes that actually exist in their
// proxy roster — no point in offering 250 ISO codes when 4 are in use.
function countryFilterOptions(
  rows: readonly { country_code?: string | null }[] | undefined,
  resolveName: (code: string | null | undefined, fallback: string | null | undefined) => string,
): { value: string; label: string }[] {
  const seen = new Set<string>();
  const out: { value: string; label: string }[] = [];
  for (const row of rows ?? []) {
    const code = (row.country_code ?? "").trim().toUpperCase();
    if (!code || seen.has(code)) continue;
    seen.add(code);
    const name = resolveName(code, null);
    out.push({ value: code, label: name ? `${name} (${code})` : code });
  }
  out.sort((a, b) => a.label.localeCompare(b.label));
  return out;
}

interface LastProxyTest {
  at: string;
  ok: boolean;
  latency_ms: number;
  error_class: string;
}

// readLastTest extracts the persisted `_last_test` snapshot the backend writes
// into proxy.metadata after each Test action. The metadata field is
// loosely-typed `JsonObject` so we narrow defensively — older rows that have
// never been tested return null and the column renders "Never tested".
function readLastTest(p: ProxyDefinition): LastProxyTest | null {
  const metadata = (p as { metadata?: Record<string, unknown> }).metadata;
  if (!metadata || typeof metadata !== "object") return null;
  const raw = (metadata as Record<string, unknown>)["_last_test"];
  if (!raw || typeof raw !== "object") return null;
  const obj = raw as Record<string, unknown>;
  if (typeof obj.at !== "string") return null;
  return {
    at: obj.at,
    ok: obj.ok === true,
    latency_ms: typeof obj.latency_ms === "number" ? obj.latency_ms : 0,
    error_class: typeof obj.error_class === "string" ? obj.error_class : "",
  };
}
