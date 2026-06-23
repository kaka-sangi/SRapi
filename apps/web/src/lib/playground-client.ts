/**
 * Raw-fetch client for the user playground (交界地). It posts to the
 * session-authenticated, billed endpoint POST /api/v1/me/playground/chat and
 * parses the OpenAI-compatible SSE the gateway returns (content +
 * reasoning_content deltas). No tools, no admin events.
 */

import type { ReasoningEffort } from "@/components/chat/types";
import type { CopilotImagePart } from "@/lib/image-utils";
import { CSRF_STORAGE_KEY } from "@/lib/sdk-client";

export interface PlaygroundMessage {
  role: "user" | "assistant";
  content?: string;
  reasoning?: string;
  images?: CopilotImagePart[];
}

/** Per-turn gateway telemetry: which model actually served the request, token
 * accounting, and timing. Any field may be undefined on older gateways that
 * don't emit a usage chunk. */
export interface PlaygroundTurnMeta {
  servedModel?: string;
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  latencyMs?: number;
  firstTokenMs?: number;
}

function apiBaseUrl(): string {
  return (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || "").replace(/\/+$/, "");
}

function csrfToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(CSRF_STORAGE_KEY);
}

export interface StreamPlaygroundOptions {
  messages: PlaygroundMessage[];
  model: string;
  reasoningEffort?: ReasoningEffort;
  /** Optional system prompt prepended server-side. */
  system?: string;
  /** Sampling temperature (0–2); undefined keeps the model default. */
  temperature?: number;
  /** Response token cap; undefined keeps the model default. */
  maxTokens?: number;
  signal?: AbortSignal;
  onDelta: (kind: "content" | "reasoning", text: string) => void;
  /** Called once when the stream closes, with whatever telemetry was observed. */
  onMeta?: (meta: PlaygroundTurnMeta) => void;
  onError: (message: string) => void;
}

/** Streams one billed playground turn. Resolves when the stream closes. */
export async function streamPlaygroundChat(options: StreamPlaygroundOptions): Promise<void> {
  const csrf = csrfToken();
  const start = performance.now();
  let response: Response;
  try {
    response = await fetch(`${apiBaseUrl()}/api/v1/me/playground/chat`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        ...(csrf ? { "X-CSRF-Token": csrf } : {}),
      },
      body: JSON.stringify({
        model: options.model,
        reasoning_effort:
          options.reasoningEffort && options.reasoningEffort !== "off" ? options.reasoningEffort : undefined,
        system: options.system?.trim() || undefined,
        temperature: options.temperature,
        max_tokens: options.maxTokens,
        // Ask the OpenAI-compatible gateway to emit a final usage chunk so we
        // can surface token accounting. Older gateways ignore this harmlessly.
        stream_options: { include_usage: true },
        messages: options.messages.map((m) => ({
          role: m.role,
          content: m.content ?? "",
          images: m.images && m.images.length ? m.images : undefined,
        })),
      }),
      signal: options.signal,
    });
  } catch (err) {
    if ((err as Error)?.name === "AbortError") throw err;
    options.onError((err as Error)?.message ?? "Network error");
    return;
  }

  if (!response.ok || !response.body) {
    options.onError(await errorMessage(response));
    return;
  }

  // Telemetry accumulated across the stream. `start` was captured before the
  // fetch; firstTokenMs is filled when the first delta lands; token fields stay
  // undefined unless a usage chunk arrives.
  const meta: PlaygroundTurnMeta = {};
  const onDelta = (kind: "content" | "reasoning", text: string) => {
    if (meta.firstTokenMs === undefined) meta.firstTokenMs = performance.now() - start;
    options.onDelta(kind, text);
  };

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      handleFrame(buffer.slice(0, boundary), onDelta, meta);
      buffer = buffer.slice(boundary + 2);
      boundary = buffer.indexOf("\n\n");
    }
  }
  // Final flush: a multibyte UTF-8 char split across the last chunk boundary is
  // still pending in the decoder, so drain it before processing the tail. Then
  // handle any complete frame(s) left in the buffer (a stream may close without
  // a trailing blank line), and the final partial frame if it carries data.
  buffer += decoder.decode();
  let boundary = buffer.indexOf("\n\n");
  while (boundary >= 0) {
    handleFrame(buffer.slice(0, boundary), onDelta, meta);
    buffer = buffer.slice(boundary + 2);
    boundary = buffer.indexOf("\n\n");
  }
  if (buffer) handleFrame(buffer, onDelta, meta);

  meta.latencyMs = performance.now() - start;
  options.onMeta?.(meta);
}

function handleFrame(
  frame: string,
  onDelta: (kind: "content" | "reasoning", text: string) => void,
  meta: PlaygroundTurnMeta,
) {
  for (const line of frame.split("\n")) {
    if (!line.startsWith("data:")) continue;
    const payload = line.slice(5).trim();
    if (!payload || payload === "[DONE]") continue;
    try {
      const chunk = JSON.parse(payload) as {
        model?: string;
        choices?: Array<{ delta?: { content?: string; reasoning_content?: string } }>;
        usage?: { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number };
      };
      // The served model rides on every chunk's top-level `model`; keep the last
      // seen value. Usage typically arrives in a final chunk whose `choices` is
      // empty, so read both BEFORE bailing on a missing delta.
      if (typeof chunk.model === "string" && chunk.model) meta.servedModel = chunk.model;
      if (chunk.usage) {
        if (typeof chunk.usage.prompt_tokens === "number") meta.promptTokens = chunk.usage.prompt_tokens;
        if (typeof chunk.usage.completion_tokens === "number") meta.completionTokens = chunk.usage.completion_tokens;
        if (typeof chunk.usage.total_tokens === "number") meta.totalTokens = chunk.usage.total_tokens;
      }
      const delta = chunk.choices?.[0]?.delta;
      if (!delta) continue;
      if (delta.reasoning_content) onDelta("reasoning", delta.reasoning_content);
      if (delta.content) onDelta("content", delta.content);
    } catch {
      // ignore malformed chunks
    }
  }
}

async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { error?: { message?: string }; message?: string };
    return body?.error?.message ?? body?.message ?? `Request failed (${response.status})`;
  } catch {
    return `Request failed (${response.status})`;
  }
}
