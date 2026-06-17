"use client";

import { configuredApiBaseUrl, getStoredCSRFToken } from "./_shared";

// Descriptor projection mirrors the Go FileDescriptor with the on-disk
// metadata the admin list needs. The shape is set by the Go handler
// (runtime_admin_request_log_files_handlers.go::descriptorToJSON).
export interface RequestLogFileDescriptor {
  name: string;
  size: number;
  created_at: string;
  request_id: string;
  is_error_only: boolean;
}

export interface RequestLogFilesQuery {
  request_id?: string;
  error_only?: boolean;
  from?: string;
  to?: string;
  limit?: number;
}

// TODO(sdk-gap): the request-log-files endpoints below are not in the
// generated SDK yet (the request_log_files module ships file-based capture
// that lives outside the OpenAPI surface). They stay raw fetches — auth +
// base URL via the shared sdk-client helpers — until generated. The shape
// matches the Go handler's inline {data, request_id} envelope.
export const requestLogFilesApi = {
  async list(
    query?: RequestLogFilesQuery,
  ): Promise<RequestLogFileDescriptor[]> {
    const base = configuredApiBaseUrl();
    const params = new URLSearchParams();
    if (query?.request_id) params.set("request_id", query.request_id);
    if (query?.error_only) params.set("error_only", "true");
    if (query?.from) params.set("from", query.from);
    if (query?.to) params.set("to", query.to);
    if (query?.limit) params.set("limit", String(query.limit));
    const qs = params.toString();
    const res = await fetch(
      `${base}/api/v1/admin/request-log-files${qs ? `?${qs}` : ""}`,
      {
        credentials: "include",
        headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
      },
    );
    if (!res.ok) throw new Error("Failed to list request log files");
    const json = (await res.json()) as { data?: RequestLogFileDescriptor[] };
    return json.data ?? [];
  },

  async get(name: string): Promise<RequestLogFileDescriptor> {
    const base = configuredApiBaseUrl();
    const res = await fetch(
      `${base}/api/v1/admin/request-log-files/${encodeURIComponent(name)}`,
      {
        credentials: "include",
        headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
      },
    );
    if (!res.ok) throw new Error("Failed to load request log file");
    const json = (await res.json()) as { data?: RequestLogFileDescriptor };
    if (!json.data) throw new Error("Empty response");
    return json.data;
  },

  // Download returns the raw text content of the captured dump so the
  // detail dialog can render it inline + offer a save-to-file button.
  async download(name: string): Promise<string> {
    const base = configuredApiBaseUrl();
    const res = await fetch(
      `${base}/api/v1/admin/request-log-files/${encodeURIComponent(name)}/download`,
      {
        credentials: "include",
        headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
      },
    );
    if (!res.ok) throw new Error("Failed to download request log file");
    return res.text();
  },

  async remove(name: string): Promise<void> {
    const base = configuredApiBaseUrl();
    const res = await fetch(
      `${base}/api/v1/admin/request-log-files/${encodeURIComponent(name)}`,
      {
        method: "DELETE",
        credentials: "include",
        headers: { "X-CSRF-Token": getStoredCSRFToken() ?? "" },
      },
    );
    if (!res.ok) throw new Error("Failed to delete request log file");
  },
};
