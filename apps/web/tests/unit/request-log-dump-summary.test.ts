import { describe, expect, it } from "vitest";
import { parseRequestDumpSummary } from "@/app/admin/logs/_panels/request-log-dump-summary";

describe("parseRequestDumpSummary", () => {
  it("extracts diagnostic fields from the SRapi/CLIProxyAPI dump format", () => {
    const summary = parseRequestDumpSummary(`=== REQUEST INFO ===
Request-ID: req-123
User-ID: 42
API-Key-ID: 7
Account-ID: 9
Source-Protocol: openai-compatible
Source-Endpoint: /v1/chat/completions
Started-At: 2026-06-18T10:00:00Z

=== REQUEST 1 ===
POST https://upstream.invalid/v1/chat/completions

{}

=== RESPONSE 1 ===
Status: 429

rate limited

=== REQUEST 2 ===
POST https://upstream.invalid/v1/chat/completions

{}

=== RESPONSE 2 ===
Status: 200

{}

=== SUMMARY ===
Success: true
Status: 200
Latency-MS: 124
`);

    expect(summary).toMatchObject({
      requestID: "req-123",
      userID: "42",
      apiKeyID: "7",
      accountID: "9",
      sourceProtocol: "openai-compatible",
      sourceEndpoint: "/v1/chat/completions",
      startedAt: "2026-06-18T10:00:00Z",
      success: true,
      statusCode: 200,
      latencyMS: 124,
      attemptCount: 2,
      responseCount: 2,
      hasSummary: true,
    });
  });

  it("keeps partial evidence when the summary block is missing", () => {
    const summary = parseRequestDumpSummary(`=== REQUEST INFO ===
Request-ID: req-midcut

=== REQUEST 3 ===
POST https://upstream.invalid/v1/responses
`);

    expect(summary.requestID).toBe("req-midcut");
    expect(summary.success).toBeUndefined();
    expect(summary.statusCode).toBeUndefined();
    expect(summary.attemptCount).toBe(3);
    expect(summary.responseCount).toBe(0);
    expect(summary.hasSummary).toBe(false);
  });

  it("extracts failed outcomes and error classes", () => {
    const summary = parseRequestDumpSummary(`=== RESPONSE 1 ===
Status: 502

=== SUMMARY ===
Success: false
Error-Class: server_bad
Status: 502
Latency-MS: 3001
`);

    expect(summary.success).toBe(false);
    expect(summary.errorClass).toBe("server_bad");
    expect(summary.statusCode).toBe(502);
    expect(summary.latencyMS).toBe(3001);
  });
});
