import { ADMIN_ROUTES } from "@/lib/routes";

export interface LogCorrelationIDs {
  request_id?: string | null;
  trace_id?: string | null;
}

export interface ErrorLogInvestigationLinkParams {
  account_id?: string | number | null;
  provider_id?: string | number | null;
  error_class?: string | null;
  source_endpoint?: string | null;
  model?: string | null;
}

export interface RequestEvidenceInvestigationLinkParams extends LogCorrelationIDs {
  account_id?: string | number | null;
  provider_id?: string | number | null;
  error_class?: string | null;
  source_endpoint?: string | null;
  model?: string | null;
  start?: string | null;
  end?: string | null;
}

export interface SchedulerDecisionInvestigationLinkParams extends LogCorrelationIDs {
  account_id?: string | number | null;
  provider_id?: string | number | null;
  model?: string | null;
  source_endpoint?: string | null;
  start?: string | null;
  end?: string | null;
}

export interface AdminAccountHrefParams {
  account_id?: string | number | null;
  provider_id?: string | number | null;
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

/** Build an Error logs link for an ops alert or aggregate investigation. */
export function adminErrorInvestigationHref(params: ErrorLogInvestigationLinkParams): string | null {
  const query = new URLSearchParams();
  query.set("tab", "error");
  const search = clean(params.source_endpoint);
  if (search) query.set("q", search);
  setIfPresent(query, "f_account", params.account_id);
  setIfPresent(query, "f_provider", params.provider_id);
  setIfPresent(query, "f_error_class", params.error_class);
  setIfPresent(query, "f_model", params.model);
  const hasFilter = Boolean(
    query.get("q") ||
      query.get("f_account") ||
      query.get("f_provider") ||
      query.get("f_error_class") ||
      query.get("f_model"),
  );
  return hasFilter ? `${ADMIN_ROUTES.logs}?${query.toString()}` : null;
}

/** Build a filtered Request evidence link for a request investigation. */
export function adminRequestEvidenceHref(params: RequestEvidenceInvestigationLinkParams): string | null {
  const query = new URLSearchParams();
  query.set("tab", "request-evidence");
  setIfPresent(query, "f_request_id", params.request_id);
  setIfPresent(query, "f_account_id", params.account_id);
  setIfPresent(query, "f_provider_id", params.provider_id);
  setIfPresent(query, "f_error_class", params.error_class);
  setIfPresent(query, "f_source_endpoint", params.source_endpoint);
  setIfPresent(query, "f_model", params.model);
  setIfPresent(query, "f_start", params.start);
  setIfPresent(query, "f_end", params.end);
  return hasRequestEvidenceFilter(query) ? `${ADMIN_ROUTES.logs}?${query.toString()}` : null;
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

/** Build a filtered Scheduler decisions link for request or routing investigation. */
export function adminSchedulerDecisionsHref(params: SchedulerDecisionInvestigationLinkParams): string | null {
  const query = new URLSearchParams();
  query.set("tab", "scheduler-decisions");
  setIfPresent(query, "f_request_id", params.request_id);
  setIfPresent(query, "f_account_id", params.account_id);
  setIfPresent(query, "f_provider_id", params.provider_id);
  setIfPresent(query, "f_model", params.model);
  setIfPresent(query, "f_source_endpoint", params.source_endpoint);
  setIfPresent(query, "f_start", params.start);
  setIfPresent(query, "f_end", params.end);
  return hasSchedulerDecisionFilter(query) ? `${ADMIN_ROUTES.ops}?${query.toString()}` : null;
}

/** Build an account-health link that opens the exact account when an id is available. */
export function adminAccountsHealthHref(params: AdminAccountHrefParams): string {
  const query = new URLSearchParams();
  query.set("view", "health");
  setIfPresent(query, "f_providerId", params.provider_id);
  setIfPresent(query, "f_accountId", params.account_id);
  return `${ADMIN_ROUTES.accounts}?${query.toString()}`;
}

/** Build a provider-list link scoped by provider id when available. */
export function adminProvidersHref(providerID?: string | number | null): string {
  const id = clean(providerID);
  return id ? `${ADMIN_ROUTES.providers}?q=${encodeURIComponent(id)}` : ADMIN_ROUTES.providers;
}

function firstCorrelation(params: LogCorrelationIDs): string {
  return clean(params.request_id) || clean(params.trace_id);
}

function hasCorrelation(query: URLSearchParams): boolean {
  return Boolean(query.get("f_request_id"));
}

function hasRequestEvidenceFilter(query: URLSearchParams): boolean {
  return Boolean(
    query.get("f_request_id") ||
      query.get("f_account_id") ||
      query.get("f_provider_id") ||
      query.get("f_error_class") ||
      query.get("f_source_endpoint") ||
      query.get("f_model") ||
      query.get("f_start") ||
      query.get("f_end"),
  );
}

function hasSchedulerDecisionFilter(query: URLSearchParams): boolean {
  return Boolean(
    query.get("f_request_id") ||
      query.get("f_account_id") ||
      query.get("f_provider_id") ||
      query.get("f_model") ||
      query.get("f_source_endpoint") ||
      query.get("f_start") ||
      query.get("f_end"),
  );
}

function setIfPresent(query: URLSearchParams, key: string, value?: string | number | null): void {
  const normalized = clean(value);
  if (normalized) query.set(key, normalized);
}

function clean(value?: string | number | null): string {
  if (value === null || value === undefined) return "";
  return String(value).trim();
}
