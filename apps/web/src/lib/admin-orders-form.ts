import type {
  CreateAdminPaymentProviderData,
  Id,
  PaymentOrder,
  PaymentOrderStatus,
  PaymentProviderStatus,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const REFUNDABLE_ORDER_STATUSES: PaymentOrderStatus[] = [
  "paid",
  "fulfilled",
  "partially_refunded",
];

export interface RefundOrderFormState {
  orderId: Id;
  orderNo: string;
  amount: string;
  currency: string;
  reason: string;
}

export interface PaymentProviderFormState {
  provider: string;
  name: string;
  status: PaymentProviderStatus;
  supportedMethodsText: string;
  configJson: string;
  limitsJson: string;
  metadataJson: string;
  sortOrder: string;
}

export function isRefundableOrder(order: Pick<PaymentOrder, "status">): boolean {
  return REFUNDABLE_ORDER_STATUSES.includes(order.status);
}

export function refundFormFromOrder(order: PaymentOrder): RefundOrderFormState {
  return {
    orderId: order.id,
    orderNo: order.order_no,
    amount: "",
    currency: order.currency,
    reason: "admin_requested",
  };
}

export function buildRefundPaymentOrderBody(
  form: RefundOrderFormState,
): { id: Id; amount?: string; reason?: string } {
  return {
    id: form.orderId,
    amount: optionalDecimalString(form.amount, "Refund amount"),
    reason: optionalLimitedText(form.reason, 500, "Reason"),
  };
}

export const PAYMENT_PROVIDER_STATUSES: PaymentProviderStatus[] = [
  "active",
  "disabled",
  "archived",
];

export function emptyPaymentProviderForm(): PaymentProviderFormState {
  return {
    provider: "",
    name: "",
    status: "disabled",
    supportedMethodsText: "card\nwallet",
    configJson: "{}",
    limitsJson: "{}",
    metadataJson: "{}",
    sortOrder: "0",
  };
}

export function buildCreatePaymentProviderBody(
  form: PaymentProviderFormState,
): CreateAdminPaymentProviderData["body"] {
  return {
    provider: requiredText(form.provider, "Provider"),
    name: requiredText(form.name, "Name"),
    status: form.status,
    config: parseJsonObject(form.configJson, "Config"),
    supported_methods: parseLines(form.supportedMethodsText),
    limits: parseJsonObject(form.limitsJson, "Limits"),
    metadata: parseJsonObject(form.metadataJson, "Metadata"),
    sort_order: parseInteger(form.sortOrder, "Sort order"),
  };
}

export function sumOrderAmounts(orders: Array<Pick<PaymentOrder, "amount">>): string {
  return sumDecimalStrings(orders.map((order) => order.amount));
}

export function sumDecimalStrings(values: string[]): string {
  const parsed = values.map(parseDecimalParts);
  const scale = parsed.reduce((max, value) => Math.max(max, value.scale), 0);
  let total = BigInt(0);
  for (const value of parsed) {
    total += value.units * BigInt(10) ** BigInt(scale - value.scale);
  }
  return formatUnits(total, scale);
}

function optionalDecimalString(value: string, fieldName: string): string | undefined {
  const trimmed = value.trim();
  if (!trimmed) {
    return undefined;
  }
  if (!/^[0-9]+(\.[0-9]+)?$/.test(trimmed)) {
    throw new Error(`${fieldName} must be a non-negative decimal string.`);
  }
  return trimmed;
}

function optionalLimitedText(value: string, maxLength: number, fieldName: string): string | undefined {
  const trimmed = value.trim();
  if (!trimmed) {
    return undefined;
  }
  if (trimmed.length > maxLength) {
    throw new Error(`${fieldName} must be ${maxLength} characters or fewer.`);
  }
  return trimmed;
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}

function parseLines(value: string): string[] {
  return value
    .split(/\r?\n|,/)
    .map((line) => line.trim())
    .filter(Boolean);
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

function parseInteger(value: string, fieldName: string): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed)) {
    throw new Error(`${fieldName} must be an integer.`);
  }
  return parsed;
}

function parseDecimalParts(value: string): { units: bigint; scale: number } {
  const trimmed = value.trim();
  if (!/^-?[0-9]+(\.[0-9]+)?$/.test(trimmed)) {
    throw new Error(`Invalid decimal string: ${value}`);
  }
  const negative = trimmed.startsWith("-");
  const unsigned = negative ? trimmed.slice(1) : trimmed;
  const [whole, fractional = ""] = unsigned.split(".");
  const digits = `${whole}${fractional}`.replace(/^0+(?=\d)/, "");
  const units = BigInt(digits || "0") * (negative ? BigInt(-1) : BigInt(1));
  return { units, scale: fractional.length };
}

function formatUnits(units: bigint, scale: number): string {
  const negative = units < BigInt(0);
  const unsigned = negative ? -units : units;
  if (scale === 0) {
    return `${negative ? "-" : ""}${unsigned.toString()}`;
  }
  const raw = unsigned.toString().padStart(scale + 1, "0");
  const whole = raw.slice(0, -scale);
  const fractional = raw.slice(-scale).replace(/0+$/, "");
  return `${negative ? "-" : ""}${whole}${fractional ? `.${fractional}` : ""}`;
}
