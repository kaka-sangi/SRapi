import { describe, expect, it } from "vitest";
import {
  buildCreatePricingRuleBody,
  buildCreateSubscriptionPlanBody,
  buildCreateUserSubscriptionBody,
  canConfirmPricingRuleCreate,
  createPricingRuleConfirmation,
  emptyPricingRuleForm,
  emptySubscriptionPlanForm,
  emptyUserSubscriptionForm,
} from "@/lib/admin-subscription-form";

describe("admin-subscription-form", () => {
  it("builds subscription plans without converting decimal strings to floats", () => {
    const form = {
      ...emptySubscriptionPlanForm(),
      name: "Pro",
      price: "19.99000000",
      validityDays: "30",
      entitlementsJson: '{"monthly_tokens":1000000}',
    };

    expect(buildCreateSubscriptionPlanBody(form)).toMatchObject({
      name: "Pro",
      price: "19.99000000",
      validity_days: 30,
      entitlements: { monthly_tokens: 1000000 },
      for_sale: true,
      status: "active",
    });
  });

  it("rejects non-object entitlement JSON", () => {
    const form = {
      ...emptySubscriptionPlanForm(),
      name: "Bad",
      entitlementsJson: "[]",
    };

    expect(() => buildCreateSubscriptionPlanBody(form)).toThrow(
      "Entitlements must be a JSON object.",
    );
  });

  it("reports invalid entitlement JSON explicitly", () => {
    const form = {
      ...emptySubscriptionPlanForm(),
      name: "Bad",
      entitlementsJson: "{not-json",
    };

    expect(() => buildCreateSubscriptionPlanBody(form)).toThrow(
      "Entitlements must be valid JSON.",
    );
  });

  it("builds user subscription payloads with optional ISO windows", () => {
    const form = {
      ...emptyUserSubscriptionForm("plan-1", "user-1"),
      startsAtLocal: "2026-05-24T10:30",
      expiresAtLocal: "2026-06-24T10:30",
      sourceId: "manual-grant",
    };

    const body = buildCreateUserSubscriptionBody(form);

    expect(body).toMatchObject({
      user_id: "user-1",
      plan_id: "plan-1",
      status: "active",
      source_type: "admin",
      source_id: "manual-grant",
    });
    expect(body.starts_at).toContain("2026-05-24T");
    expect(body.expires_at).toContain("2026-06-24T");
  });

  it("builds pricing rules with decimal-string token prices", () => {
    const form = {
      ...emptyPricingRuleForm("model-1", "provider-1"),
      inputPricePerMillionTokens: "1.25000000",
      outputPricePerMillionTokens: "2.50000000",
      cacheReadPricePerMillionTokens: "0.10000000",
      cacheWritePricePerMillionTokens: "0.20000000",
    };

    expect(buildCreatePricingRuleBody(form)).toMatchObject({
      model_id: "model-1",
      provider_id: "provider-1",
      input_price_per_million_tokens: "1.25000000",
      output_price_per_million_tokens: "2.50000000",
      cache_read_price_per_million_tokens: "0.10000000",
      cache_write_price_per_million_tokens: "0.20000000",
      currency: "USD",
    });
  });

  it("rejects invalid decimal strings", () => {
    const form = {
      ...emptyPricingRuleForm("model-1", "provider-1"),
      inputPricePerMillionTokens: "-1",
    };

    expect(() => buildCreatePricingRuleBody(form)).toThrow(
      "Input price must be a non-negative decimal string.",
    );
  });

  it("requires exact confirmation for pricing rule creation", () => {
    const state = createPricingRuleConfirmation({
      modelLabel: "gpt-4o-mini",
      providerLabel: "OpenAI Compatible",
    });

    expect(state.phrase).toBe("CREATE PRICING gpt-4o-mini OpenAI Compatible");
    expect(canConfirmPricingRuleCreate({ ...state, confirmation: state.phrase })).toBe(true);
    expect(canConfirmPricingRuleCreate({ ...state, confirmation: "create pricing gpt-4o-mini OpenAI Compatible" })).toBe(false);
  });
});
