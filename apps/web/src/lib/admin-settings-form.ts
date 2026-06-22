import type {
  AdminSettings,
  AdminSettingsCopilotWritable,
  AdminSettingsGateway,
  CustomMenuItem,
  OAuthProviderConfig,
} from "../../../../packages/sdk/typescript/src/types.gen";

// The write model carries the write-only `dedicated_api_key`; the copilot
// settings form edits that field, so it works against the writable shape.
export type AdminSettingsCopilot = AdminSettingsCopilotWritable;
export type ProtocolConversionRoute = NonNullable<AdminSettingsGateway["protocol_conversion_routes"]>[number];

export const PROTOCOL_CONVERSION_ROUTES = [
  "chat_completions_to_responses",
  "chat_completions_to_messages",
  "responses_to_chat_completions",
  "responses_to_messages",
  "messages_to_chat_completions",
  "messages_to_responses",
] as const satisfies readonly ProtocolConversionRoute[];

const protocolConversionRouteSet = new Set<string>(PROTOCOL_CONVERSION_ROUTES);

export type SettingsTab =
  | "general"
  | "agreement"
  | "features"
  | "security"
  | "users"
  | "gateway"
  | "payment"
  | "email"
  | "copilot"
  | "backup"
  | "maintenance";

export const SETTINGS_TABS: Array<{ id: SettingsTab; label: string }> = [
  { id: "general", label: "General" },
  { id: "agreement", label: "Agreement" },
  { id: "features", label: "Features" },
  { id: "security", label: "Security" },
  { id: "users", label: "Users" },
  { id: "gateway", label: "Gateway" },
  { id: "payment", label: "Payment" },
  { id: "email", label: "Email" },
  { id: "copilot", label: "Copilot" },
  { id: "backup", label: "Backup" },
  { id: "maintenance", label: "Maintenance" },
];

export interface AdminSettingsDraft {
  value: AdminSettings;
  customMenus: CustomMenuItem[];
  enabledChannels: string[];
  registrationEmailSuffixAllowlist: string[];
  oauthProviders: string[];
  oauthProviderConfigs: OAuthProviderConfig[];
  schedulerRolloutModels: string[];
  schedulerRolloutApiKeyHashes: string[];
  protocolConversionRoutes: ProtocolConversionRoute[];
  passthroughHeaderAllowlist: string[];
  paymentProviders: string[];
  emailTemplates: Record<string, string>;
}

interface SettingsSaveConfirmationState {
  tab: SettingsTab;
  phrase: string;
  confirmation: string;
}

export function createSettingsDraft(value: AdminSettings): AdminSettingsDraft {
  return {
    value,
    customMenus: cloneCustomMenuItems(value.general.custom_menus),
    enabledChannels: [...(value.features.enabled_channels ?? [])],
    registrationEmailSuffixAllowlist: [...(value.security.registration_email_suffix_allowlist ?? [])],
    oauthProviders: [...(value.security.oauth_providers ?? [])],
    oauthProviderConfigs: cloneOAuthProviderConfigs(value.security.oauth_provider_configs),
    schedulerRolloutModels: [...(value.gateway.scheduler_strategy_rollout_models ?? [])],
    schedulerRolloutApiKeyHashes: [...(value.gateway.scheduler_strategy_rollout_api_key_hashes ?? [])],
    protocolConversionRoutes: cleanProtocolConversionRoutes(value.gateway.protocol_conversion_routes),
    passthroughHeaderAllowlist: [...(value.gateway.passthrough_header_allowlist ?? [])],
    paymentProviders: [...(value.payment.providers ?? [])],
    emailTemplates: { ...(value.email.templates ?? {}) },
  };
}

export function materializeSettingsDraft(draft: AdminSettingsDraft): AdminSettings {
  return {
    ...draft.value,
    general: {
      ...draft.value.general,
      custom_menus: cleanCustomMenuItems(draft.customMenus),
    },
    features: {
      ...draft.value.features,
      enabled_channels: cleanList(draft.enabledChannels),
    },
    security: {
      ...draft.value.security,
      registration_email_suffix_allowlist: cleanList(draft.registrationEmailSuffixAllowlist),
      oauth_providers: cleanList(draft.oauthProviders),
      oauth_provider_configs: cleanOAuthProviderConfigs(draft.oauthProviderConfigs),
    },
    gateway: {
      ...draft.value.gateway,
      scheduler_strategy_rollout_models: cleanList(draft.schedulerRolloutModels),
      scheduler_strategy_rollout_api_key_hashes: cleanList(draft.schedulerRolloutApiKeyHashes),
      protocol_conversion_routes: cleanProtocolConversionRoutes(draft.protocolConversionRoutes),
      passthrough_header_allowlist: cleanList(draft.passthroughHeaderAllowlist),
    },
    payment: {
      ...draft.value.payment,
      providers: cleanList(draft.paymentProviders),
    },
    email: {
      ...draft.value.email,
      templates: cleanTemplates(draft.emailTemplates),
    },
  };
}

