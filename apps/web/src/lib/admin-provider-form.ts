import type {
  CreateAdminProviderData,
  Provider,
  ProviderAdapterType,
  ProviderProtocol,
  ResourceStatus,
  UpdateAdminProviderData,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const PROVIDER_ADAPTER_TYPES: ProviderAdapterType[] = [
  "openai-compatible",
  "generic-reverse-proxy",
  "anthropic-compatible",
  "gemini-compatible",
  "rerank-compatible",
  "native-openai",
  "native-anthropic",
  "native-gemini",
  "openrouter",
  "reverse-proxy-chatgpt-web",
  "reverse-proxy-codex-cli",
  "reverse-proxy-claude-web",
  "reverse-proxy-claude-code-cli",
  "reverse-proxy-antigravity",
  "custom",
];

export const PROVIDER_PROTOCOLS: ProviderProtocol[] = [
  "openai-compatible",
  "anthropic-compatible",
  "gemini-compatible",
  "rerank-compatible",
];

export const RESOURCE_STATUSES: ResourceStatus[] = ["active", "disabled", "pending", "archived"];

export type ProviderEndpointCapabilityMode = "auto" | "on" | "off";

const ENDPOINT_CAPABILITY_KEYS = {
  chatCompletionsCapability: "chat_completions",
  responsesCapability: "responses",
  responsesCompactCapability: "responses_compact",
  responsesInputItemsCapability: "responses_input_items",
  messagesCapability: "messages",
} as const satisfies Record<string, string>;

const ENDPOINT_CAPABILITY_FIELDS = Object.keys(
  ENDPOINT_CAPABILITY_KEYS,
) as (keyof typeof ENDPOINT_CAPABILITY_KEYS)[];

export interface ProviderFormState {
  name: string;
  displayName: string;
  adapterType: ProviderAdapterType;
  protocol: ProviderProtocol;
  status: ResourceStatus;
  chatCompletionsCapability: ProviderEndpointCapabilityMode;
  responsesCapability: ProviderEndpointCapabilityMode;
  responsesCompactCapability: ProviderEndpointCapabilityMode;
  responsesInputItemsCapability: ProviderEndpointCapabilityMode;
  messagesCapability: ProviderEndpointCapabilityMode;
  excludedModels: string[];
  capabilities: Record<string, unknown>;
  configSchema: Record<string, unknown>;
}

export function emptyProviderForm(): ProviderFormState {
  return {
    name: "",
    displayName: "",
    adapterType: "openai-compatible",
    protocol: "openai-compatible",
    // Default active, consistent with every other resource form and with the
    // Quick Setup wizard (which creates providers active). A disabled default
    // silently breaks every account created under the new provider.
    status: "active",
    chatCompletionsCapability: "auto",
    responsesCapability: "auto",
    responsesCompactCapability: "auto",
    responsesInputItemsCapability: "auto",
    messagesCapability: "auto",
    excludedModels: [],
    capabilities: {},
    configSchema: {},
  };
}

export function providerFormFromProvider(provider: Provider): ProviderFormState {
  const capabilities = (provider.capabilities ?? {}) as Record<string, unknown>;
  const configSchema = (provider.config_schema ?? {}) as Record<string, unknown>;
  return {
    name: provider.name,
    displayName: provider.display_name,
    adapterType: provider.adapter_type,
    protocol: provider.protocol,
    status: provider.status,
    chatCompletionsCapability: capabilityMode(capabilities.chat_completions),
    responsesCapability: capabilityMode(capabilities.responses),
    responsesCompactCapability: capabilityMode(capabilities.responses_compact),
    responsesInputItemsCapability: capabilityMode(capabilities.responses_input_items),
    messagesCapability: capabilityMode(capabilities.messages),
    excludedModels: stringListFromValue(configSchema.excluded_models ?? configSchema["excluded-models"]),
    capabilities,
    configSchema,
  };
}

export function buildCreateProviderBody(form: ProviderFormState): CreateAdminProviderData["body"] {
  return {
    name: parseProviderName(form.name),
    display_name: requiredText(form.displayName, "Display name"),
    adapter_type: form.adapterType,
    protocol: form.protocol,
    status: form.status,
    capabilities: composeProviderCapabilities(form),
    config_schema: composeProviderConfigSchema(form),
  };
}

export function buildUpdateProviderBody(form: ProviderFormState): UpdateAdminProviderData["body"] {
  return {
    display_name: requiredText(form.displayName, "Display name"),
    adapter_type: form.adapterType,
    protocol: form.protocol,
    status: form.status,
    capabilities: composeProviderCapabilities(form),
    config_schema: composeProviderConfigSchema(form),
  };
}

function composeProviderCapabilities(form: ProviderFormState): Record<string, unknown> {
  const next: Record<string, unknown> = { ...form.capabilities };
  for (const field of ENDPOINT_CAPABILITY_FIELDS) {
    const key = ENDPOINT_CAPABILITY_KEYS[field];
    const mode = form[field];
    if (mode === "auto") {
      delete next[key];
      continue;
    }
    next[key] = mode === "on";
  }
  return next;
}

function composeProviderConfigSchema(form: ProviderFormState): Record<string, unknown> {
  const next: Record<string, unknown> = { ...form.configSchema };
  delete next["excluded-models"];
  const excludedModels = cleanStringList(form.excludedModels);
  if (excludedModels.length > 0) {
    next.excluded_models = excludedModels;
  } else {
    delete next.excluded_models;
  }
  return next;
}

function capabilityMode(value: unknown): ProviderEndpointCapabilityMode {
  if (value === true) return "on";
  if (value === false) return "off";
  return "auto";
}

function parseProviderName(value: string): string {
  const normalized = requiredText(value, "Provider name");
  if (!/^[a-z0-9][a-z0-9_-]{1,62}$/.test(normalized)) {
    throw new Error("Provider name must be 2-63 chars: lowercase letters, numbers, '_' or '-'.");
  }
  return normalized;
}

function cleanStringList(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed) continue;
    const key = trimmed.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(trimmed);
  }
  return out;
}

function stringListFromValue(value: unknown): string[] {
  if (Array.isArray(value)) return cleanStringList(value.map((item) => String(item)));
  if (typeof value === "string") return cleanStringList(value.split(","));
  if (value == null) return [];
  return cleanStringList([String(value)]);
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
