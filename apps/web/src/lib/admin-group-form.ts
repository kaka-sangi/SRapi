import type {
  AccountGroup,
  AccountGroupStatus,
  CreateAccountGroupRequest,
  Id,
  Model,
  Provider,
  UpdateAccountGroupRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const ACCOUNT_GROUP_STATUSES: AccountGroupStatus[] = ["active", "disabled"];
export const GROUP_STRATEGY_HINTS = [
  "balanced",
  "cost_saver",
  "latency_first",
  "quota_protect",
  "sticky_first",
  "cache_affinity_first",
  "premium_quality",
];

export interface AccountGroupFormState {
  name: string;
  description: string;
  providerScopeJson: string;
  modelScopeJson: string;
  selectedProviderId: Id;
  selectedModelName: string;
  strategyHint: string;
  status: AccountGroupStatus;
}

export function emptyAccountGroupForm(): AccountGroupFormState {
  return {
    name: "",
    description: "",
    providerScopeJson: "{}",
    modelScopeJson: "{}",
    selectedProviderId: "",
    selectedModelName: "",
    strategyHint: "balanced",
    status: "active",
  };
}

export function accountGroupFormFromGroup(group: AccountGroup): AccountGroupFormState {
  return {
    name: group.name,
    description: group.description,
    providerScopeJson: JSON.stringify(group.provider_scope ?? {}, null, 2),
    modelScopeJson: JSON.stringify(group.model_scope ?? {}, null, 2),
    selectedProviderId: typeof group.provider_scope?.provider_id === "string" ? group.provider_scope.provider_id : "",
    selectedModelName: typeof group.model_scope?.canonical_name === "string" ? group.model_scope.canonical_name : "",
    strategyHint: group.strategy_hint || "balanced",
    status: group.status,
  };
}

export function buildCreateAccountGroupBody(form: AccountGroupFormState): CreateAccountGroupRequest {
  return {
    name: requiredText(form.name, "Name"),
    description: form.description.trim(),
    provider_scope: parseJsonObject(form.providerScopeJson, "Provider scope"),
    model_scope: parseJsonObject(form.modelScopeJson, "Model scope"),
    strategy_hint: form.strategyHint.trim(),
    status: form.status,
  };
}

export function buildUpdateAccountGroupBody(form: AccountGroupFormState): UpdateAccountGroupRequest {
  return buildCreateAccountGroupBody(form);
}

export function applyProviderScopeSelection(
  form: AccountGroupFormState,
  providerId: Id,
): AccountGroupFormState {
  return {
    ...form,
    selectedProviderId: providerId,
    providerScopeJson: JSON.stringify(providerId ? { provider_id: providerId } : {}, null, 2),
  };
}

export function applyModelScopeSelection(
  form: AccountGroupFormState,
  canonicalName: string,
): AccountGroupFormState {
  return {
    ...form,
    selectedModelName: canonicalName,
    modelScopeJson: JSON.stringify(canonicalName ? { canonical_name: canonicalName } : {}, null, 2),
  };
}

export function providerScopeLabel(scope: Record<string, unknown>, providers: Provider[]): string {
  const providerId = typeof scope.provider_id === "string" ? scope.provider_id : "";
  if (!providerId) {
    return "All providers";
  }
  return providers.find((provider) => provider.id === providerId)?.display_name ?? providerId;
}

export function modelScopeLabel(scope: Record<string, unknown>, models: Model[]): string {
  const canonicalName = typeof scope.canonical_name === "string" ? scope.canonical_name : "";
  if (canonicalName) {
    return models.find((model) => model.canonical_name === canonicalName)?.display_name ?? canonicalName;
  }
  const family = typeof scope.family === "string" ? scope.family : "";
  return family ? `family:${family}` : "All models";
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

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
