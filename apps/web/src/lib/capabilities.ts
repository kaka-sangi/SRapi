import type { CapabilityDescriptor } from "../../../../packages/sdk/typescript/src/types.gen";

/**
 * Endpoint capabilities a model can expose, surfaced as graphical chips instead
 * of a hand-written `CapabilityDescriptor[]` JSON array. Keys match the canonical
 * capability taxonomy (specs/design/CAPABILITY_TAXONOMY_SPEC.md); labels are the standard
 * API endpoint names (kept verbatim per PRODUCT_TONE "term's real name").
 */
export const MODEL_CAPABILITY_OPTIONS: { value: string; label: string }[] = [
  { value: "chat_completions", label: "Chat completions" },
  { value: "responses", label: "Responses" },
  { value: "responses_compact", label: "Responses · compact" },
  { value: "responses_input_items", label: "Responses · input items" },
  { value: "anthropic_count_tokens", label: "Anthropic · count tokens" },
  { value: "gemini_generate_content", label: "Gemini · generate content" },
  { value: "gemini_count_tokens", label: "Gemini · count tokens" },
  { value: "embeddings", label: "Embeddings" },
  { value: "image_generations", label: "Images · generations" },
  { value: "image_edits", label: "Images · edits" },
  { value: "image_variations", label: "Images · variations" },
  { value: "videos", label: "Videos" },
  { value: "audio_transcriptions", label: "Audio · transcriptions" },
  { value: "audio_speech", label: "Audio · speech" },
  { value: "moderations", label: "Moderations" },
  { value: "rerank", label: "Rerank" },
  { value: "token_counting", label: "Token counting" },
  { value: "realtime_websocket", label: "Realtime · WebSocket" },
];

/** Keys of a model's existing capability descriptors, for the chip selector. */
export function descriptorsToCapabilityKeys(
  descriptors: CapabilityDescriptor[] | undefined,
): string[] {
  return (descriptors ?? []).map((d) => d.key);
}

/**
 * Build the canonical descriptor array from selected capability keys. Every
 * selected capability is recorded as the standard `v1 / stable / required`
 * descriptor — the shape the scheduler hard-filters on. Per-capability
 * version/status/level tuning is intentionally not exposed graphically.
 */
export function capabilityKeysToDescriptors(keys: string[]): CapabilityDescriptor[] {
  return keys.map((key) => ({
    key,
    version: "v1",
    status: "stable",
    level: "required",
  }));
}
