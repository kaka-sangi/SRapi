// ---------------------------------------------------------------------------
// Platform presets (mirrors backend provider/preset registry)
// ---------------------------------------------------------------------------

export type AuthType = "api_key" | "oauth_refresh";

export interface PlatformPreset {
  key: string;
  name: string;
  description: string;
  authTypes: AuthType[];
  defaultModels: string[];
  custom?: boolean;
}

export const PLATFORMS: PlatformPreset[] = [
  {
    key: "codex-cli",
    name: "Codex CLI",
    description: "OpenAI Codex via ChatGPT backend",
    authTypes: ["oauth_refresh"],
    defaultModels: [
      "gpt-5.5",
      "gpt-5.4",
      "gpt-5.4-mini",
      "codex-auto-review",
      "gpt-5.3-codex",
      "gpt-5.3-codex-spark",
      "gpt-5.2",
      "codex-mini-latest",
    ],
  },
  {
    key: "openai",
    name: "OpenAI",
    description: "GPT / o-series via API key",
    authTypes: ["api_key"],
    defaultModels: [
      "gpt-5.5",
      "gpt-5.4",
      "gpt-5.4-mini",
      "gpt-4.1",
      "gpt-4.1-mini",
      "gpt-4.1-nano",
      "o4-mini",
      "o3",
      "o3-pro",
    ],
  },
  {
    key: "anthropic",
    name: "Anthropic",
    description: "Claude Opus / Sonnet / Haiku",
    authTypes: ["api_key"],
    defaultModels: ["claude-fable-5", "claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"],
  },
  {
    key: "deepseek",
    name: "DeepSeek",
    description: "DeepSeek R1 / V3 / Coder",
    authTypes: ["api_key"],
    defaultModels: ["deepseek-r1", "deepseek-v3-0324", "deepseek-chat", "deepseek-reasoner"],
  },
  {
    key: "groq",
    name: "Groq",
    description: "Ultra-fast inference via Groq Cloud",
    authTypes: ["api_key"],
    defaultModels: ["llama-4-scout-17b-16e-instruct", "llama-4-maverick-17b-128e-instruct", "qwen-qwq-32b", "deepseek-r1-distill-llama-70b", "llama-3.3-70b-versatile", "llama-3.1-8b-instant"],
  },
  {
    key: "mistral",
    name: "Mistral",
    description: "Mistral Large / Medium / Small",
    authTypes: ["api_key"],
    defaultModels: ["mistral-large-latest", "mistral-medium-latest", "mistral-small-latest", "codestral-latest", "open-mistral-nemo"],
  },
  {
    key: "gemini",
    name: "Google Gemini",
    description: "Gemini 2.5 Pro / Flash via API key",
    authTypes: ["api_key"],
    defaultModels: ["gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"],
  },
  {
    key: "grok",
    name: "Grok",
    description: "xAI Grok models via API",
    authTypes: ["api_key"],
    defaultModels: ["grok-3", "grok-3-mini", "grok-2"],
  },
  {
    key: "kimi",
    name: "Kimi / Moonshot",
    description: "Moonshot AI models via API",
    authTypes: ["api_key"],
    defaultModels: ["moonshot-v1-128k", "moonshot-v1-32k", "moonshot-v1-8k"],
  },
  {
    key: "qwen",
    name: "Qwen",
    description: "Alibaba Qwen models via DashScope",
    authTypes: ["api_key"],
    defaultModels: ["qwen-max", "qwen-plus", "qwen-turbo", "qwen-long"],
  },
  {
    key: "together",
    name: "Together AI",
    description: "Open-source models via Together",
    authTypes: ["api_key"],
    defaultModels: ["meta-llama/Llama-4-Scout-17B-16E-Instruct", "meta-llama/Llama-4-Maverick-17B-128E-Instruct", "deepseek-ai/DeepSeek-R1"],
  },
  {
    key: "cerebras",
    name: "Cerebras",
    description: "Ultra-fast inference on Cerebras hardware",
    authTypes: ["api_key"],
    defaultModels: ["llama-4-scout-17b-16e-instruct", "llama3.3-70b"],
  },
  {
    key: "openrouter",
    name: "OpenRouter",
    description: "Multi-provider aggregator",
    authTypes: ["api_key"],
    defaultModels: ["openai/gpt-4.1", "anthropic/claude-sonnet-4-6", "google/gemini-2.5-pro", "deepseek/deepseek-r1", "meta-llama/llama-4-scout"],
  },
  {
    key: "custom",
    name: "Custom",
    description: "Any OpenAI-compatible API endpoint",
    authTypes: ["api_key"],
    defaultModels: [],
    custom: true,
  },
];

// ---------------------------------------------------------------------------
// Platform icon abbreviations
// ---------------------------------------------------------------------------

export const PLATFORM_ICONS: Record<string, string> = {
  "codex-cli": "CX",
  openai: "OA",
  anthropic: "AN",
  deepseek: "DS",
  groq: "GQ",
  mistral: "MI",
  gemini: "GE",
  grok: "XA",
  kimi: "KM",
  qwen: "QW",
  together: "TG",
  cerebras: "CB",
  openrouter: "OR",
  custom: "++",
};

export const PLATFORM_ICON_COLORS: Record<string, string> = {
  "codex-cli": "bg-emerald-50 text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-400",
  openai: "bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400",
  anthropic: "bg-orange-50 text-orange-600 dark:bg-orange-950/40 dark:text-orange-400",
  deepseek: "bg-blue-50 text-blue-600 dark:bg-blue-950/40 dark:text-blue-400",
  groq: "bg-amber-50 text-amber-600 dark:bg-amber-950/40 dark:text-amber-400",
  mistral: "bg-indigo-50 text-indigo-600 dark:bg-indigo-950/40 dark:text-indigo-400",
  gemini: "bg-sky-50 text-sky-600 dark:bg-sky-950/40 dark:text-sky-400",
  grok: "bg-slate-100 text-slate-700 dark:bg-slate-800/40 dark:text-slate-300",
  kimi: "bg-teal-50 text-teal-600 dark:bg-teal-950/40 dark:text-teal-400",
  qwen: "bg-purple-50 text-purple-600 dark:bg-purple-950/40 dark:text-purple-400",
  together: "bg-rose-50 text-rose-600 dark:bg-rose-950/40 dark:text-rose-400",
  cerebras: "bg-cyan-50 text-cyan-600 dark:bg-cyan-950/40 dark:text-cyan-400",
  openrouter: "bg-violet-50 text-violet-600 dark:bg-violet-950/40 dark:text-violet-400",
};

// ---------------------------------------------------------------------------
// Step flow
// ---------------------------------------------------------------------------

export type Step = "platform" | "credentials" | "result";

export const STEPS: Step[] = ["platform", "credentials", "result"];
