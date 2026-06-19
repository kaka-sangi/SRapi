import { describe, expect, it } from "vitest";
import {
  compactUpstreamErrorDiagnostic,
  parseUpstreamErrorDiagnostic,
  resolveUpstreamErrorDiagnostic,
} from "@/lib/upstream-error-diagnostic";

describe("upstream error diagnostics", () => {
  it("parses SRapi delimited upstream excerpts", () => {
    expect(
      parseUpstreamErrorDiagnostic(
        "class=rate_limited | status=429 | type=rate_limit_error | code=too_many_requests | message=quota exceeded",
      ),
    ).toEqual({
      className: "rate_limited",
      status: 429,
      type: "rate_limit_error",
      code: "too_many_requests",
      message: "quota exceeded",
      isGenericGatewayWrapper: false,
    });
  });

  it("parses JSON error envelopes", () => {
    expect(
      parseUpstreamErrorDiagnostic(
        JSON.stringify({
          error: {
            type: "overloaded_error",
            code: "server_overloaded",
            message: "provider overloaded",
          },
          status_code: 503,
        }),
      ),
    ).toEqual({
      className: undefined,
      status: 503,
      type: "overloaded_error",
      code: "server_overloaded",
      message: "provider overloaded",
      isGenericGatewayWrapper: false,
    });
  });

  it("prefers attempt evidence when the top-level body is a generic gateway wrapper", () => {
    const diagnostic = resolveUpstreamErrorDiagnostic({
      errorBodyExcerpt: JSON.stringify({
        error: {
          type: "upstream_error",
          message: "Upstream request failed",
        },
      }),
      upstreamErrors: [
        {
          body_excerpt:
            "class=upstream_error | status=503 | type=overloaded_error | message=real upstream detail",
        },
      ],
    });

    expect(diagnostic).toMatchObject({
      source: "attempt",
      status: 503,
      type: "overloaded_error",
      message: "real upstream detail",
      isGenericGatewayWrapper: false,
    });
  });

  it("ignores scheduler evidence and malformed excerpts", () => {
    expect(
      parseUpstreamErrorDiagnostic(
        JSON.stringify({
          scheduler_decision_id: 77,
          scheduler_primary_reject_reason: "capability_mismatch:responses",
        }),
      ),
    ).toBeNull();
    expect(parseUpstreamErrorDiagnostic("plain provider text")).toBeNull();
  });

  it("returns compact evidence for dense table cells", () => {
    expect(compactUpstreamErrorDiagnostic("class=network_error | message=dial tcp timeout")).toEqual({
      parts: ["network_error"],
      message: "dial tcp timeout",
    });
    expect(
      compactUpstreamErrorDiagnostic(
        JSON.stringify({ error: { type: "upstream_error", message: "Upstream request failed" } }),
      ),
    ).toBeNull();
  });
});
