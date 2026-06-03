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
  "reverse-proxy-gemini-cli",
  "reverse-proxy-antigravity",
  "custom",
];

export const PROVIDER_PROTOCOLS: ProviderProtocol[] = [
  "openai-compatible",
  "anthropic-compatible",
  "gemini-compatible",
  "rerank-compatible",
];

export const RESOURCE_STATUSES: ResourceStatus[] = [
  "active",
  "disabled",
  "pending",
  "archived",
];

export interface ProviderFormState {
  name: string;
  displayName: string;
  adapterType: ProviderAdapterType;
  protocol: ProviderProtocol;
  status: ResourceStatus;
  capabilities: Record<string, unknown>;
  configSchema: Record<string, unknown>;
}

export function emptyProviderForm(): ProviderFormState {
  return {
    name: "",
    displayName: "",
    adapterType: "openai-compatible",
    protocol: "openai-compatible",
    status: "disabled",
    capabilities: {},
    configSchema: {},
  };
}

export function providerFormFromProvider(provider: Provider): ProviderFormState {
  return {
    name: provider.name,
    displayName: provider.display_name,
    adapterType: provider.adapter_type,
    protocol: provider.protocol,
    status: provider.status,
    capabilities: (provider.capabilities ?? {}) as Record<string, unknown>,
    configSchema: (provider.config_schema ?? {}) as Record<string, unknown>,
  };
}

export function buildCreateProviderBody(
  form: ProviderFormState,
): CreateAdminProviderData["body"] {
  return {
    name: parseProviderName(form.name),
    display_name: requiredText(form.displayName, "Display name"),
    adapter_type: form.adapterType,
    protocol: form.protocol,
    status: form.status,
    capabilities: form.capabilities,
    config_schema: form.configSchema,
  };
}

export function buildUpdateProviderBody(
  form: ProviderFormState,
): UpdateAdminProviderData["body"] {
  return {
    display_name: requiredText(form.displayName, "Display name"),
    adapter_type: form.adapterType,
    protocol: form.protocol,
    status: form.status,
    capabilities: form.capabilities,
    config_schema: form.configSchema,
  };
}

function parseProviderName(value: string): string {
  const normalized = requiredText(value, "Provider name");
  if (!/^[a-z0-9][a-z0-9_-]{1,62}$/.test(normalized)) {
    throw new Error("Provider name must be 2-63 chars: lowercase letters, numbers, '_' or '-'.");
  }
  return normalized;
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
