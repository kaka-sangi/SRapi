import type {
  CreateErrorPassthroughRuleRequest,
  ErrorPassthroughRule,
} from "../../../../packages/sdk/typescript/src/types.gen";

export type ErrorPassthroughAction = ErrorPassthroughRule["action"];

export const ERROR_PASSTHROUGH_ACTIONS: ErrorPassthroughAction[] = ["expose", "mask"];

export interface ErrorPassthroughFormState {
  name: string;
  action: ErrorPassthroughAction;
  priority: string;
  enabled: boolean;
  statusCodes: string[];
  classes: string[];
  keywords: string[];
}

export function emptyErrorPassthroughForm(): ErrorPassthroughFormState {
  return {
    name: "",
    action: "expose",
    priority: "0",
    enabled: true,
    statusCodes: [],
    classes: [],
    keywords: [],
  };
}

export function errorPassthroughFormFromRule(rule: ErrorPassthroughRule): ErrorPassthroughFormState {
  return {
    name: rule.name,
    action: rule.action,
    priority: String(rule.priority ?? 0),
    enabled: rule.enabled,
    statusCodes: (rule.status_codes ?? []).map(String),
    classes: rule.classes ?? [],
    keywords: rule.keywords ?? [],
  };
}

export function buildErrorPassthroughBody(
  form: ErrorPassthroughFormState,
): CreateErrorPassthroughRuleRequest {
  const name = form.name.trim();
  if (!name) {
    throw new Error("Name is required.");
  }
  // A rule that matches nothing would silently never fire — make the operator
  // give it at least one of status codes / classes / keywords to act on.
  const statusCodes = form.statusCodes
    .map((code) => Number(code.trim()))
    .filter((code) => Number.isInteger(code) && code >= 100 && code <= 599);
  const classes = form.classes.map((value) => value.trim()).filter(Boolean);
  const keywords = form.keywords.map((value) => value.trim()).filter(Boolean);
  if (statusCodes.length === 0 && classes.length === 0 && keywords.length === 0) {
    throw new Error("Add at least one status code, error class, or keyword to match.");
  }
  const priority = form.priority.trim();
  return {
    name,
    action: form.action,
    enabled: form.enabled,
    priority: priority === "" ? 0 : Number(priority),
    status_codes: statusCodes,
    classes,
    keywords,
  };
}
