import { describe, expect, it } from "vitest";
import {
  buildBatchGenerateRedeemCodesBody,
  buildCreateRedeemCodeBody,
  buildPromoCodeBody,
  canConfirmRedeemDisable,
  canDeletePromoCode,
  emptyPromoCodeForm,
  emptyRedeemBatchForm,
  emptyRedeemCodeForm,
  redeemDisableStateFromSelection,
} from "@/lib/admin-commerce-code-form";
import type { RedeemCode } from "../../../../packages/sdk/typescript/src/types.gen";

describe("admin-commerce-code-form", () => {
  it("builds a create redeem-code payload without leaking defaults into secrets", () => {
    const body = buildCreateRedeemCodeBody({
      ...emptyRedeemCodeForm(),
      code: "WELCOME-100",
      value: "100.50",
      currency: "usd",
      maxRedemptions: "2",
    });

    expect(body).toMatchObject({
      code: "WELCOME-100",
      type: "balance",
      value: "100.50",
      currency: "USD",
      max_redemptions: 2,
    });
  });

  it("limits batch redeem-code generation to a bounded production range", () => {
    expect(
      buildBatchGenerateRedeemCodesBody({
        ...emptyRedeemBatchForm(),
        prefix: "LAUNCH",
        count: "25",
        value: "5",
      }),
    ).toMatchObject({ prefix: "LAUNCH", count: 25, value: "5" });

    expect(() =>
      buildBatchGenerateRedeemCodesBody({ ...emptyRedeemBatchForm(), count: "501" }),
    ).toThrow("Count must be an integer between 1 and 500.");
  });

  it("validates promo percentage discounts", () => {
    expect(
      buildPromoCodeBody({
        ...emptyPromoCodeForm(),
        code: "SAVE20",
        discountType: "percent",
        discountValue: "20",
        maxUses: "10",
      }),
    ).toMatchObject({
      code: "SAVE20",
      discount_type: "percent",
      discount_value: "20",
      max_uses: 10,
    });

    expect(() =>
      buildPromoCodeBody({
        ...emptyPromoCodeForm(),
        code: "BAD",
        discountType: "percent",
        discountValue: "120",
      }),
    ).toThrow("Percent discount must be between 0 and 100.");
  });

  it("requires exact confirmation for destructive code actions", () => {
    expect(canConfirmRedeemDisable({ ids: ["1"], label: "WELCOME", confirmation: "WELCOME" })).toBe(true);
    expect(canConfirmRedeemDisable({ ids: ["1"], label: "WELCOME", confirmation: "welcome" })).toBe(false);
    expect(canDeletePromoCode({ id: "1", code: "SAVE20", confirmation: "SAVE20" })).toBe(true);
    expect(canDeletePromoCode({ id: "1", code: "SAVE20", confirmation: "" })).toBe(false);
  });

  it("builds batch disable confirmation state from active selected codes", () => {
    const codes = [
      { id: "1", code: "A", status: "active" },
      { id: "2", code: "B", status: "disabled" },
      { id: "3", code: "C", status: "active" },
    ] as RedeemCode[];

    expect(redeemDisableStateFromSelection(codes)).toEqual({
      ids: ["1", "3"],
      label: "DISABLE 2 CODES",
      confirmation: "",
    });
  });
});
