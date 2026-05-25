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
  enabledChannelsText: string;
  oauthProvidersText: string;
  schedulerRolloutModelsText: string;
  schedulerRolloutApiKeyHashesText: string;
  paymentProvidersText: string;
  emailTemplatesJson: string;
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
    enabledChannelsText: listToText(value.features.enabled_channels),
    oauthProvidersText: listToText(value.security.oauth_providers),
    schedulerRolloutModelsText: listToText(value.gateway.scheduler_strategy_rollout_models),
    schedulerRolloutApiKeyHashesText: listToText(value.gateway.scheduler_strategy_rollout_api_key_hashes),
    paymentProvidersText: listToText(value.payment.providers),
    emailTemplatesJson: JSON.stringify(value.email.templates ?? {}, null, 2),
  };
}

export function materializeSettingsDraft(draft: AdminSettingsDraft): AdminSettings {
  const customMenus = parseArrayOfObjects(draft.customMenusJson, "Custom menus");
  const emailTemplates = parseStringMap(draft.emailTemplatesJson, "Email templates");
  return {
    ...draft.value,
    general: {
      ...draft.value.general,
      custom_menus: customMenus,
    },
    features: {
      ...draft.value.features,
      enabled_channels: textToList(draft.enabledChannelsText),
    },
    security: {
      ...draft.value.security,
      oauth_providers: textToList(draft.oauthProvidersText),
    },
    gateway: {
      ...draft.value.gateway,
      scheduler_strategy_rollout_models: textToList(draft.schedulerRolloutModelsText),
      scheduler_strategy_rollout_api_key_hashes: textToList(draft.schedulerRolloutApiKeyHashesText),
    },
    payment: {
      ...draft.value.payment,
      providers: textToList(draft.paymentProvidersText),
    },
    email: {
      ...draft.value.email,
      templates: emailTemplates,
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

export function listToText(items: string[] | undefined): string {
  return (items ?? []).join("\n");
}

export function textToList(value: string): string[] {
  return value
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean);
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

function parseStringMap(value: string, fieldName: string): Record<string, string> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || "{}") as unknown;
  } catch {
    throw new Error(`${fieldName} must be valid JSON.`);
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error(`${fieldName} must be a JSON object.`);
  }
  const out: Record<string, string> = {};
  for (const [key, item] of Object.entries(parsed as Record<string, unknown>)) {
    if (typeof item !== "string") {
      throw new Error(`${fieldName} values must be strings.`);
    }
    out[key] = item;
  }
  return out;
}
