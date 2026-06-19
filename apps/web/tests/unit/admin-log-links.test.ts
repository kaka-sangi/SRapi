import { describe, expect, it } from "vitest";
import {
  adminErrorInvestigationHref,
  adminErrorLogsHref,
  adminRequestDumpsHref,
  adminRequestEvidenceHref,
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
    ).toBe("/admin/logs?tab=error&q=server_bad&f_model=gpt-ops");
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

  it("builds a request-evidence link only when request id is present", () => {
    expect(adminRequestEvidenceHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/logs?tab=request-evidence&f_request_id=req_123",
    );
    expect(adminRequestEvidenceHref({ trace_id: "trace_456" })).toBeNull();
  });

  it("builds a system-log exact filter link from request and trace ids", () => {
    expect(adminSystemLogsHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/ops/system-logs?f_request_id=req_123&f_trace_id=trace_456",
    );
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
  });

  it("omits links without correlation ids", () => {
    expect(adminErrorLogsHref({})).toBeNull();
    expect(adminRequestEvidenceHref({})).toBeNull();
    expect(adminRequestDumpsHref({})).toBeNull();
    expect(adminSystemLogsHref({})).toBeNull();
  });
});
