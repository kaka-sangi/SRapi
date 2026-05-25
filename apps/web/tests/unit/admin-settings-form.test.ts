import { describe, expect, it } from "vitest";
import {
  canConfirmSettingsSave,
  createSettingsSaveConfirmation,
  createSettingsDraft,
  materializeSettingsDraft,
  settingsTabRequiresConfirmation,
  textToList,
} from "@/lib/admin-settings-form";
import type { AdminSettings } from "../../../../packages/sdk/typescript/src/types.gen";

const baseSettings: AdminSettings = {
  general: {
    site_name: "SRapi",
    logo_url: "",
    version_label: "v0.1.0",
    custom_menus: [{ label: "Docs", href: "https://example.invalid/docs" }],
  },
  agreement: {
    user_agreement: "",
    privacy_policy: "",
  },
  features: {
    enabled_channels: ["openai-compatible"],
    channel_monitoring_enabled: true,
    invitation_rebate_enabled: false,
    payments_enabled: false,
  },
  security: {
    admin_api_key: { configured: true },
    registration_enabled: true,
    oauth_enabled: false,
    oauth_providers: [],
  },
  users: {
    default_balance: "0",
    default_group: "default",
    user_self_delete_enabled: false,
    rpm_limit_default: 0,
  },
  gateway: {
    overload_cooldown_seconds: 30,
    rate_limit_cooldown_seconds: 30,
    stream_timeout_seconds: 600,
    request_shaper_enabled: true,
    beta_strategy: "allow_configured",
    scheduler_strategy_rollout_enabled: false,
    scheduler_strategy_shadow_strategy: "cost_saver",
    scheduler_strategy_rollout_percent: 0,
    scheduler_strategy_rollout_models: [],
    scheduler_strategy_rollout_api_key_hashes: [],
  },
  payment: {
    enabled: false,
    providers: [],
    subscription_plans_enabled: false,
  },
  email: {
    smtp_configured: true,
    templates: { welcome: "Hello" },
  },
  backup: {
    enabled: false,
    last_backup_at: "2026-05-24T00:00:00Z",
    retention_days: 30,
  },
};

describe("admin-settings-form", () => {
  it("splits newline and comma separated lists", () => {
    expect(textToList("openai-compatible\nanthropic-compatible, gemini-compatible")).toEqual([
      "openai-compatible",
      "anthropic-compatible",
      "gemini-compatible",
    ]);
  });

  it("round-trips editable JSON-backed fields", () => {
    const draft = createSettingsDraft(baseSettings);
    draft.enabledChannelsText = "openai-compatible\nanthropic-compatible";
    draft.oauthProvidersText = "github, google";
    draft.schedulerRolloutModelsText = "gpt-4o\nclaude-sonnet";
    draft.schedulerRolloutApiKeyHashesText = "sha256:first, sha256:second";
    draft.paymentProvidersText = "stripe";
    draft.customMenusJson = '[{"label":"Ops","href":"/admin/ops"}]';
    draft.emailTemplatesJson = '{"welcome":"Welcome to SRapi"}';

    const materialized = materializeSettingsDraft(draft);

    expect(materialized.general.custom_menus).toEqual([{ label: "Ops", href: "/admin/ops" }]);
    expect(materialized.features.enabled_channels).toEqual([
      "openai-compatible",
      "anthropic-compatible",
    ]);
    expect(materialized.security.oauth_providers).toEqual(["github", "google"]);
    expect(materialized.gateway.scheduler_strategy_rollout_models).toEqual(["gpt-4o", "claude-sonnet"]);
    expect(materialized.gateway.scheduler_strategy_rollout_api_key_hashes).toEqual(["sha256:first", "sha256:second"]);
    expect(materialized.payment.providers).toEqual(["stripe"]);
    expect(materialized.email.templates).toEqual({ welcome: "Welcome to SRapi" });
  });

  it("rejects custom menus that are not an array of objects", () => {
    const draft = createSettingsDraft(baseSettings);
    draft.customMenusJson = '["bad"]';

    expect(() => materializeSettingsDraft(draft)).toThrow(
      "Custom menus must be a JSON array of objects.",
    );
  });

  it("reports invalid custom menu JSON without leaking parser internals", () => {
    const draft = createSettingsDraft(baseSettings);
    draft.customMenusJson = "[not-json";

    expect(() => materializeSettingsDraft(draft)).toThrow(
      "Custom menus must be valid JSON.",
    );
  });

  it("reports invalid email template JSON without leaking parser internals", () => {
    const draft = createSettingsDraft(baseSettings);
    draft.emailTemplatesJson = "{not-json";

    expect(() => materializeSettingsDraft(draft)).toThrow(
      "Email templates must be valid JSON.",
    );
  });

  it("keeps secret settings as configured state only", () => {
    const draft = createSettingsDraft(baseSettings);
    const materialized = materializeSettingsDraft(draft);

    expect(materialized.security.admin_api_key).toEqual({ configured: true });
    expect(JSON.stringify(materialized.security)).not.toContain("secret");
  });

  it("requires confirmation for high-risk settings tabs", () => {
    expect(settingsTabRequiresConfirmation("general")).toBe(false);
    expect(settingsTabRequiresConfirmation("security")).toBe(true);
    expect(settingsTabRequiresConfirmation("gateway")).toBe(true);

    const state = createSettingsSaveConfirmation("security");
    expect(state.phrase).toBe("SAVE SECURITY SETTINGS");
    expect(canConfirmSettingsSave({ ...state, confirmation: "SAVE SECURITY SETTINGS" })).toBe(true);
    expect(canConfirmSettingsSave({ ...state, confirmation: "save security settings" })).toBe(false);
  });
});
