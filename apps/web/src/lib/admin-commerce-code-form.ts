import type {
  BatchGenerateRedeemCodesRequest,
  CreatePromoCodeRequest,
  CreateRedeemCodeRequest,
  PromoCode,
  PromoCodeStatus,
  PromoDiscountType,
  RedeemCode,
  RedeemCodeType,
  UpdatePromoCodeRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";

export interface RedeemCodeFormState {
  code: string;
  type: RedeemCodeType;
  value: string;
  currency: string;
  maxRedemptions: string;
  expiresAtLocal: string;
}

export interface RedeemBatchFormState {
  prefix: string;
  count: string;
  type: RedeemCodeType;
  value: string;
  currency: string;
  maxRedemptions: string;
  expiresAtLocal: string;
}

export interface RedeemDisableState {
  ids: string[];
  label: string;
  confirmation: string;
}

export interface PromoCodeFormState {
  code: string;
  discountType: PromoDiscountType;
  discountValue: string;
  currency: string;
  maxUses: string;
  perUserLimit: string;
  minOrderAmount: string;
  status: PromoCodeStatus;
  startsAtLocal: string;
  expiresAtLocal: string;
}

interface PromoDeleteState {
  id: string;
  code: string;
  confirmation: string;
}

export const REDEEM_CODE_TYPES: RedeemCodeType[] = ["balance", "subscription"];
export const PROMO_DISCOUNT_TYPES: PromoDiscountType[] = ["amount", "percent"];
export const PROMO_CODE_STATUSES: PromoCodeStatus[] = ["active", "disabled", "expired"];

export function emptyRedeemCodeForm(): RedeemCodeFormState {
  return {
    code: "",
    type: "balance",
    value: "0",
    currency: "USD",
    maxRedemptions: "1",
    expiresAtLocal: "",
  };
}

export function emptyRedeemBatchForm(): RedeemBatchFormState {
  return {
    prefix: "SR",
    count: "10",
    type: "balance",
    value: "0",
    currency: "USD",
    maxRedemptions: "1",
    expiresAtLocal: "",
  };
}

export function emptyPromoCodeForm(): PromoCodeFormState {
  return {
    code: "",
    discountType: "amount",
    discountValue: "0",
    currency: "USD",
    maxUses: "1",
    perUserLimit: "0",
    minOrderAmount: "",
    status: "active",
    startsAtLocal: "",
    expiresAtLocal: "",
  };
}

export function promoFormFromCode(promo: PromoCode): PromoCodeFormState {
  return {
    code: promo.code,
    discountType: promo.discount_type,
    discountValue: promo.discount_value,
    currency: promo.currency,
    maxUses: String(promo.max_uses),
    perUserLimit: String(promo.per_user_limit ?? 0),
    minOrderAmount: promo.min_order_amount ?? "",
    status: promo.status,
    startsAtLocal: isoToLocalDateTime(promo.starts_at),
    expiresAtLocal: isoToLocalDateTime(promo.expires_at),
  };
}

export function buildCreateRedeemCodeBody(form: RedeemCodeFormState): CreateRedeemCodeRequest {
  const body: CreateRedeemCodeRequest = {
    code: requiredText(form.code, "Code"),
    type: form.type,
    value: parseDecimalString(form.value, "Value"),
    currency: requiredText(form.currency, "Currency").toUpperCase(),
    max_redemptions: parsePositiveInteger(form.maxRedemptions, "Max redemptions"),
  };
  const expiresAt = localDateTimeToIso(form.expiresAtLocal, "Expires at");
  if (expiresAt) {
    body.expires_at = expiresAt;
  }
  return body;
}

export function buildBatchGenerateRedeemCodesBody(
  form: RedeemBatchFormState,
): BatchGenerateRedeemCodesRequest {
  const body: BatchGenerateRedeemCodesRequest = {
    count: parseBoundedInteger(form.count, "Count", 1, 500),
    type: form.type,
    value: parseDecimalString(form.value, "Value"),
    currency: requiredText(form.currency, "Currency").toUpperCase(),
    max_redemptions: parsePositiveInteger(form.maxRedemptions, "Max redemptions"),
  };
  const prefix = optionalText(form.prefix);
  const expiresAt = localDateTimeToIso(form.expiresAtLocal, "Expires at");
  if (prefix) {
    body.prefix = prefix;
  }
  if (expiresAt) {
    body.expires_at = expiresAt;
  }
  return body;
}

export function buildPromoCodeBody(
  form: PromoCodeFormState,
): CreatePromoCodeRequest | UpdatePromoCodeRequest {
  const body: CreatePromoCodeRequest = {
    code: requiredText(form.code, "Code"),
    discount_type: form.discountType,
    discount_value: parsePromoDiscountValue(form.discountType, form.discountValue),
    currency: requiredText(form.currency, "Currency").toUpperCase(),
    max_uses: parsePositiveInteger(form.maxUses, "Max uses"),
    per_user_limit: parseNonNegativeInteger(form.perUserLimit, "Per-user limit"),
    status: form.status,
  };
  const minOrderAmount = optionalText(form.minOrderAmount);
  if (minOrderAmount) {
    body.min_order_amount = parseDecimalString(minOrderAmount, "Minimum order amount");
  }
  const startsAt = localDateTimeToIso(form.startsAtLocal, "Starts at");
  const expiresAt = localDateTimeToIso(form.expiresAtLocal, "Expires at");
  if (startsAt) {
    body.starts_at = startsAt;
  }
  if (expiresAt) {
    body.expires_at = expiresAt;
  }
  return body;
}

function redeemDisableStateFromCode(code: RedeemCode): RedeemDisableState {
  return { ids: [code.id], label: code.code, confirmation: "" };
}

export function redeemDisableStateFromSelection(codes: RedeemCode[]): RedeemDisableState {
  const activeCodes = codes.filter((code) => code.status === "active");
  return {
    ids: activeCodes.map((code) => code.id),
    label: `DISABLE ${activeCodes.length} CODES`,
    confirmation: "",
  };
}

function canConfirmRedeemDisable(state: RedeemDisableState | null): boolean {
  return Boolean(state?.ids.length && state.confirmation.trim() === state.label);
}

function promoDeleteStateFromCode(promo: PromoCode): PromoDeleteState {
  return { id: promo.id, code: promo.code, confirmation: "" };
}

function canDeletePromoCode(state: PromoDeleteState | null): boolean {
  return Boolean(state?.id && state.confirmation.trim() === state.code);
}

function parsePromoDiscountValue(type: PromoDiscountType, value: string): string {
  const decimal = parseDecimalString(value, "Discount value");
  if (type === "percent" && Number(decimal) > 100) {
    throw new Error("Percent discount must be between 0 and 100.");
  }
  return decimal;
}

function parseDecimalString(value: string, fieldName: string): string {
  const normalized = requiredText(value, fieldName);
  if (!/^[0-9]+(\.[0-9]+)?$/.test(normalized)) {
    throw new Error(`${fieldName} must be a non-negative decimal string.`);
  }
  return normalized;
}

function parsePositiveInteger(value: string, fieldName: string): number {
  return parseBoundedInteger(value, fieldName, 1, Number.MAX_SAFE_INTEGER);
}

function parseNonNegativeInteger(value: string, fieldName: string): number {
  return parseBoundedInteger(value, fieldName, 0, Number.MAX_SAFE_INTEGER);
}

function parseBoundedInteger(value: string, fieldName: string, min: number, max: number): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < min || parsed > max) {
    throw new Error(`${fieldName} must be an integer between ${min} and ${max}.`);
  }
  return parsed;
}

function localDateTimeToIso(value: string, fieldName: string): string | undefined {
  const trimmed = value.trim();
  if (!trimmed) {
    return undefined;
  }
  const date = new Date(trimmed);
  if (Number.isNaN(date.getTime())) {
    throw new Error(`${fieldName} must be a valid date and time.`);
  }
  return date.toISOString();
}

function isoToLocalDateTime(value?: string): string {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const offsetMs = date.getTimezoneOffset() * 60 * 1000;
  return new Date(date.getTime() - offsetMs).toISOString().slice(0, 16);
}

function optionalText(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed ? trimmed : undefined;
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
