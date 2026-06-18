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

/** Build a filtered Request dumps link for a request investigation. */
export function adminRequestDumpsHref(params: LogCorrelationIDs): string | null {
  const query = new URLSearchParams();
  query.set("tab", "request-files");
  if (params.request_id) query.set("f_request_id", params.request_id);
  return hasCorrelation(query) ? `${ADMIN_ROUTES.logs}?${query.toString()}` : null;
}

function firstCorrelation(params: LogCorrelationIDs): string {
  return params.request_id || params.trace_id || "";
}

function hasCorrelation(query: URLSearchParams): boolean {
  return Boolean(query.get("f_request_id"));
}
