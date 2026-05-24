import { describe, expect, it } from "vitest";
import {
  buildRiskControlConfig,
  canConfirmRiskControlSave,
  createRiskControlSaveConfirmation,
  createRiskControlForm,
} from "@/lib/admin-risk-control-form";
import type { RiskControlConfig } from "../../../../packages/sdk/typescript/src/types.gen";

const baseConfig: RiskControlConfig = {
  enabled: true,
  mode: "monitor",
  max_failed_requests_per_minute: 20,
  max_cost_per_day: "100.50000000",
  cooldown_seconds: 60,
  blocked_countries: ["US", "CN"],
  blocked_ips: ["192.0.2.1"],
};

describe("admin-risk-control-form", () => {
  it("round-trips risk-control config without changing decimal strings", () => {
    const form = createRiskControlForm(baseConfig);
    const config = buildRiskControlConfig(form);

    expect(config).toEqual(baseConfig);
    expect(config.max_cost_per_day).toBe("100.50000000");
  });

  it("normalizes comma and newline lists", () => {
    const form = {
      ...createRiskControlForm(baseConfig),
      blockedCountriesText: "us\njp, de",
      blockedIpsText: "192.0.2.1\n198.51.100.2, 203.0.113.5",
    };

    expect(buildRiskControlConfig(form)).toMatchObject({
      blocked_countries: ["US", "JP", "DE"],
      blocked_ips: ["192.0.2.1", "198.51.100.2", "203.0.113.5"],
    });
  });

  it("rejects negative counters", () => {
    const form = {
      ...createRiskControlForm(baseConfig),
      cooldownSeconds: "-1",
    };

    expect(() => buildRiskControlConfig(form)).toThrow(
      "Cooldown seconds must be a non-negative integer.",
    );
  });

  it("rejects non-decimal cost limits", () => {
    const form = {
      ...createRiskControlForm(baseConfig),
      maxCostPerDay: "free",
    };

    expect(() => buildRiskControlConfig(form)).toThrow(
      "Max cost per day must be a decimal string.",
    );
  });

  it("requires exact confirmation for saving risk-control config", () => {
    const state = createRiskControlSaveConfirmation();

    expect(state.phrase).toBe("SAVE RISK CONTROL CONFIG");
    expect(canConfirmRiskControlSave({ ...state, confirmation: "SAVE RISK CONTROL CONFIG" })).toBe(true);
    expect(canConfirmRiskControlSave({ ...state, confirmation: "save risk control config" })).toBe(false);
  });
});
