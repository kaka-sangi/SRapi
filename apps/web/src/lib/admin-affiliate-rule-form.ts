import type {
  AffiliateRule,
  AffiliateRuleStatus,
  AffiliateRuleTriggerType,
  CreateAffiliateRuleRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";

export interface AffiliateRuleFormState {
  name: string;
  status: AffiliateRuleStatus;
  triggerType: AffiliateRuleTriggerType;
  rate: string;
  fixedAmount: string;
  currency: string;
  maxRebateAmount: string;
  validFromLocal: string;
  validToLocal: string;
}

export const AFFILIATE_RULE_STATUSES: AffiliateRuleStatus[] = ["active", "disabled", "archived"];
export const AFFILIATE_RULE_TRIGGER_TYPES: AffiliateRuleTriggerType[] = ["payment_paid"];

export function emptyAffiliateRuleForm(): AffiliateRuleFormState {
  return {
    name: "",
    status: "active",
    triggerType: "payment_paid",
    rate: "0.1",
    fixedAmount: "0",
    currency: "USD",
    maxRebateAmount: "0",
    validFromLocal: "",
    validToLocal: "",
  };
}

export function affiliateRuleFormFromRule(rule: AffiliateRule): AffiliateRuleFormState {
  return {
    name: rule.name,
    status: rule.status,
    triggerType: rule.trigger_type,
    rate: rule.rate,
    fixedAmount: rule.fixed_amount,
    currency: rule.currency,
    maxRebateAmount: rule.max_rebate_amount,
    validFromLocal: isoToLocalDateTime(rule.valid_from ?? undefined),
    validToLocal: isoToLocalDateTime(rule.valid_to ?? undefined),
  };
}

export function buildAffiliateRuleBody(
  form: AffiliateRuleFormState,
): CreateAffiliateRuleRequest {
  const body: CreateAffiliateRuleRequest = {
    name: requiredText(form.name, "Name"),
    status: form.status,
    trigger_type: form.triggerType,
    rate: parseDecimalString(form.rate, "Rate"),
    fixed_amount: parseDecimalString(form.fixedAmount, "Fixed amount"),
    currency: requiredText(form.currency, "Currency").toUpperCase(),
    max_rebate_amount: parseDecimalString(form.maxRebateAmount, "Max rebate"),
  };
  const validFrom = localDateTimeToIso(form.validFromLocal, "Valid from");
  const validTo = localDateTimeToIso(form.validToLocal, "Valid to");
  if (validFrom) {
    body.valid_from = validFrom;
  }
  if (validTo) {
    body.valid_to = validTo;
  }
  return body;
}

function parseDecimalString(value: string, fieldName: string): string {
  const normalized = requiredText(value, fieldName);
  if (!/^[0-9]+(\.[0-9]+)?$/.test(normalized)) {
    throw new Error(`${fieldName} must be a non-negative decimal string.`);
  }
  return normalized;
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

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
