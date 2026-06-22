"use client";

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { ChevronDown, ChevronRight, ExternalLink } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DensityToggle, type DensityValue } from "@/components/ui/density-toggle";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { ExpandableRow } from "@/components/ui/expandable-row";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";
import {
  adminErrorLogsHref,
  adminRequestDumpsHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";

// LiveErrorsPanel subscribes to the in-memory SSE error stream backed by the
// error_event_stream module on the API side. It mirrors the CLIProxyAPI
// SubscribeErrors fan-out: every recorded gateway provider attempt failure
// arrives here within ~10ms of being classified, so the operator can watch a
// rolling failure feed without polling /admin/ops/error-logs.
//
// The panel caps the in-memory list at 200 entries (newest first) so a long
// idle browser tab does not grow unbounded; older events are dropped from the
// UI but the backend ring buffer still serves replay via the ?since= filter on
// re-subscribe.
const MAX_EVENTS = 200;

interface LiveEvent {
  at_unix_ms: number;
  request_id: string;
  trace_id?: string;
  user_id?: number;
  account_id?: number;
  provider_id?: number;
  account_name?: string;
  provider_name?: string;
  model?: string;
  requested_model?: string;
  upstream_model?: string;
  source_endpoint?: string;
  source_protocol?: string;
  target_protocol?: string;
  attempt_no?: number;
  status_code: number;
  upstream_request_id?: string;
  error_class?: string;
  error_phase?: string;
  error_owner?: string;
  error_source?: string;
  message?: string;
  body_excerpt?: string;
}

function eventKey(ev: LiveEvent): string {
  return [
    ev.request_id,
    ev.trace_id ?? "",
    ev.upstream_request_id ?? "",
    ev.attempt_no ?? "",
    ev.at_unix_ms,
  ].join("|");
}

function identityLabel(name?: string, id?: number) {
  if (name && id != null) return `${name} #${id}`;
  if (name) return name;
  if (id != null) return `#${id}`;
  return "-";
}

function protocolLabel(source?: string, target?: string) {
  if (source && target) return `${source} -> ${target}`;
  return source || target || "-";
}

// Maps a raw HTTP status into the row severity (drives both the .log-row
// stripe and the SegmentedControl quick filter).
function liveEventSeverity(code: number): "critical" | "error" | "warning" | "info" {
  if (code >= 500) return "critical";
  if (code >= 400) return "error";
  if (code > 0) return "warning";
  return "info";
}

export function LiveErrorsPanel() {
  const { t } = useLanguage();
  const [events, setEvents] = useState<LiveEvent[]>([]);
  const [paused, setPaused] = useState(false);
  const [connected, setConnected] = useState(false);
  const [accountId, setAccountId] = useState("");
  const [errorClass, setErrorClass] = useState("");
  const [appliedFilters, setAppliedFilters] = useState({ accountId: "", errorClass: "" });
  const [severityFilter, setSeverityFilter] = useState<"all" | "critical" | "error" | "warning">("all");
  const [density, setDensity] = useState<DensityValue>("regular");
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const pausedRef = useRef(paused);
  const lastSeenUnixMsRef = useRef(0);

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  // EventSource is opened against the SSE endpoint with the applied filters
  // baked into the URL. Changing filters tears the connection down and
  // reopens — simpler than a per-event JS filter and avoids burning the
  // server-side subscriber quota on broad subscriptions.
  useEffect(() => {
    const params = new URLSearchParams();
    if (appliedFilters.accountId.trim() !== "") {
      params.set("account_id", appliedFilters.accountId.trim());
    }
    if (appliedFilters.errorClass.trim() !== "") {
      params.set("error_class", appliedFilters.errorClass.trim());
    }
    if (lastSeenUnixMsRef.current > 0) {
      params.set("since", String(lastSeenUnixMsRef.current + 1));
    }
    const qs = params.toString();
    const url = `/api/v1/admin/error-stream${qs ? `?${qs}` : ""}`;
    const es = new EventSource(url, { withCredentials: true });
    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    const handler = (ev: MessageEvent) => {
      if (pausedRef.current) return;
      try {
        const parsed = JSON.parse(ev.data) as LiveEvent;
        if (parsed.at_unix_ms > lastSeenUnixMsRef.current) {
          lastSeenUnixMsRef.current = parsed.at_unix_ms;
        }
        setEvents((prev) => {
          const parsedKey = eventKey(parsed);
          if (prev.some((item) => eventKey(item) === parsedKey)) {
            return prev;
          }
          const next = [parsed, ...prev];
          return next.length > MAX_EVENTS ? next.slice(0, MAX_EVENTS) : next;
        });
      } catch {
        /* malformed frame — ignore */
      }
    };
    es.addEventListener("gateway_error", handler);
    return () => {
      es.close();
      setConnected(false);
    };
  }, [appliedFilters]);

  const applyFilters = useCallback(() => {
    setAppliedFilters({ accountId, errorClass });
  }, [accountId, errorClass]);

  const clearFilters = useCallback(() => {
    setAccountId("");
    setErrorClass("");
    setAppliedFilters({ accountId: "", errorClass: "" });
  }, []);

  const formatted = useMemo(
    () =>
      events
        .map((ev) => ({
          ...ev,
          atLabel: formatDateTime(new Date(ev.at_unix_ms).toISOString()),
          severity: liveEventSeverity(ev.status_code),
        }))
        .filter((ev) => severityFilter === "all" || ev.severity === severityFilter),
    [events, severityFilter],
  );
  const rowPad = density === "compact" ? "py-1.5 px-3" : "py-3 px-4";

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("adminLiveErrors.title")}
        description={t("adminLiveErrors.subtitle")}
      />

      <div className="space-y-2 rounded-xl border border-srapi-border bg-srapi-card p-3">
        {/* Severity chip strip — narrows the live tail to «5xx critical»,
            «4xx error», or «other» with a single click. Sits above the
            general filters because severity is the dominant pivot. */}
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            Severity
          </span>
          <SegmentedControl
            value={severityFilter}
            onChange={(v) => setSeverityFilter(v)}
            options={[
              { value: "all", label: "All" },
              { value: "critical", label: "5xx" },
              { value: "error", label: "4xx" },
              { value: "warning", label: "Other" },
            ]}
            size="sm"
            ariaLabel="live error severity filter"
          />
          <div className="flex-1" />
          <DensityToggle value={density} onChange={setDensity} />
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <input
            value={accountId}
            onChange={(e) => setAccountId(e.target.value)}
            placeholder={t("adminLiveErrors.accountIdPlaceholder")}
            className="h-9 w-32 rounded-lg border border-srapi-border bg-srapi-card px-2.5 text-sm"
          />
          <input
            value={errorClass}
            onChange={(e) => setErrorClass(e.target.value)}
            placeholder={t("adminLiveErrors.errorClassPlaceholder")}
            className="h-9 w-48 rounded-lg border border-srapi-border bg-srapi-card px-2.5 text-sm"
          />
          <button
            type="button"
            onClick={applyFilters}
            className="rounded-lg border border-srapi-border px-3 py-1.5 text-xs font-medium text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            {t("adminLiveErrors.applyFilters")}
          </button>
          <button
            type="button"
            onClick={clearFilters}
            className="rounded-lg border border-srapi-border px-3 py-1.5 text-xs font-medium text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            {t("adminLiveErrors.clearFilters")}
          </button>
          <div className="flex-1" />
          <button
            type="button"
            onClick={() => setPaused((p) => !p)}
            className="rounded-lg border border-srapi-border px-3 py-1.5 text-xs font-medium text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            {paused ? t("adminLiveErrors.resume") : t("adminLiveErrors.pause")}
          </button>
          <button
            type="button"
            onClick={() => setEvents([])}
            className="rounded-lg border border-srapi-border px-3 py-1.5 text-xs font-medium text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            {t("adminLiveErrors.clear")}
          </button>
          <span
            className={
              "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium " +
              (connected
                ? "bg-srapi-success/12 text-srapi-success"
                : "bg-srapi-warning/12 text-srapi-warning")
            }
          >
            <span
              className={
                "inline-block h-1.5 w-1.5 rounded-full " +
                (connected ? "bg-srapi-success" : "bg-srapi-warning")
              }
            />
            {connected
              ? t("adminLiveErrors.connected")
              : t("adminLiveErrors.disconnected")}
          </span>
        </div>
      </div>

      {formatted.length === 0 ? (
        <IllustratedEmptyState
          illust="logs"
          title={t("adminLiveErrors.emptyTitle")}
          description={t("adminLiveErrors.emptyBody")}
        />
      ) : (
        <div className="overflow-x-auto rounded-xl border border-srapi-border bg-srapi-card">
          <table className="w-full table-fixed text-left text-sm">
            <thead className="border-b border-srapi-border bg-srapi-card-muted">
              <tr>
                <th className="w-44 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.time")}</th>
                <th className="w-24 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.status")}</th>
                <th className="w-40 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.errorClass")}</th>
                <th className="w-56 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.route")}</th>
                <th className="w-56 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.account")}</th>
                <th className="w-48 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.model")}</th>
                <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.message")}</th>
                <th className="w-56 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminLiveErrors.evidence")}</th>
              </tr>
            </thead>
            <tbody>
              {formatted.map((row, idx) => {
                const key = `${row.request_id}-${row.at_unix_ms}-${idx}`;
                const open = expandedKey === key;
                return (
                  <React.Fragment key={key}>
                    <tr
                      data-sev={row.severity}
                      onClick={(e) => {
                        const target = e.target as HTMLElement | null;
                        if (target?.closest('a,button,input')) return;
                        setExpandedKey((prev) => (prev === key ? null : key));
                      }}
                      className={`log-row cursor-pointer border-t border-srapi-border/70 transition-colors hover:bg-srapi-card-muted/50`}
                    >
                      <td className={`${rowPad} text-[12px] tabular text-srapi-text-tertiary`}>
                        <span className="inline-flex items-center gap-1.5">
                          {open ? (
                            <ChevronDown aria-hidden className="size-3 shrink-0" />
                          ) : (
                            <ChevronRight aria-hidden className="size-3 shrink-0" />
                          )}
                          {row.atLabel}
                        </span>
                      </td>
                      <td className={`${rowPad} text-xs`}>
                        {/* DataTooltip exposes the breakdown that previously
                            only existed at row-level: class / phase / owner /
                            attempt — surfaced on hover so a scanning operator
                            doesn't have to read the full row to triage. */}
                        <DataTooltip
                          title={`HTTP ${row.status_code || "—"}`}
                          primary={row.status_code || "—"}
                          rows={[
                            { label: "class", value: row.error_class || "—", tone: "error" },
                            { label: "phase", value: row.error_phase || "—" },
                            { label: "owner", value: row.error_owner || "—" },
                            { label: "attempt", value: row.attempt_no ?? 1, tone: "muted" },
                          ]}
                        >
                          <span
                            className={
                              "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium tabular " +
                              (row.severity === "critical"
                                ? "bg-srapi-error/12 text-srapi-error"
                                : row.severity === "error"
                                ? "bg-srapi-warning/12 text-srapi-warning"
                                : "bg-srapi-card-muted text-srapi-text-secondary")
                            }
                          >
                            {row.status_code || "-"}
                          </span>
                        </DataTooltip>
                      </td>
                      <td className={`${rowPad} text-xs`}>
                        <div className="text-srapi-error">{row.error_class ?? "-"}</div>
                        <div className="mt-0.5 text-[11px] text-srapi-text-tertiary">
                          {[row.error_phase, row.error_owner].filter(Boolean).join(" / ") || "-"}
                        </div>
                      </td>
                      <td className={`${rowPad} text-xs text-srapi-text-tertiary`}>
                        <div className="truncate text-srapi-text-secondary" title={row.source_endpoint ?? ""}>
                          {row.source_endpoint ?? "-"}
                        </div>
                        <div
                          className="mt-0.5 truncate text-[11px]"
                          title={protocolLabel(row.source_protocol, row.target_protocol)}
                        >
                          {protocolLabel(row.source_protocol, row.target_protocol)}
                        </div>
                      </td>
                      <td className={`${rowPad} text-xs text-srapi-text-tertiary`}>
                        <div
                          className="truncate text-srapi-text-secondary"
                          title={identityLabel(row.provider_name, row.provider_id)}
                        >
                          {identityLabel(row.provider_name, row.provider_id)}
                        </div>
                        <div
                          className="mt-0.5 truncate text-[11px]"
                          title={identityLabel(row.account_name, row.account_id)}
                        >
                          {identityLabel(row.account_name, row.account_id)}
                        </div>
                      </td>
                      <td className={`${rowPad} text-xs text-srapi-text-tertiary`}>
                        <div className="truncate text-srapi-text-secondary" title={row.model ?? ""}>
                          {row.model ?? "-"}
                        </div>
                        <div
                          className="mt-0.5 truncate text-[11px]"
                          title={row.upstream_model ?? row.requested_model ?? ""}
                        >
                          {row.upstream_model || row.requested_model || "-"}
                        </div>
                      </td>
                      <td className={`${rowPad} text-xs`}>
                        <div className="truncate" title={row.message ?? ""}>
                          {row.message ?? "-"}
                        </div>
                      </td>
                      <td className={`${rowPad} text-[11px] text-srapi-text-tertiary`}>
                        <div className="truncate" title={row.request_id}>
                          {row.request_id}
                        </div>
                        <div className="mt-0.5 truncate" title={row.upstream_request_id ?? ""}>
                          {t("adminLiveErrors.attempt")} {row.attempt_no ?? 1}
                          {row.upstream_request_id ? ` / ${row.upstream_request_id}` : ""}
                        </div>
                        <LiveEventEvidenceLinks requestID={row.request_id} traceID={row.trace_id} />
                      </td>
                    </tr>
                    {open ? (
                      <tr data-expand-for={key}>
                        <td colSpan={8} className="p-0">
                          <ExpandableRow expanded>
                            <InlineDetailGrid
                              sections={[
                                {
                                  title: "Request",
                                  rows: [
                                    { label: "request_id", value: row.request_id || "—", mono: true },
                                    { label: "trace_id", value: row.trace_id || "—", mono: true, tone: "muted" },
                                    { label: "endpoint", value: row.source_endpoint || "—", mono: true },
                                    { label: "protocol", value: protocolLabel(row.source_protocol, row.target_protocol), mono: true },
                                  ],
                                },
                                {
                                  title: "Response",
                                  rows: [
                                    { label: "status", value: row.status_code || "—", mono: true, tone: row.severity === "critical" ? "error" : row.severity === "error" ? "warning" : "default" },
                                    { label: "class", value: row.error_class || "—", mono: true, tone: "error" },
                                    { label: "phase", value: row.error_phase || "—" },
                                    { label: "owner", value: row.error_owner || "—" },
                                    { label: "message", value: row.message || "—", tone: "muted" },
                                  ],
                                },
                                {
                                  title: "Routing",
                                  rows: [
                                    { label: "provider", value: identityLabel(row.provider_name, row.provider_id) },
                                    { label: "account", value: identityLabel(row.account_name, row.account_id) },
                                    { label: "model", value: row.model || "—", mono: true },
                                    { label: "upstream model", value: row.upstream_model || row.requested_model || "—", mono: true, tone: "muted" },
                                    { label: "attempt", value: row.attempt_no ?? 1, mono: true, tone: "muted" },
                                    { label: "upstream req_id", value: row.upstream_request_id || "—", mono: true, tone: "muted" },
                                  ],
                                },
                              ]}
                            />
                            {row.body_excerpt ? (
                              <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 pb-4 pt-3">
                                <div className="mb-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                                  upstream body excerpt
                                </div>
                                <pre className="max-h-40 overflow-auto rounded-lg bg-srapi-card-muted p-3 font-mono text-[11px] text-srapi-text-secondary">
                                  {row.body_excerpt}
                                </pre>
                              </div>
                            ) : null}
                          </ExpandableRow>
                        </td>
                      </tr>
                    ) : null}
                  </React.Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function LiveEventEvidenceLinks({ requestID, traceID }: { requestID?: string; traceID?: string }) {
  const { t } = useLanguage();
  const errorHref = adminErrorLogsHref({ request_id: requestID, trace_id: traceID });
  const systemHref = adminSystemLogsHref({ request_id: requestID, trace_id: traceID });
  const requestDumpHref = adminRequestDumpsHref({ request_id: requestID });
  const links = [
    errorHref ? { href: errorHref, label: t("adminRequestLogFiles.openErrorLogs") } : null,
    systemHref ? { href: systemHref, label: t("adminRequestLogFiles.openSystemLogs") } : null,
    requestDumpHref ? { href: requestDumpHref, label: t("adminOpsSystemLogs.openRequestDumps") } : null,
  ].filter((item): item is { href: string; label: string } => item !== null);

  if (links.length === 0) return null;

  return (
    <div className="mt-1 flex flex-wrap gap-1.5">
      {links.map((link) => (
        <Link
          key={link.href}
          href={link.href}
          className="inline-flex items-center gap-1 rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary hover:text-srapi-text-primary"
        >
          {link.label}
          <ExternalLink className="size-2.5" aria-hidden />
        </Link>
      ))}
    </div>
  );
}
