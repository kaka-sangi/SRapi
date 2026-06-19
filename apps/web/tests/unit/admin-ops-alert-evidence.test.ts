import { describe, expect, it } from "vitest";
import {
  buildOpsAlertEvidenceLinks,
  buildOpsAlertRunbookSteps,
} from "@/lib/admin-ops-alert-evidence";

describe("buildOpsAlertEvidenceLinks", () => {
  it("builds scoped alert evidence links from SLO/rule details", () => {
    expect(
      buildOpsAlertEvidenceLinks({
        source_endpoint: "/v1/chat/completions",
        model: "gpt-ops",
        provider_id: 7,
        error_class: "timeout",
        window_start: "2026-06-18T10:00:00Z",
        window_end: "2026-06-18T10:05:00Z",
      }),
    ).toEqual({
      errorLogs:
        "/admin/logs?tab=error&q=%2Fv1%2Fchat%2Fcompletions&f_provider=7&f_error_class=timeout&f_model=gpt-ops",
      requestEvidence:
        "/admin/logs?tab=request-evidence&f_provider_id=7&f_error_class=timeout&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_model=gpt-ops&f_start=2026-06-18T10%3A00%3A00Z&f_end=2026-06-18T10%3A05%3A00Z",
      schedulerDecision:
        "/admin/ops?tab=scheduler-decisions&f_provider_id=7&f_model=gpt-ops&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_start=2026-06-18T10%3A00%3A00Z&f_end=2026-06-18T10%3A05%3A00Z",
      accountHealth: "/admin/accounts?view=health&f_providerId=7",
    });
  });

  it("builds request, scheduler, and account links when ids are present", () => {
    expect(
      buildOpsAlertEvidenceLinks({
        request_id: "req_123",
        account_id: "acct-1",
        provider_id: "provider-1",
        model: "gpt-ops",
      }),
    ).toEqual({
      errorLogs: "/admin/logs?tab=error&f_account=acct-1&f_provider=provider-1&f_model=gpt-ops",
      requestEvidence:
        "/admin/logs?tab=request-evidence&f_request_id=req_123&f_account_id=acct-1&f_provider_id=provider-1&f_model=gpt-ops",
      schedulerDecision:
        "/admin/ops?tab=scheduler-decisions&f_request_id=req_123&f_account_id=acct-1&f_provider_id=provider-1&f_model=gpt-ops",
      accountHealth: "/admin/accounts?view=health&f_providerId=provider-1&f_accountId=acct-1",
    });
  });

  it("accepts common camelCase aliases and omits empty links", () => {
    expect(
      buildOpsAlertEvidenceLinks({
        requestId: " req_alias ",
        sourceEndpoint: "/v1/responses",
        providerId: 3,
        errorClass: "upstream_error",
      }),
    ).toEqual({
      errorLogs: "/admin/logs?tab=error&q=%2Fv1%2Fresponses&f_provider=3&f_error_class=upstream_error",
      requestEvidence:
        "/admin/logs?tab=request-evidence&f_request_id=req_alias&f_provider_id=3&f_error_class=upstream_error&f_source_endpoint=%2Fv1%2Fresponses",
      schedulerDecision:
        "/admin/ops?tab=scheduler-decisions&f_request_id=req_alias&f_provider_id=3&f_source_endpoint=%2Fv1%2Fresponses",
      accountHealth: "/admin/accounts?view=health&f_providerId=3",
    });

    expect(buildOpsAlertEvidenceLinks({ observed_value: 0.5 })).toEqual({
      errorLogs: null,
      requestEvidence: null,
      schedulerDecision: null,
      accountHealth: null,
    });
  });
});

describe("buildOpsAlertRunbookSteps", () => {
  it("prioritizes request, scheduler, account, provider-network, rule, and scope checks", () => {
    expect(
      buildOpsAlertRunbookSteps({
        request_id: "req_123",
        account_id: "acct-1",
        provider_id: "provider-1",
        source_endpoint: "/v1/chat/completions",
        model: "gpt-ops",
        error_class: "timeout",
        rule_id: "rule.7",
        metric_type: "error_rate",
      }),
    ).toEqual([
      "openErrorLogs",
      "inspectRequestEvidence",
      "inspectSchedulerDecision",
      "checkAccountHealth",
      "checkProviderNetwork",
      "reviewAlertRule",
      "validateRoutingScope",
    ]);
  });

  it("adds quota/rate-limit guidance for provider throttling alerts", () => {
    expect(
      buildOpsAlertRunbookSteps({
        provider_id: "provider-1",
        error_class: "rate_limited",
      }),
    ).toEqual([
      "openErrorLogs",
      "inspectRequestEvidence",
      "inspectSchedulerDecision",
      "checkAccountHealth",
      "checkQuotaOrRateLimit",
      "validateRoutingScope",
    ]);
  });

  it("adds credential guidance for auth-related alerts", () => {
    expect(buildOpsAlertRunbookSteps({ errorClass: "session_invalid" })).toEqual([
      "openErrorLogs",
      "inspectRequestEvidence",
      "checkCredentials",
    ]);
  });

  it("recognizes SLO burn-rate details without request-level evidence", () => {
    expect(
      buildOpsAlertRunbookSteps({
        slo_id: "slo-1",
        source_endpoint: "/v1/responses",
        long_burn_rate: 12,
      }),
    ).toEqual([
      "openErrorLogs",
      "inspectRequestEvidence",
      "inspectSchedulerDecision",
      "reviewSloBurnRate",
      "validateRoutingScope",
    ]);
  });

  it("falls back to alert-rule review when details contain no diagnostic fields", () => {
    expect(buildOpsAlertRunbookSteps({ observed_value: 0.5 })).toEqual(["reviewAlertRule"]);
  });
});
