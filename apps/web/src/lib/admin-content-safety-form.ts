import type {
  ContentSafetyConfig,
  ContentSafetyMode,
} from "../../../../packages/sdk/typescript/src/types.gen";

export interface ContentSafetyFormState {
  enabled: boolean;
  mode: ContentSafetyMode;
  redactPii: boolean;
  blockPii: boolean;
  blockPromptInjection: boolean;
  blockCustomKeywords: boolean;
  customKeywords: string[];
  modelScopes: string[];
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
    modelScopes: cleanList(config.model_scopes),
  };
}

export function buildContentSafetyConfig(form: ContentSafetyFormState): ContentSafetyConfig {
  return {
    enabled: form.enabled,
    mode: form.mode,
    redact_pii: form.redactPii,
    block_pii: form.blockPii,
    block_prompt_injection: form.blockPromptInjection,
    block_custom_keywords: form.blockCustomKeywords,
    custom_keywords: cleanList(form.customKeywords),
    model_scopes: cleanList(form.modelScopes).map((scope) => scope.toLowerCase()),
  };
}

function cleanList(items: string[]): string[] {
  const out: string[] = [];
  for (const item of items) {
    const trimmed = item.trim();
    if (trimmed && !out.includes(trimmed)) out.push(trimmed);
  }
  return out;
}
