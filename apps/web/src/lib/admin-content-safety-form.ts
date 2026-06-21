import type {
  ContentSafetyConfig,
  ContentSafetyConfigWritable,
  ContentSafetyMode,
  ContentSafetyModerationConfig,
  ContentSafetyModerationConfigWritable,
} from "../../../../packages/sdk/typescript/src/types.gen";

/**
 * Flat form state mirroring the existing FieldConfig dialog convention so
 * every editable knob is addressable by a single key. The moderation
 * fields are prefixed `moderation*` and re-nested when materializing the
 * API body; per-category thresholds are not editable here and are
 * round-tripped verbatim from the server (preserved across saves).
 */
export interface ContentSafetyFormState {
  enabled: boolean;
  mode: ContentSafetyMode;
  redactPii: boolean;
  blockPii: boolean;
  blockPromptInjection: boolean;
  blockCustomKeywords: boolean;
  customKeywords: string[];
  modelScopes: string[];
  moderationEnabled: boolean;
  moderationProvider: ContentSafetyModerationConfig["provider"];
  moderationModel: string;
  moderationBaseUrl: string;
  moderationBlockOnFlag: boolean;
  moderationTimeoutMs: number;
  moderationCacheTtlSeconds: number;
  moderationApiKey: string;
  moderationApiKeyConfigured: boolean;
  // The threshold map is not exposed in the basic form — preserve any
  // existing values verbatim so an operator who set them via API/snapshot
  // is not silently reset on the next dialog save.
  moderationCategories: Record<string, number>;
}

export function createContentSafetyForm(config: ContentSafetyConfig): ContentSafetyFormState {
  return {
    enabled: config.enabled,
    mode: config.mode,
    redactPii: config.redact_pii,
    blockPii: config.block_pii,
    blockPromptInjection: config.block_prompt_injection,
    blockCustomKeywords: config.block_custom_keywords,
    customKeywords: cleanList(config.custom_keywords),
    modelScopes: cleanList(config.model_scopes).map((scope) => scope.toLowerCase()),
    moderationEnabled: config.moderation.enabled,
    moderationProvider: config.moderation.provider,
    moderationModel: config.moderation.model,
    moderationBaseUrl: config.moderation.base_url,
    moderationBlockOnFlag: config.moderation.block_on_flag,
    moderationTimeoutMs: config.moderation.timeout_ms,
    moderationCacheTtlSeconds: config.moderation.cache_ttl_seconds,
    moderationApiKey: "",
    moderationApiKeyConfigured: config.moderation.api_key_configured,
    moderationCategories: cleanCategories(config.moderation.categories),
  };
}

export function buildContentSafetyConfig(form: ContentSafetyFormState): ContentSafetyConfigWritable {
  return {
    enabled: form.enabled,
    mode: form.mode,
    redact_pii: form.redactPii,
    block_pii: form.blockPii,
    block_prompt_injection: form.blockPromptInjection,
    block_custom_keywords: form.blockCustomKeywords,
    custom_keywords: cleanList(form.customKeywords),
    model_scopes: cleanList(form.modelScopes).map((scope) => scope.toLowerCase()),
    moderation: buildModerationConfig(form),
  };
}

function buildModerationConfig(form: ContentSafetyFormState): ContentSafetyModerationConfigWritable {
  const result: ContentSafetyModerationConfigWritable = {
    enabled: form.moderationEnabled,
    provider: form.moderationProvider,
    model: form.moderationModel.trim(),
    base_url: form.moderationBaseUrl.trim().replace(/\/+$/, ""),
    block_on_flag: form.moderationBlockOnFlag,
    timeout_ms: Math.max(250, Math.min(30_000, Math.trunc(form.moderationTimeoutMs) || 0)),
    cache_ttl_seconds: Math.max(0, Math.min(604_800, Math.trunc(form.moderationCacheTtlSeconds) || 0)),
    categories: cleanCategories(form.moderationCategories),
    api_key_configured: form.moderationApiKeyConfigured,
  };
  // `api_key` is write-only — only attach when the operator typed a new
  // value, so an unchanged form keeps the stored ciphertext.
  if (form.moderationApiKey.trim()) {
    result.api_key = form.moderationApiKey.trim();
  }
  return result;
}

function cleanCategories(values: ContentSafetyModerationConfig["categories"] | undefined): Record<string, number> {
  if (!values) return {};
  const out: Record<string, number> = {};
  for (const [key, value] of Object.entries(values)) {
    const name = key.trim().toLowerCase();
    if (!name) continue;
    out[name] = clampThreshold(value);
  }
  return out;
}

function clampThreshold(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(1, Math.max(0, value));
}

function cleanList(items: string[]): string[] {
  const out: string[] = [];
  for (const item of items) {
    const trimmed = item.trim();
    if (trimmed && !out.includes(trimmed)) out.push(trimmed);
  }
  return out;
}
