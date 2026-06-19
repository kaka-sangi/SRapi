import { describe, expect, it } from "vitest";
import { buildErrorLogTriage } from "@/lib/admin-error-log-triage";

describe("buildErrorLogTriage", () => {
  it("prioritizes scheduler and scope evidence for routing failures", () => {
    const triage = buildErrorLogTriage({
      request_id: " req-detail ",
      trace_id: "trace-detail",
      account_id: "12",
      provider_id: "3",
      source_endpoint: "/v1/chat/completions",
      model: "gpt-4o-mini",
      status_code: 503,
      error_class: "no_available_account",
      error_phase: "routing",
      error_owner: "scheduler",
      occurred_at: "2026-06-18T10:00:00Z",
    });

    expect(triage.steps).toEqual([
      "inspectRequestEvidence",
      "inspectSchedulerDecision",
      "checkAccountHealth",
      "validateRoutingScope",
    ]);
    expect(triage.links).toEqual([
      { kind: "systemLogs", href: "/admin/ops/system-logs?f_request_id=req-detail" },
      {
        kind: "requestEvidence",
        href: "/admin/logs?tab=request-evidence&f_request_id=req-detail&f_account_id=12&f_provider_id=3&f_error_class=no_available_account&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_model=gpt-4o-mini&f_start=2026-06-18T09%3A55%3A00.000Z&f_end=2026-06-18T10%3A05%3A00.000Z",
      },
      {
        kind: "schedulerDecision",
        href: "/admin/ops?tab=scheduler-decisions&f_request_id=req-detail&f_account_id=12&f_provider_id=3&f_model=gpt-4o-mini&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_start=2026-06-18T09%3A55%3A00.000Z&f_end=2026-06-18T10%3A05%3A00.000Z",
      },
      { kind: "accountHealth", href: "/admin/accounts?view=health&f_providerId=3&f_accountId=12" },
      { kind: "requestDumps", href: "/admin/logs?tab=request-files&f_request_id=req-detail" },
    ]);
  });

  it("routes rate limits to quota checks before provider-network checks", () => {
    const triage = buildErrorLogTriage({
      request_id: "req-429",
      provider_id: "3",
      status_code: 429,
      error_class: "rate_limited",
      error_phase: "upstream",
      error_owner: "provider",
    });

    expect(triage.steps).toEqual([
      "inspectRequestEvidence",
      "checkAccountHealth",
      "checkQuotaOrRateLimit",
      "validateRoutingScope",
    ]);
  });

  it("routes upstream 5xx failures to provider and account-health evidence", () => {
    const triage = buildErrorLogTriage({
      request_id: "req-503",
      account_id: "12",
      provider_id: "3",
      status_code: 503,
      error_class: "upstream_error",
      error_phase: "upstream",
      error_owner: "provider",
    });

    expect(triage.steps).toEqual([
      "inspectRequestEvidence",
      "checkAccountHealth",
      "checkProviderNetwork",
      "validateRoutingScope",
    ]);
    expect(triage.links.map((link) => link.kind)).toEqual([
      "systemLogs",
      "requestEvidence",
      "schedulerDecision",
      "accountHealth",
      "requestDumps",
    ]);
  });

  it("routes provider auth failures to credential checks", () => {
    const triage = buildErrorLogTriage({
      status_code: 401,
      error_class: "auth_failed",
      error_owner: "provider",
    });

    expect(triage.steps).toEqual(["checkCredentials"]);
  });
});
