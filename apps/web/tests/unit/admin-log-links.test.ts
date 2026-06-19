import { describe, expect, it } from "vitest";
import {
  adminErrorInvestigationHref,
  adminErrorLogsHref,
  adminAccountsHealthHref,
  adminProvidersHref,
  adminRequestDumpsHref,
  adminRequestEvidenceHref,
  adminSchedulerDecisionsHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";

describe("admin log evidence links", () => {
  it("builds an error-log search link from request id first", () => {
    expect(adminErrorLogsHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/logs?tab=error&q=req_123",
    );
  });

  it("builds an error-log investigation link from error class and model", () => {
    expect(
      adminErrorInvestigationHref({
        error_class: "server_bad",
        source_endpoint: "/v1/chat/completions",
        model: "gpt-ops",
      }),
    ).toBe("/admin/logs?tab=error&q=%2Fv1%2Fchat%2Fcompletions&f_error_class=server_bad&f_model=gpt-ops");
  });

  it("builds exact account and provider filters for account-health investigation", () => {
    expect(
      adminErrorInvestigationHref({
        account_id: "12",
        provider_id: 3,
        error_class: "rate_limited",
      }),
    ).toBe("/admin/logs?tab=error&f_account=12&f_provider=3&f_error_class=rate_limited");
  });

  it("falls back to source endpoint for error-log investigation search", () => {
    expect(adminErrorInvestigationHref({ source_endpoint: "/v1/messages" })).toBe(
      "/admin/logs?tab=error&q=%2Fv1%2Fmessages",
    );
    expect(adminErrorInvestigationHref({})).toBeNull();
  });

  it("falls back to trace id for error-log search", () => {
    expect(adminErrorLogsHref({ trace_id: "trace_456" })).toBe("/admin/logs?tab=error&q=trace_456");
  });

  it("builds a request-dumps link only when request id is present", () => {
    expect(adminRequestDumpsHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/logs?tab=request-files&f_request_id=req_123",
    );
    expect(adminRequestDumpsHref({ trace_id: "trace_456" })).toBeNull();
  });

  it("builds a request-evidence link from request id and ops filters", () => {
    expect(adminRequestEvidenceHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/logs?tab=request-evidence&f_request_id=req_123",
    );
    expect(
      adminRequestEvidenceHref({
        account_id: "12",
        provider_id: 3,
        error_class: "rate_limited",
        source_endpoint: "/v1/chat/completions",
        model: "gpt-ops",
        start: "2026-06-18T10:00:00Z",
        end: "2026-06-18T10:05:00Z",
      }),
    ).toBe(
      "/admin/logs?tab=request-evidence&f_account_id=12&f_provider_id=3&f_error_class=rate_limited&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_model=gpt-ops&f_start=2026-06-18T10%3A00%3A00Z&f_end=2026-06-18T10%3A05%3A00Z",
    );
    expect(adminRequestEvidenceHref({ trace_id: "trace_456" })).toBeNull();
  });

  it("builds a system-log exact filter link from request and trace ids", () => {
    expect(adminSystemLogsHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/ops/system-logs?f_request_id=req_123&f_trace_id=trace_456",
    );
  });

  it("builds a scheduler decisions exact filter link from request and account filters", () => {
    expect(adminSchedulerDecisionsHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/ops?tab=scheduler-decisions&f_request_id=req_123",
    );
    expect(
      adminSchedulerDecisionsHref({
        account_id: "12",
        provider_id: 3,
        model: "gpt-ops",
        source_endpoint: "/v1/chat/completions",
        start: "2026-06-18T10:00:00Z",
        end: "2026-06-18T10:05:00Z",
      }),
    ).toBe(
      "/admin/ops?tab=scheduler-decisions&f_account_id=12&f_provider_id=3&f_model=gpt-ops&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_start=2026-06-18T10%3A00%3A00Z&f_end=2026-06-18T10%3A05%3A00Z",
    );
    expect(adminSchedulerDecisionsHref({ trace_id: "trace_456" })).toBeNull();
  });

  it("builds account-health and provider investigation links", () => {
    expect(adminAccountsHealthHref({ account_id: "12", provider_id: 3 })).toBe(
      "/admin/accounts?view=health&f_providerId=3&f_accountId=12",
    );
    expect(adminAccountsHealthHref({})).toBe("/admin/accounts?view=health");
    expect(adminProvidersHref(3)).toBe("/admin/providers?q=3");
    expect(adminProvidersHref()).toBe("/admin/providers");
  });

  it("trims correlation ids before building links", () => {
    expect(adminErrorLogsHref({ request_id: "  req_123  " })).toBe(
      "/admin/logs?tab=error&q=req_123",
    );
    expect(adminSystemLogsHref({ trace_id: "  trace_456  " })).toBe(
      "/admin/ops/system-logs?f_trace_id=trace_456",
    );
    expect(adminRequestEvidenceHref({ request_id: "  req_123  " })).toBe(
      "/admin/logs?tab=request-evidence&f_request_id=req_123",
    );
    expect(adminSchedulerDecisionsHref({ request_id: "  req_123  " })).toBe(
      "/admin/ops?tab=scheduler-decisions&f_request_id=req_123",
    );
  });

  it("omits links without correlation ids", () => {
    expect(adminErrorLogsHref({})).toBeNull();
    expect(adminRequestEvidenceHref({})).toBeNull();
    expect(adminRequestDumpsHref({})).toBeNull();
    expect(adminSchedulerDecisionsHref({})).toBeNull();
    expect(adminSystemLogsHref({})).toBeNull();
  });
});
