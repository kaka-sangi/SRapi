import { describe, expect, it } from "vitest";
import {
  buildCreatePaymentProviderBody,
  buildRefundPaymentOrderBody,
  emptyPaymentProviderForm,
  isRefundableOrder,
  refundFormFromOrder,
  sumDecimalStrings,
  sumOrderAmounts,
} from "@/lib/admin-orders-form";
import type { PaymentOrder } from "../../../../packages/sdk/typescript/src/types.gen";

const order: PaymentOrder = {
  id: "order-1",
  user_id: "user-1",
  order_no: "ord_1",
  provider_instance_id: "provider-1",
  amount: "19.99000000",
  currency: "USD",
  status: "paid",
  product_type: "balance_credit",
  product_id: "balance",
  provider_snapshot: {},
  metadata: {},
  created_at: "2026-05-24T00:00:00Z",
  updated_at: "2026-05-24T00:00:00Z",
};

describe("admin-orders-form", () => {
  it("sums decimal strings without floating point drift", () => {
    expect(sumDecimalStrings(["0.1", "0.2", "19.99000000"])).toBe("20.29");
    expect(sumOrderAmounts([{ amount: "1.00000001" }, { amount: "2.00000002" }])).toBe(
      "3.00000003",
    );
  });

  it("identifies refundable order statuses", () => {
    expect(isRefundableOrder({ status: "paid" })).toBe(true);
    expect(isRefundableOrder({ status: "fulfilled" })).toBe(true);
    expect(isRefundableOrder({ status: "pending" })).toBe(false);
    expect(isRefundableOrder({ status: "refunded" })).toBe(false);
  });

  it("builds refund payloads with optional full-refund amount", () => {
    const form = {
      ...refundFormFromOrder(order),
      amount: "",
      reason: "duplicate purchase",
    };

    expect(buildRefundPaymentOrderBody(form)).toEqual({
      id: "order-1",
      amount: undefined,
      reason: "duplicate purchase",
    });
  });

  it("rejects invalid refund amount strings", () => {
    const form = {
      ...refundFormFromOrder(order),
      amount: "-1",
    };

    expect(() => buildRefundPaymentOrderBody(form)).toThrow(
      "Refund amount must be a non-negative decimal string.",
    );
  });

  it("builds payment provider payloads with JSON config objects", () => {
    const form = {
      ...emptyPaymentProviderForm(),
      provider: "stripe",
      name: "Stripe production",
      supportedMethodsText: "card\nwallet, bank_transfer",
      configJson: '{"secret_key":"sk_live_x","webhook_secret":"whsec_x"}',
      limitsJson: '{"daily_amount":"1000.00"}',
      metadataJson: '{"owner":"billing"}',
      sortOrder: "10",
    };

    expect(buildCreatePaymentProviderBody(form)).toEqual({
      provider: "stripe",
      name: "Stripe production",
      status: "disabled",
      supported_methods: ["card", "wallet", "bank_transfer"],
      config: { secret_key: "sk_live_x", webhook_secret: "whsec_x" },
      limits: { daily_amount: "1000.00" },
      metadata: { owner: "billing" },
      sort_order: 10,
    });
  });

  it("rejects invalid payment provider config", () => {
    const form = {
      ...emptyPaymentProviderForm(),
      provider: "stripe",
      name: "Stripe production",
      configJson: "[]",
    };

    expect(() => buildCreatePaymentProviderBody(form)).toThrow(
      "Config must be a JSON object.",
    );
  });

  it("reports invalid payment provider config JSON explicitly", () => {
    const form = {
      ...emptyPaymentProviderForm(),
      provider: "stripe",
      name: "Stripe production",
      configJson: "{not-json",
    };

    expect(() => buildCreatePaymentProviderBody(form)).toThrow(
      "Config must be valid JSON.",
    );
  });

  it("requires payment provider identity fields", () => {
    expect(() => buildCreatePaymentProviderBody(emptyPaymentProviderForm())).toThrow(
      "Provider is required.",
    );
  });
});
