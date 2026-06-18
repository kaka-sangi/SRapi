import { ADMIN_ROUTES } from "@/lib/routes";

export interface LogCorrelationIDs {
  request_id?: string | null;
  trace_id?: string | null;
}

/** Build a filtered Error logs link for a request/trace investigation. */
export function adminErrorLogsHref(params: LogCorrelationIDs): string | null {
  const query = new URLSearchParams();
  query.set("tab", "error");
  const search = firstCorrelation(params);
  if (!search) return null;
  query.set("q", search);
  return `${ADMIN_ROUTES.logs}?${query.toString()}`;
}

/** Build a filtered System logs link for a request/trace investigation. */
export function adminSystemLogsHref(params: LogCorrelationIDs): string | null {
  const query = new URLSearchParams();
  setIfPresent(query, "f_request_id", params.request_id);
  setIfPresent(query, "f_trace_id", params.trace_id);
  const search = query.toString();
  return search ? `${ADMIN_ROUTES.opsSystemLogs}?${search}` : null;
}

/** Build a filtered Request dumps link for a request investigation. */
export function adminRequestDumpsHref(params: LogCorrelationIDs): string | null {
  const query = new URLSearchParams();
  query.set("tab", "request-files");
  setIfPresent(query, "f_request_id", params.request_id);
  return hasCorrelation(query) ? `${ADMIN_ROUTES.logs}?${query.toString()}` : null;
}

function firstCorrelation(params: LogCorrelationIDs): string {
  return clean(params.request_id) || clean(params.trace_id);
}

function hasCorrelation(query: URLSearchParams): boolean {
  return Boolean(query.get("f_request_id"));
}

function setIfPresent(query: URLSearchParams, key: string, value?: string | null): void {
  const normalized = clean(value);
  if (normalized) query.set(key, normalized);
}

function clean(value?: string | null): string {
  return value?.trim() ?? "";
}
