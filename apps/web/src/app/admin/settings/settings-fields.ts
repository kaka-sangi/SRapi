import type { AdminSettingsDraft } from "@/lib/admin-settings-form";
import { type SettingsTab } from "@/lib/admin-settings-form";

/** Graphical controls for the list/map fields the draft tracks outside `value`. */
export type SpecialKind = "tags" | "models" | "conversion-routes" | "templates" | "json";
export interface SpecialField {
  key: keyof AdminSettingsDraft;
  kind: SpecialKind;
  skip: string;
}
export const SPECIAL_FIELDS: Partial<Record<SettingsTab, SpecialField[]>> = {
  general: [{ key: "customMenusJson", kind: "json", skip: "custom_menus" }],
  features: [{ key: "enabledChannels", kind: "tags", skip: "enabled_channels" }],
  security: [{ key: "oauthProviders", kind: "tags", skip: "oauth_providers" }],
  gateway: [
    { key: "schedulerRolloutModels", kind: "models", skip: "scheduler_strategy_rollout_models" },
    {
      key: "schedulerRolloutApiKeyHashes",
      kind: "tags",
      skip: "scheduler_strategy_rollout_api_key_hashes",
    },
    {
      key: "protocolConversionRoutes",
      kind: "conversion-routes",
      skip: "protocol_conversion_routes",
    },
    {
      key: "passthroughHeaderAllowlist",
      kind: "tags",
      skip: "passthrough_header_allowlist",
    },
  ],
  payment: [{ key: "paymentProviders", kind: "tags", skip: "providers" }],
  email: [{ key: "emailTemplates", kind: "templates", skip: "templates" }],
};

/**
 * Gateway numeric settings that must stay non-negative integers (the
 * operator-tunable retry/failover knobs and cooldown/timeout values). The Go
 * side clamps too, but clamping in the input keeps the control honest.
 */
export const GATEWAY_NON_NEGATIVE_INT_FIELDS = new Set<string>([
  "overload_cooldown_seconds",
  "rate_limit_cooldown_seconds",
  "stream_timeout_seconds",
  "retry_count",
  "max_retry_credentials",
  "max_retry_interval_ms",
]);

function humanize(key: string): string {
  return key.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

/** Localized settings-field label; falls back to humanized English for any unmapped key. */
export function fieldLabel(key: string, t: (k: string) => string): string {
  const id = `adminSettings.fields.${key}`;
  const label = t(id);
  return label === id ? humanize(key) : label;
}