function updateSettingsValue(
  draft: AdminSettingsDraft,
  updater: (value: AdminSettings) => AdminSettings,
): AdminSettingsDraft {
  return {
    ...draft,
    value: updater(draft.value),
  };
}

const HIGH_RISK_SETTINGS_TABS = new Set<SettingsTab>([
  "security",
  "users",
  "gateway",
  "payment",
  "email",
  "backup",
  "maintenance",
]);

export function settingsTabRequiresConfirmation(tab: SettingsTab): boolean {
  return HIGH_RISK_SETTINGS_TABS.has(tab);
}

export function settingsSaveConfirmationPhrase(tab: SettingsTab): string {
  return `SAVE ${tab.toUpperCase()} SETTINGS`;
}

function createSettingsSaveConfirmation(tab: SettingsTab): SettingsSaveConfirmationState {
  return {
    tab,
    phrase: settingsSaveConfirmationPhrase(tab),
    confirmation: "",
  };
}

function canConfirmSettingsSave(state: SettingsSaveConfirmationState | null): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

/** Trim, drop blanks, and dedupe a chip/picker list before persisting. */
function cleanList(items: string[] | undefined): string[] {
  const out: string[] = [];
  for (const item of items ?? []) {
    const trimmed = item.trim();
    if (trimmed && !out.includes(trimmed)) out.push(trimmed);
  }
  return out;
}

/** Keep only known endpoint conversion routes while preserving display order. */
function cleanProtocolConversionRoutes(items: readonly string[] | undefined): ProtocolConversionRoute[] {
  const selected = new Set<string>();
  for (const item of items ?? []) {
    const trimmed = item.trim();
    if (protocolConversionRouteSet.has(trimmed)) selected.add(trimmed);
  }
  return PROTOCOL_CONVERSION_ROUTES.filter((route) => selected.has(route));
}

/** Drop blank keys and coerce values to strings for the email-template map. */
function cleanTemplates(map: Record<string, string> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(map ?? {})) {
    const trimmed = key.trim();
    if (trimmed) out[trimmed] = typeof value === "string" ? value : JSON.stringify(value);
  }
  return out;
}

function cloneOAuthProviderConfigs(
  values: readonly OAuthProviderConfig[] | undefined,
): OAuthProviderConfig[] {
  return (values ?? []).map((value) => ({
    ...value,
    scopes: [...(value.scopes ?? [])],
  }));
}

function cleanOAuthProviderConfigs(
  values: readonly OAuthProviderConfig[] | undefined,
): OAuthProviderConfig[] {
  return (values ?? []).map((value) => ({
    provider: value.provider,
    provider_key: value.provider_key.trim(),
    display_name: value.display_name.trim(),
    client_id: value.client_id.trim(),
    authorize_url: value.authorize_url.trim(),
    token_url: value.token_url?.trim() || undefined,
    userinfo_url: value.userinfo_url?.trim() || undefined,
    token_auth_method: "none",
    redirect_uri: value.redirect_uri.trim(),
    scopes: cleanOAuthScopes(value.scopes),
  }));
}

function cleanOAuthScopes(items: readonly string[] | undefined): string[] {
  const out: string[] = [];
  for (const item of items ?? []) {
    for (const scope of item.replaceAll(",", " ").split(/\s+/)) {
      const trimmed = scope.trim();
      if (trimmed && !out.includes(trimmed)) out.push(trimmed);
    }
  }
  return out;
}

function cloneCustomMenuItems(values: readonly CustomMenuItem[] | undefined): CustomMenuItem[] {
  return (values ?? []).map((value, index) => ({
    id: value.id,
    label: value.label,
    url: value.url,
    visibility: value.visibility,
    sort_order: value.sort_order ?? index,
  }));
}

function cleanCustomMenuItems(values: readonly CustomMenuItem[] | undefined): CustomMenuItem[] {
  return (values ?? [])
    .map((value, index): CustomMenuItem => ({
      id: normalizeCustomMenuId(value.id || value.label),
      label: value.label.trim(),
      url: value.url.trim(),
      visibility: value.visibility === "admin" ? "admin" : "user",
      sort_order: index,
    }))
    .filter((value) => value.label && value.url);
}

function normalizeCustomMenuId(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._ -]+/g, "")
    .replace(/[._\s-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80);
}
