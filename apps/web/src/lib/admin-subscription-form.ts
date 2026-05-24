import type {
  CreateAdminPricingRuleData,
  CreateAdminSubscriptionPlanData,
  CreateAdminUserSubscriptionData,
  Id,
  SubscriptionPlanStatus,
  UserSubscriptionStatus,
} from "../../../../packages/sdk/typescript/src/types.gen";

export interface SubscriptionPlanFormState {
  name: string;
  description: string;
  price: string;
  currency: string;
  validityDays: string;
  entitlementsJson: string;
  forSale: boolean;
  sortOrder: string;
  status: SubscriptionPlanStatus;
}

export interface UserSubscriptionFormState {
  userId: Id;
  planId: Id;
  status: UserSubscriptionStatus;
  startsAtLocal: string;
  expiresAtLocal: string;
  sourceType: string;
  sourceId: string;
}

export interface PricingRuleFormState {
  modelId: Id;
  providerId: Id;
  inputPricePerMillionTokens: string;
  outputPricePerMillionTokens: string;
  cacheReadPricePerMillionTokens: string;
  cacheWritePricePerMillionTokens: string;
  currency: string;
  effectiveFromLocal: string;
  effectiveToLocal: string;
}

export interface PricingRuleCreateConfirmationState {
  modelLabel: string;
  providerLabel: string;
  phrase: string;
  confirmation: string;
}

export const SUBSCRIPTION_PLAN_STATUSES: SubscriptionPlanStatus[] = [
  "active",
  "disabled",
  "archived",
];

export const USER_SUBSCRIPTION_STATUSES: UserSubscriptionStatus[] = [
  "active",
  "expired",
  "cancelled",
  "suspended",
];

export function emptySubscriptionPlanForm(): SubscriptionPlanFormState {
  return {
    name: "",
    description: "",
    price: "0",
    currency: "USD",
    validityDays: "30",
    entitlementsJson: "{}",
    forSale: true,
    sortOrder: "0",
    status: "active",
  };
}

export function emptyUserSubscriptionForm(planId = "", userId = ""): UserSubscriptionFormState {
  return {
    userId,
    planId,
    status: "active",
    startsAtLocal: "",
    expiresAtLocal: "",
    sourceType: "admin",
    sourceId: "",
  };
}

export function emptyPricingRuleForm(modelId = "", providerId = ""): PricingRuleFormState {
  return {
    modelId,
    providerId,
    inputPricePerMillionTokens: "0",
    outputPricePerMillionTokens: "0",
    cacheReadPricePerMillionTokens: "0",
    cacheWritePricePerMillionTokens: "0",
    currency: "USD",
    effectiveFromLocal: "",
    effectiveToLocal: "",
  };
}

export function buildCreateSubscriptionPlanBody(
  form: SubscriptionPlanFormState,
): CreateAdminSubscriptionPlanData["body"] {
  return {
    name: requiredText(form.name, "Name"),
    description: optionalText(form.description),
    price: parseDecimalString(form.price, "Price"),
    currency: requiredText(form.currency, "Currency").toUpperCase(),
    validity_days: parsePositiveInteger(form.validityDays, "Validity days"),
    entitlements: parseJsonObject(form.entitlementsJson, "Entitlements"),
    for_sale: form.forSale,
    sort_order: parseInteger(form.sortOrder, "Sort order"),
    status: form.status,
  };
}

export function buildCreateUserSubscriptionBody(
  form: UserSubscriptionFormState,
): CreateAdminUserSubscriptionData["body"] {
  const body: CreateAdminUserSubscriptionData["body"] = {
    user_id: requiredText(form.userId, "User"),
    plan_id: requiredText(form.planId, "Plan"),
    status: form.status,
    source_type: optionalText(form.sourceType),
    source_id: optionalText(form.sourceId),
  };
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

export function buildCreatePricingRuleBody(
  form: PricingRuleFormState,
): CreateAdminPricingRuleData["body"] {
  return {
    model_id: requiredText(form.modelId, "Model"),
    provider_id: requiredText(form.providerId, "Provider"),
    input_price_per_million_tokens: parseDecimalString(
      form.inputPricePerMillionTokens,
      "Input price",
    ),
    output_price_per_million_tokens: parseDecimalString(
      form.outputPricePerMillionTokens,
      "Output price",
    ),
    cache_read_price_per_million_tokens: parseDecimalString(
      form.cacheReadPricePerMillionTokens,
      "Cache read price",
    ),
    cache_write_price_per_million_tokens: parseDecimalString(
      form.cacheWritePricePerMillionTokens,
      "Cache write price",
    ),
    currency: requiredText(form.currency, "Currency").toUpperCase(),
    effective_from: localDateTimeToIso(form.effectiveFromLocal, "Effective from"),
    effective_to: localDateTimeToIso(form.effectiveToLocal, "Effective to"),
  };
}

export function createPricingRuleConfirmation({
  modelLabel,
  providerLabel,
}: {
  modelLabel: string;
  providerLabel: string;
}): PricingRuleCreateConfirmationState {
  return {
    modelLabel,
    providerLabel,
    phrase: `CREATE PRICING ${modelLabel} ${providerLabel}`,
    confirmation: "",
  };
}

export function canConfirmPricingRuleCreate(
  state: PricingRuleCreateConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

function parseJsonObject(value: string, fieldName: string): Record<string, unknown> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || "{}") as unknown;
  } catch {
    throw new Error(`${fieldName} must be valid JSON.`);
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error(`${fieldName} must be a JSON object.`);
  }
  return parsed as Record<string, unknown>;
}

function parseDecimalString(value: string, fieldName: string): string {
  const normalized = requiredText(value, fieldName);
  if (!/^[0-9]+(\.[0-9]+)?$/.test(normalized)) {
    throw new Error(`${fieldName} must be a non-negative decimal string.`);
  }
  return normalized;
}

function parsePositiveInteger(value: string, fieldName: string): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 1) {
    throw new Error(`${fieldName} must be a positive integer.`);
  }
  return parsed;
}

function parseInteger(value: string, fieldName: string): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed)) {
    throw new Error(`${fieldName} must be an integer.`);
  }
  return parsed;
}

function localDateTimeToIso(value: string, fieldName: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) {
    return null;
  }
  const date = new Date(trimmed);
  if (Number.isNaN(date.getTime())) {
    throw new Error(`${fieldName} must be a valid date and time.`);
  }
  return date.toISOString();
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
