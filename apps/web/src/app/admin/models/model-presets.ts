export interface ModelPreset {
  displayName: string;
  family: string;
  contextWindow?: number;
  maxOutputTokens?: number;
  qualityTier?: string;
}

export const MODEL_PRESETS: Record<string, ModelPreset> = {
  // Anthropic
  "claude-fable-5": { displayName: "Claude Fable 5", family: "claude", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "premium" },
  "claude-opus-4-8": { displayName: "Claude Opus 4.8", family: "claude", contextWindow: 1048576, maxOutputTokens: 32768, qualityTier: "premium" },
  "claude-opus-4-7": { displayName: "Claude Opus 4.7", family: "claude", contextWindow: 1048576, maxOutputTokens: 32768, qualityTier: "premium" },
  "claude-opus-4-6": { displayName: "Claude Opus 4.6", family: "claude", contextWindow: 1048576, maxOutputTokens: 32768, qualityTier: "premium" },
  "claude-sonnet-4-6": { displayName: "Claude Sonnet 4.6", family: "claude", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "standard" },
  "claude-haiku-4-5": { displayName: "Claude Haiku 4.5", family: "claude", contextWindow: 1048576, maxOutputTokens: 8192, qualityTier: "economy" },
  // OpenAI
  "gpt-5.5": { displayName: "GPT-5.5", family: "gpt", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "premium" },
  "gpt-5.4": { displayName: "GPT-5.4", family: "gpt", contextWindow: 1048576, maxOutputTokens: 32768, qualityTier: "premium" },
  "gpt-5.4-mini": { displayName: "GPT-5.4 Mini", family: "gpt", contextWindow: 1048576, maxOutputTokens: 16384, qualityTier: "economy" },
  "gpt-4.1": { displayName: "GPT-4.1", family: "gpt", contextWindow: 1047576, maxOutputTokens: 32768, qualityTier: "standard" },
  "gpt-4.1-mini": { displayName: "GPT-4.1 Mini", family: "gpt", contextWindow: 1047576, maxOutputTokens: 16384, qualityTier: "economy" },
  "gpt-4.1-nano": { displayName: "GPT-4.1 Nano", family: "gpt", contextWindow: 1047576, maxOutputTokens: 16384, qualityTier: "economy" },
  "o4-mini": { displayName: "o4-mini", family: "o-series", contextWindow: 200000, maxOutputTokens: 100000, qualityTier: "standard" },
  "o3": { displayName: "o3", family: "o-series", contextWindow: 200000, maxOutputTokens: 100000, qualityTier: "premium" },
  "o3-pro": { displayName: "o3-pro", family: "o-series", contextWindow: 200000, maxOutputTokens: 100000, qualityTier: "premium" },
  "gpt-5.4-nano": { displayName: "GPT-5.4 Nano", family: "gpt", contextWindow: 1048576, maxOutputTokens: 8192, qualityTier: "economy" },
  "gpt-5.2": { displayName: "GPT-5.2", family: "gpt", contextWindow: 1048576, maxOutputTokens: 32768, qualityTier: "standard" },
  // Codex CLI
  "codex-mini-latest": { displayName: "Codex Mini", family: "codex", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "standard" },
  "codex-auto-review": { displayName: "Codex Auto Review", family: "codex", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "standard" },
  "gpt-5.3-codex": { displayName: "GPT-5.3 Codex", family: "codex", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "standard" },
  "gpt-5.3-codex-spark": { displayName: "GPT-5.3 Codex Spark", family: "codex", contextWindow: 1048576, maxOutputTokens: 32768, qualityTier: "economy" },
  // Gemini
  "gemini-2.5-pro": { displayName: "Gemini 2.5 Pro", family: "gemini", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "premium" },
  "gemini-2.5-flash": { displayName: "Gemini 2.5 Flash", family: "gemini", contextWindow: 1048576, maxOutputTokens: 65536, qualityTier: "standard" },
  "gemini-2.0-flash": { displayName: "Gemini 2.0 Flash", family: "gemini", contextWindow: 1048576, maxOutputTokens: 8192, qualityTier: "economy" },
  // DeepSeek
  "deepseek-r1": { displayName: "DeepSeek R1", family: "deepseek", contextWindow: 65536, maxOutputTokens: 8192, qualityTier: "standard" },
  "deepseek-v3-0324": { displayName: "DeepSeek V3", family: "deepseek", contextWindow: 65536, maxOutputTokens: 8192, qualityTier: "standard" },
  "deepseek-chat": { displayName: "DeepSeek Chat", family: "deepseek", contextWindow: 65536, maxOutputTokens: 8192, qualityTier: "economy" },
  // Grok
  "grok-3": { displayName: "Grok 3", family: "grok", contextWindow: 131072, maxOutputTokens: 16384, qualityTier: "premium" },
  "grok-3-mini": { displayName: "Grok 3 Mini", family: "grok", contextWindow: 131072, maxOutputTokens: 16384, qualityTier: "standard" },
  // Mistral
  "mistral-large-latest": { displayName: "Mistral Large", family: "mistral", contextWindow: 131072, maxOutputTokens: 8192, qualityTier: "premium" },
  "mistral-small-latest": { displayName: "Mistral Small", family: "mistral", contextWindow: 131072, maxOutputTokens: 8192, qualityTier: "economy" },
  "codestral-latest": { displayName: "Codestral", family: "mistral", contextWindow: 262144, maxOutputTokens: 8192, qualityTier: "standard" },
};

export function getModelPreset(canonicalName: string): ModelPreset | null {
  return MODEL_PRESETS[canonicalName] ?? null;
}
