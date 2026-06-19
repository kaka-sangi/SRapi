import { describe, expect, it } from "vitest";
import { buildOpsAlertEvidenceLinks } from "@/lib/admin-ops-alert-evidence";

describe("buildOpsAlertEvidenceLinks", () => {
  it("builds scoped alert evidence links from SLO/rule details", () => {
    expect(
      buildOpsAlertEvidenceLinks({
        source_endpoint: "/v1/chat/completions",
        model: "gpt-ops",
        provider_id: 7,
        error_class: "timeout",
      }),
    ).toEqual({
      errorLogs:
        "/admin/logs?tab=error&q=%2Fv1%2Fchat%2Fcompletions&f_provider=7&f_error_class=timeout&f_model=gpt-ops",
      requestEvidence: null,
      schedulerDecision: null,
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
      requestEvidence: "/admin/logs?tab=request-evidence&f_request_id=req_123",
      schedulerDecision: "/admin/ops?tab=scheduler-decisions&f_request_id=req_123",
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
      requestEvidence: "/admin/logs?tab=request-evidence&f_request_id=req_alias",
      schedulerDecision: "/admin/ops?tab=scheduler-decisions&f_request_id=req_alias",
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
