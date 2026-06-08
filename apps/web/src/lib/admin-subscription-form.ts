import type {
  CreateAdminPricingRuleData,
  UpdateAdminPricingRuleData,
  CreateAdminSubscriptionPlanData,
  CreateAdminUserSubscriptionData,
  Id,
  PricingRule,
  SubscriptionPlan,
  SubscriptionPlanStatus,
  UserSubscriptionStatus,
} from "../../../../packages/sdk/typescript/src/types.gen";

export type CostQuotaMode = "hard_cap" | "allowance";

export interface SubscriptionPlanFormState {
  name: string;
  description: string;
  price: string;
  currency: string;
  validityDays: string;
  // Structured entitlements — composed into the `entitlements` JSON on submit so
  // an admin never has to know the exact backend keys.
  allowedModels: string[];
  monthlyTokenQuota: string;
  monthlyCostQuota: string;
  costQuotaMode: CostQuotaMode;
  schedulerStrategy: string;
  accountGroupScope: string[];
  // Escape hatch for custom / future entitlement keys the structured fields
  // don't cover (kept under "Advanced" so capability is never lost).
  extraEntitlements: Record<string, unknown>;
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

export const COST_QUOTA_MODES: CostQuotaMode[] = ["hard_cap", "allowance"];

// "default" leaves scheduler_strategy unset (gateway default). The rest mirror the
// Go scheduler registry (apps/api/.../scheduling).
export const SCHEDULER_STRATEGIES = [
  "default",
  "balanced",
  "cost_saver",
  "priority",
  "priority_weight",
] as const;

// Entitlement keys the structured fields own; everything else round-trips through
// `extraEntitlements` so custom/future keys survive an edit.
const KNOWN_ENTITLEMENT_KEYS = [
  "allowed_models",
  "monthly_token_quota",
  "monthly_cost_quota",
  "cost_quota_mode",
  "account_group_scope",
  "scheduler_strategy",
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
    allowedModels: [],
    monthlyTokenQuota: "",
    monthlyCostQuota: "",
    costQuotaMode: "hard_cap",
    schedulerStrategy: "default",
    accountGroupScope: [],
    extraEntitlements: {},
    forSale: true,
    sortOrder: "0",
    status: "active",
  };
}

// Split a stored plan's entitlements JSON back into the structured form fields,
// dropping known keys into their fields and the remainder into extraEntitlements.
export function subscriptionPlanFormFromPlan(plan: SubscriptionPlan): SubscriptionPlanFormState {
  const ent = (plan.entitlements ?? {}) as Record<string, unknown>;
  const extra: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(ent)) {
    if (!KNOWN_ENTITLEMENT_KEYS.includes(key)) {
      extra[key] = value;
    }
  }
  return {
    name: plan.name,
    description: plan.description ?? "",
    price: plan.price,
    currency: plan.currency,
    validityDays: String(plan.validity_days),
    allowedModels: toStringArray(ent.allowed_models),
    monthlyTokenQuota: ent.monthly_token_quota == null ? "" : String(ent.monthly_token_quota),
    monthlyCostQuota: ent.monthly_cost_quota == null ? "" : String(ent.monthly_cost_quota),
    costQuotaMode: ent.cost_quota_mode === "allowance" ? "allowance" : "hard_cap",
    schedulerStrategy:
      typeof ent.scheduler_strategy === "string" && ent.scheduler_strategy
        ? ent.scheduler_strategy
        : "default",
    accountGroupScope: toStringArray(ent.account_group_scope),
    extraEntitlements: extra,
    forSale: plan.for_sale,
    sortOrder: String(plan.sort_order),
    status: plan.status,
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

// Builds the full plan body (used for both create and update — the PATCH endpoint
// accepts the same shape with every field optional).
export function buildSubscriptionPlanBody(
  form: SubscriptionPlanFormState,
): CreateAdminSubscriptionPlanData["body"] {
  return {
    name: requiredText(form.name, "Name"),
    description: optionalText(form.description),
    price: parseDecimalString(form.price, "Price"),
    currency: requiredText(form.currency, "Currency").toUpperCase(),
    validity_days: parsePositiveInteger(form.validityDays, "Validity days"),
    entitlements: composeEntitlements(form),
    for_sale: form.forSale,
    sort_order: parseInteger(form.sortOrder, "Sort order"),
    status: form.status,
  };
}

// Compose the structured fields back into the backend entitlements JSON. Unset
// fields are omitted (0 / blank quota = unlimited), and extraEntitlements is the
// base so structured keys take precedence over any custom key of the same name.
function composeEntitlements(form: SubscriptionPlanFormState): Record<string, unknown> {
  const out: Record<string, unknown> = { ...form.extraEntitlements };
  if (form.allowedModels.length > 0) {
    out.allowed_models = form.allowedModels;
  }
  const tokenQuota = optionalPositiveInt(form.monthlyTokenQuota, "Monthly token quota");
  if (tokenQuota != null) {
    out.monthly_token_quota = tokenQuota;
  }
  const costQuota = optionalDecimal(form.monthlyCostQuota, "Monthly cost quota");
  if (costQuota != null) {
    out.monthly_cost_quota = costQuota;
    // cost_quota_mode only matters when a cost quota is set.
    out.cost_quota_mode = form.costQuotaMode;
  }
  if (form.accountGroupScope.length > 0) {
    out.account_group_scope = form.accountGroupScope.map((id) => {
      const parsed = Number(id);
      if (!Number.isInteger(parsed)) {
        throw new Error("Account group scope must be integer ids.");
      }
      return parsed;
    });
  }
  if (form.schedulerStrategy && form.schedulerStrategy !== "default") {
    out.scheduler_strategy = form.schedulerStrategy;
  }
  return out;
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

function isoToLocal(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function pricingRuleFormFromRule(rule: PricingRule): PricingRuleFormState {
  return {
    modelId: String(rule.model_id),
    providerId: String(rule.provider_id),
    inputPricePerMillionTokens: rule.input_price_per_million_tokens,
    outputPricePerMillionTokens: rule.output_price_per_million_tokens,
    cacheReadPricePerMillionTokens: rule.cache_read_price_per_million_tokens,
    cacheWritePricePerMillionTokens: rule.cache_write_price_per_million_tokens,
    currency: rule.currency,
    effectiveFromLocal: isoToLocal(rule.effective_from),
    effectiveToLocal: isoToLocal(rule.effective_to),
  };
}

export function buildUpdatePricingRuleBody(
  form: PricingRuleFormState,
): UpdateAdminPricingRuleData["body"] {
  return {
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

function toStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.map((entry) => String(entry)) : [];
}

// Blank or "0" means "unlimited" → omit the entitlement entirely.
function optionalPositiveInt(value: string, fieldName: string): number | null {
  const trimmed = value.trim();
  if (!trimmed || trimmed === "0") {
    return null;
  }
  const parsed = Number(trimmed);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(`${fieldName} must be a non-negative integer.`);
  }
  return parsed;
}

function optionalDecimal(value: string, fieldName: string): string | null {
  const trimmed = value.trim();
  if (!trimmed || trimmed === "0") {
    return null;
  }
  if (!/^[0-9]+(\.[0-9]+)?$/.test(trimmed)) {
    throw new Error(`${fieldName} must be a non-negative decimal string.`);
  }
  return trimmed;
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
