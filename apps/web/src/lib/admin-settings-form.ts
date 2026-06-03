import type { AdminSettings } from "../../../../packages/sdk/typescript/src/types.gen";

export type SettingsTab =
  | "general"
  | "agreement"
  | "features"
  | "security"
  | "users"
  | "gateway"
  | "payment"
  | "email"
  | "backup";

export const SETTINGS_TABS: Array<{ id: SettingsTab; label: string }> = [
  { id: "general", label: "General" },
  { id: "agreement", label: "Agreement" },
  { id: "features", label: "Features" },
  { id: "security", label: "Security" },
  { id: "users", label: "Users" },
  { id: "gateway", label: "Gateway" },
  { id: "payment", label: "Payment" },
  { id: "email", label: "Email" },
  { id: "backup", label: "Backup" },
];

export interface AdminSettingsDraft {
  value: AdminSettings;
  customMenusJson: string;
  enabledChannels: string[];
  oauthProviders: string[];
  schedulerRolloutModels: string[];
  schedulerRolloutApiKeyHashes: string[];
  paymentProviders: string[];
  emailTemplates: Record<string, string>;
}

export interface SettingsSaveConfirmationState {
  tab: SettingsTab;
  phrase: string;
  confirmation: string;
}

export function createSettingsDraft(value: AdminSettings): AdminSettingsDraft {
  return {
    value,
    customMenusJson: JSON.stringify(value.general.custom_menus ?? [], null, 2),
    enabledChannels: [...(value.features.enabled_channels ?? [])],
    oauthProviders: [...(value.security.oauth_providers ?? [])],
    schedulerRolloutModels: [...(value.gateway.scheduler_strategy_rollout_models ?? [])],
    schedulerRolloutApiKeyHashes: [...(value.gateway.scheduler_strategy_rollout_api_key_hashes ?? [])],
    paymentProviders: [...(value.payment.providers ?? [])],
    emailTemplates: { ...(value.email.templates ?? {}) },
  };
}

export function materializeSettingsDraft(draft: AdminSettingsDraft): AdminSettings {
  const customMenus = parseArrayOfObjects(draft.customMenusJson, "Custom menus");
  return {
    ...draft.value,
    general: {
      ...draft.value.general,
      custom_menus: customMenus,
    },
    features: {
      ...draft.value.features,
      enabled_channels: cleanList(draft.enabledChannels),
    },
    security: {
      ...draft.value.security,
      oauth_providers: cleanList(draft.oauthProviders),
    },
    gateway: {
      ...draft.value.gateway,
      scheduler_strategy_rollout_models: cleanList(draft.schedulerRolloutModels),
      scheduler_strategy_rollout_api_key_hashes: cleanList(draft.schedulerRolloutApiKeyHashes),
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

export function updateSettingsValue(
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
]);

export function settingsTabRequiresConfirmation(tab: SettingsTab): boolean {
  return HIGH_RISK_SETTINGS_TABS.has(tab);
}

export function settingsSaveConfirmationPhrase(tab: SettingsTab): string {
  return `SAVE ${tab.toUpperCase()} SETTINGS`;
}

export function createSettingsSaveConfirmation(tab: SettingsTab): SettingsSaveConfirmationState {
  return {
    tab,
    phrase: settingsSaveConfirmationPhrase(tab),
    confirmation: "",
  };
}

export function canConfirmSettingsSave(state: SettingsSaveConfirmationState | null): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

/** Trim, drop blanks, and dedupe a chip/picker list before persisting. */
export function cleanList(items: string[] | undefined): string[] {
  const out: string[] = [];
  for (const item of items ?? []) {
    const trimmed = item.trim();
    if (trimmed && !out.includes(trimmed)) out.push(trimmed);
  }
  return out;
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

function parseArrayOfObjects(value: string, fieldName: string): Array<Record<string, unknown>> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || "[]") as unknown;
  } catch {
    throw new Error(`${fieldName} must be valid JSON.`);
  }
  if (!Array.isArray(parsed) || parsed.some((item) => !item || Array.isArray(item) || typeof item !== "object")) {
    throw new Error(`${fieldName} must be a JSON array of objects.`);
  }
  return parsed as Array<Record<string, unknown>>;
}
