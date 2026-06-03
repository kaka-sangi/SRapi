import type {
  CreatePayloadRuleRequest,
  PayloadRule,
} from "../../../../packages/sdk/typescript/src/types.gen";

export type PayloadRuleAction = PayloadRule["action"];

export const PAYLOAD_RULE_ACTIONS: PayloadRuleAction[] = ["default", "override", "filter"];

export interface PayloadRuleFormState {
  name: string;
  action: PayloadRuleAction;
  matchModel: string;
  matchProtocol: string;
  priority: string;
  enabled: boolean;
  // path -> raw string; values are parsed as JSON (number/bool/object) when
  // possible, else kept as a string. For the "filter" action only keys matter.
  params: Record<string, string>;
}

export function emptyPayloadRuleForm(): PayloadRuleFormState {
  return {
    name: "",
    action: "override",
    matchModel: "*",
    matchProtocol: "",
    priority: "0",
    enabled: true,
    params: {},
  };
}

export function payloadRuleFormFromRule(rule: PayloadRule): PayloadRuleFormState {
  return {
    name: rule.name,
    action: rule.action,
    matchModel: rule.match_model || "*",
    matchProtocol: rule.match_protocol ?? "",
    priority: String(rule.priority ?? 0),
    enabled: rule.enabled,
    params: stringifyParams(rule.params),
  };
}

export function buildPayloadRuleBody(form: PayloadRuleFormState): CreatePayloadRuleRequest {
  const name = form.name.trim();
  if (!name) {
    throw new Error("Name is required.");
  }
  const params: Record<string, unknown> = {};
  for (const [rawKey, rawValue] of Object.entries(form.params)) {
    const key = rawKey.trim();
    if (!key) continue;
    if (form.action === "filter") {
      params[key] = null;
      continue;
    }
    params[key] = parseParamValue(rawValue);
  }
  if (Object.keys(params).length === 0) {
    throw new Error("Add at least one path to set, override, or filter.");
  }
  const priority = form.priority.trim();
  return {
    name,
    action: form.action,
    enabled: form.enabled,
    priority: priority === "" ? 0 : Number(priority),
    match_model: form.matchModel.trim() || "*",
    match_protocol: form.matchProtocol.trim(),
    params,
  };
}

// parseParamValue turns the operator's raw text into a real JSON value: 32768 ->
// number, true -> bool, {"a":1} -> object, "high"/high -> string.
function parseParamValue(raw: string): unknown {
  const trimmed = raw.trim();
  if (trimmed === "") return "";
  try {
    return JSON.parse(trimmed);
  } catch {
    return raw;
  }
}

function stringifyParams(params: Record<string, unknown> | null | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  if (!params) return out;
  for (const [key, value] of Object.entries(params)) {
    out[key] = typeof value === "string" ? value : JSON.stringify(value);
  }
  return out;
}
