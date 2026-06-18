"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";

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

export function LiveErrorsPanel() {
  const { t } = useLanguage();
  const [events, setEvents] = useState<LiveEvent[]>([]);
  const [paused, setPaused] = useState(false);
  const [connected, setConnected] = useState(false);
  const [accountId, setAccountId] = useState("");
  const [errorClass, setErrorClass] = useState("");
  const [appliedFilters, setAppliedFilters] = useState({ accountId: "", errorClass: "" });
  const pausedRef = useRef(paused);

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
    const qs = params.toString();
    const url = `/api/v1/admin/error-stream${qs ? `?${qs}` : ""}`;
    const es = new EventSource(url, { withCredentials: true });
    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    const handler = (ev: MessageEvent) => {
      if (pausedRef.current) return;
      try {
        const parsed = JSON.parse(ev.data) as LiveEvent;
        setEvents((prev) => {
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
      events.map((ev) => ({
        ...ev,
        atLabel: formatDateTime(new Date(ev.at_unix_ms).toISOString()),
      })),
    [events],
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("adminLiveErrors.title")}
        description={t("adminLiveErrors.subtitle")}
      />

      <div className="flex flex-wrap items-center gap-3 rounded-lg border border-srapi-border-subtle bg-srapi-bg-card p-3">
        <input
          value={accountId}
          onChange={(e) => setAccountId(e.target.value)}
          placeholder={t("adminLiveErrors.accountIdPlaceholder")}
          className="h-8 w-32 rounded border border-srapi-border-subtle bg-srapi-bg-input px-2 text-sm"
        />
        <input
          value={errorClass}
          onChange={(e) => setErrorClass(e.target.value)}
          placeholder={t("adminLiveErrors.errorClassPlaceholder")}
          className="h-8 w-48 rounded border border-srapi-border-subtle bg-srapi-bg-input px-2 text-sm"
        />
        <button
          type="button"
          onClick={applyFilters}
          className="rounded border border-srapi-border-subtle px-3 py-1 text-xs hover:bg-srapi-bg-card-elevated"
        >
          {t("adminLiveErrors.applyFilters")}
        </button>
        <button
          type="button"
          onClick={clearFilters}
          className="rounded border border-srapi-border-subtle px-3 py-1 text-xs hover:bg-srapi-bg-card-elevated"
        >
          {t("adminLiveErrors.clearFilters")}
        </button>
        <div className="flex-1" />
        <button
          type="button"
          onClick={() => setPaused((p) => !p)}
          className="rounded border border-srapi-border-subtle px-3 py-1 text-xs hover:bg-srapi-bg-card-elevated"
        >
          {paused ? t("adminLiveErrors.resume") : t("adminLiveErrors.pause")}
        </button>
        <button
          type="button"
          onClick={() => setEvents([])}
          className="rounded border border-srapi-border-subtle px-3 py-1 text-xs hover:bg-srapi-bg-card-elevated"
        >
          {t("adminLiveErrors.clear")}
        </button>
        <span
          className={
            "inline-flex items-center gap-1 rounded px-2 py-0.5 text-[11px] " +
            (connected
              ? "bg-emerald-500/15 text-emerald-300"
              : "bg-amber-500/15 text-amber-300")
          }
        >
          <span
            className={
              "inline-block h-1.5 w-1.5 rounded-full " +
              (connected ? "bg-emerald-400" : "bg-amber-400")
            }
          />
          {connected
            ? t("adminLiveErrors.connected")
            : t("adminLiveErrors.disconnected")}
        </span>
      </div>

      {formatted.length === 0 ? (
        <div className="rounded-lg border border-srapi-border-subtle bg-srapi-bg-card p-6 text-center text-sm text-srapi-text-tertiary">
          <p className="font-medium text-srapi-text-secondary">
            {t("adminLiveErrors.emptyTitle")}
          </p>
          <p>{t("adminLiveErrors.emptyBody")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-srapi-border-subtle bg-srapi-bg-card">
          <table className="w-full table-fixed text-left text-sm">
            <thead className="border-b border-srapi-border-subtle bg-srapi-bg-card-elevated">
              <tr>
                <th className="w-44 px-3 py-2 font-medium">{t("adminLiveErrors.time")}</th>
                <th className="w-24 px-3 py-2 font-medium">{t("adminLiveErrors.status")}</th>
                <th className="w-40 px-3 py-2 font-medium">{t("adminLiveErrors.errorClass")}</th>
                <th className="w-56 px-3 py-2 font-medium">{t("adminLiveErrors.route")}</th>
                <th className="w-56 px-3 py-2 font-medium">{t("adminLiveErrors.account")}</th>
                <th className="w-48 px-3 py-2 font-medium">{t("adminLiveErrors.model")}</th>
                <th className="px-3 py-2 font-medium">{t("adminLiveErrors.message")}</th>
                <th className="w-56 px-3 py-2 font-medium">{t("adminLiveErrors.evidence")}</th>
              </tr>
            </thead>
            <tbody>
              {formatted.map((row, idx) => (
                <tr
                  key={`${row.request_id}-${row.at_unix_ms}-${idx}`}
                  className="border-t border-srapi-border-subtle"
                >
                  <td className="px-3 py-2 text-xs text-srapi-text-tertiary">{row.atLabel}</td>
                  <td className="px-3 py-2 text-xs">
                    <span
                      className={
                        "rounded px-1.5 py-0.5 " +
                        (row.status_code >= 500
                          ? "bg-red-500/15 text-red-300"
                          : row.status_code >= 400
                          ? "bg-amber-500/15 text-amber-300"
                          : "bg-srapi-bg-input text-srapi-text-secondary")
                      }
                    >
                      {row.status_code || "-"}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-xs">
                    <div className="font-mono text-srapi-error">{row.error_class ?? "-"}</div>
                    <div className="mt-0.5 font-mono text-[11px] text-srapi-text-tertiary">
                      {[row.error_phase, row.error_owner].filter(Boolean).join(" / ") || "-"}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-xs text-srapi-text-tertiary">
                    <div className="truncate font-mono text-srapi-text-secondary" title={row.source_endpoint ?? ""}>
                      {row.source_endpoint ?? "-"}
                    </div>
                    <div
                      className="mt-0.5 truncate font-mono text-[11px]"
                      title={protocolLabel(row.source_protocol, row.target_protocol)}
                    >
                      {protocolLabel(row.source_protocol, row.target_protocol)}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-xs text-srapi-text-tertiary">
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
                  <td className="px-3 py-2 text-xs text-srapi-text-tertiary">
                    <div className="truncate text-srapi-text-secondary" title={row.model ?? ""}>
                      {row.model ?? "-"}
                    </div>
                    <div
                      className="mt-0.5 truncate font-mono text-[11px]"
                      title={row.upstream_model ?? row.requested_model ?? ""}
                    >
                      {row.upstream_model || row.requested_model || "-"}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-xs">
                    <div className="truncate" title={row.message ?? ""}>
                      {row.message ?? "-"}
                    </div>
                  </td>
                  <td className="px-3 py-2 font-mono text-[11px] text-srapi-text-tertiary">
                    <div className="truncate" title={row.request_id}>
                      {row.request_id}
                    </div>
                    <div className="mt-0.5 truncate" title={row.upstream_request_id ?? ""}>
                      {t("adminLiveErrors.attempt")} {row.attempt_no ?? 1}
                      {row.upstream_request_id ? ` / ${row.upstream_request_id}` : ""}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
