import { describe, expect, it } from "vitest";
import { adminErrorLogsHref, adminRequestDumpsHref } from "@/lib/admin-log-links";

describe("admin log evidence links", () => {
  it("builds an error-log search link from request id first", () => {
    expect(adminErrorLogsHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/logs?tab=error&q=req_123",
    );
  });

  it("falls back to trace id for error-log search", () => {
    expect(adminErrorLogsHref({ trace_id: "trace_456" })).toBe(
      "/admin/logs?tab=error&q=trace_456",
    );
  });

  it("builds a request-dumps link only when request id is present", () => {
    expect(adminRequestDumpsHref({ request_id: "req_123", trace_id: "trace_456" })).toBe(
      "/admin/logs?tab=request-files&f_request_id=req_123",
    );
    expect(adminRequestDumpsHref({ trace_id: "trace_456" })).toBeNull();
  });

  it("omits links without correlation ids", () => {
    expect(adminErrorLogsHref({})).toBeNull();
    expect(adminRequestDumpsHref({})).toBeNull();
  });
});
