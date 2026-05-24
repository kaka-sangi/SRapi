import { describe, expect, it } from "vitest";
import {
  buildCreateOpsSloBody,
  buildUpdateOpsSloBody,
  emptyOpsSloForm,
  opsSloFormFromDefinition,
  toggleErrorOwner,
} from "@/lib/admin-ops-slo-form";
import type { OpsSloDefinition } from "../../../../packages/sdk/typescript/src/types.gen";

describe("admin-ops-slo-form", () => {
  it("builds an availability SLO with multi-window burn-rate thresholds", () => {
    const form = {
      ...emptyOpsSloForm(),
      name: "Chat Availability",
      objective: "99.5",
      sourceEndpoint: "/v1/chat/completions",
      model: "gpt-4o-mini",
    };

    const body = buildCreateOpsSloBody(form);

    expect(body).toMatchObject({
      name: "Chat Availability",
      sli_type: "availability",
      objective: 99.5,
      window_days: 28,
      status: "active",
      filter: {
        source_endpoint: "/v1/chat/completions",
        model: "gpt-4o-mini",
        error_owner_exclude: ["client", "business"],
      },
      alert_policy: {
        name: "multi_window_burn_rate",
      },
    });
    expect(body.alert_policy?.thresholds).toHaveLength(3);
    expect(body.alert_policy?.thresholds[0]).toMatchObject({
      severity: "critical",
      short_window_seconds: 300,
      long_window_seconds: 3600,
    });
  });

  it("round-trips stored ratio objective as editable percent", () => {
    const definition: OpsSloDefinition = {
      id: "slo-1",
      name: "Gateway Quality",
      sli_type: "quality",
      objective: 0.995,
      window_days: 28,
      status: "active",
      filter: {
        source_endpoint: "/v1/responses",
        model: "",
        error_owner_exclude: ["client"],
      },
      alert_policy: {
        name: "multi_window_burn_rate",
        thresholds: [],
      },
      created_at: "2026-05-24T00:00:00Z",
      updated_at: "2026-05-24T00:00:00Z",
    };

    expect(opsSloFormFromDefinition(definition)).toMatchObject({
      objective: "99.5",
      sliType: "quality",
      sourceEndpoint: "/v1/responses",
    });
  });

  it("validates threshold JSON shape", () => {
    const form = {
      ...emptyOpsSloForm(),
      name: "Bad SLO",
      thresholdsJson: '[{"severity":"critical","burn_rate":0}]',
    };

    expect(() => buildCreateOpsSloBody(form)).toThrow(
      "Burn-rate threshold 1 contains invalid numeric values.",
    );
  });

  it("reports invalid threshold JSON explicitly", () => {
    const form = {
      ...emptyOpsSloForm(),
      name: "Bad SLO",
      thresholdsJson: "[not-json",
    };

    expect(() => buildCreateOpsSloBody(form)).toThrow(
      "Burn-rate thresholds must be valid JSON.",
    );
  });

  it("omits create-only SLI type from update payloads", () => {
    const form = {
      ...emptyOpsSloForm(),
      name: "Gateway Availability",
      sliType: "latency" as const,
    };

    expect(buildUpdateOpsSloBody(form)).not.toHaveProperty("sli_type");
  });

  it("toggles error owner exclusions without duplicates", () => {
    expect(toggleErrorOwner(["client"], "client", true)).toEqual(["client"]);
    expect(toggleErrorOwner(["client"], "business", true)).toEqual(["client", "business"]);
    expect(toggleErrorOwner(["client", "business"], "client", false)).toEqual(["business"]);
  });
});
