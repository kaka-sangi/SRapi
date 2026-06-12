/**
 * Raw-fetch client for the user playground (交界地). It posts to the
 * session-authenticated, billed endpoint POST /api/v1/me/playground/chat and
 * parses the OpenAI-compatible SSE the gateway returns (content +
 * reasoning_content deltas). No tools, no admin events.
 */

import type { ReasoningEffort } from "@/components/chat/types";
import type { CopilotImagePart } from "@/lib/image-utils";

const CSRF_STORAGE_KEY = "srapi_csrf_token";

export interface PlaygroundMessage {
  role: "user" | "assistant";
  content?: string;
  reasoning?: string;
  images?: CopilotImagePart[];
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
  onError: (message: string) => void;
}

/** Streams one billed playground turn. Resolves when the stream closes. */
export async function streamPlaygroundChat(options: StreamPlaygroundOptions): Promise<void> {
  const csrf = csrfToken();
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

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      handleFrame(buffer.slice(0, boundary), options.onDelta);
      buffer = buffer.slice(boundary + 2);
      boundary = buffer.indexOf("\n\n");
    }
  }
}

function handleFrame(frame: string, onDelta: (kind: "content" | "reasoning", text: string) => void) {
  for (const line of frame.split("\n")) {
    if (!line.startsWith("data:")) continue;
    const payload = line.slice(5).trim();
    if (!payload || payload === "[DONE]") continue;
    try {
      const chunk = JSON.parse(payload) as {
        choices?: Array<{ delta?: { content?: string; reasoning_content?: string } }>;
      };
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
