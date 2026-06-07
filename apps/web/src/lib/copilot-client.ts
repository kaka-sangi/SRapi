/**
 * Raw-fetch SSE client for the admin copilot chat endpoint. We bypass the
 * generated SDK here because the response is a Server-Sent-Events stream, not
 * JSON. Same-origin (`/api/...` via the Next rewrite) carries the session
 * cookie; the CSRF token mirrors lib/api.ts (localStorage `srapi_csrf_token`).
 */

const CSRF_STORAGE_KEY = "srapi_csrf_token";

export interface CopilotToolCall {
  id: string;
  name: string;
  arguments: string;
}

export interface CopilotToolResult {
  tool_call_id: string;
  content: string;
  is_error?: boolean;
}

export interface CopilotImage {
  mime_type: string;
  data: string;
}

/** A text-file attachment. Frontend-only: folded into `content` on send (the
 * backend has no `files` field), so it works with any model. */
export interface CopilotFile {
  name: string;
  content: string;
  truncated?: boolean;
}

export type ReasoningEffort = "off" | "low" | "medium" | "high";

export interface CopilotMessage {
  role: "user" | "assistant" | "tool";
  content?: string;
  reasoning?: string;
  images?: CopilotImage[];
  files?: CopilotFile[];
  tool_calls?: CopilotToolCall[];
  tool_results?: CopilotToolResult[];
}

/** Folds a message's text-file attachments into its content and drops the
 * frontend-only `files` field, so the wire payload matches the backend schema
 * (which rejects unknown fields). Images stay as native `images` parts. */
function foldFileAttachments(message: CopilotMessage): CopilotMessage {
  if (!message.files?.length) return message;
  const { files, ...rest } = message;
  const blocks = files
    .map(
      (f) =>
        `\n\n[Attached file: ${f.name}]${f.truncated ? " (truncated)" : ""}\n\`\`\`\n${f.content}\n\`\`\``,
    )
    .join("");
  return { ...rest, content: `${message.content ?? ""}${blocks}`.trim() };
}

export interface CopilotApproval {
  tool_call_id: string;
  approved: boolean;
}

export type CopilotEvent =
  | { type: "assistant_reasoning"; data: { text: string } }
  | { type: "assistant_delta"; data: { text: string } }
  | { type: "tool_call"; data: { tool_call_id: string; name: string; arguments: string } }
  | {
      type: "tool_result";
      data: { tool_call_id: string; name: string; status?: number; content: string; is_error?: boolean };
    }
  | {
      type: "pending_action";
      data: {
        tool_call_id: string;
        name: string;
        method: string;
        path: string;
        body?: string;
        summary?: string;
        danger?: boolean;
      };
    }
  | { type: "done"; data: { messages: CopilotMessage[] } }
  | { type: "error"; data: { message: string } };

function apiBaseUrl(): string {
  return (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || "").replace(/\/+$/, "");
}

function csrfToken(): string | null {
  if (typeof window === "undefined") return null;
  return window.localStorage.getItem(CSRF_STORAGE_KEY);
}

export interface StreamCopilotChatOptions {
  messages: CopilotMessage[];
  approval?: CopilotApproval;
  model?: string;
  reasoningEffort?: ReasoningEffort;
  signal?: AbortSignal;
  onEvent: (event: CopilotEvent) => void;
}

/** POSTs one copilot turn and dispatches each SSE frame to onEvent. Resolves
 * when the stream closes; never throws for protocol errors (it emits an
 * `error` event instead) — only a thrown AbortError propagates. */
export async function streamCopilotChat(options: StreamCopilotChatOptions): Promise<void> {
  const csrf = csrfToken();
  let response: Response;
  try {
    response = await fetch(`${apiBaseUrl()}/api/v1/admin/copilot/chat`, {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        ...(csrf ? { "X-CSRF-Token": csrf } : {}),
      },
      body: JSON.stringify({
        messages: options.messages.map(foldFileAttachments),
        approval: options.approval,
        model: options.model || undefined,
        reasoning_effort: options.reasoningEffort && options.reasoningEffort !== "off" ? options.reasoningEffort : undefined,
      }),
      signal: options.signal,
    });
  } catch (err) {
    if ((err as Error)?.name === "AbortError") throw err;
    options.onEvent({ type: "error", data: { message: (err as Error)?.message ?? "Network error" } });
    return;
  }

  if (!response.ok || !response.body) {
    options.onEvent({ type: "error", data: { message: await errorMessage(response) } });
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
      const frame = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      const event = parseFrame(frame);
      if (event) options.onEvent(event);
      boundary = buffer.indexOf("\n\n");
    }
  }
}

function parseFrame(frame: string): CopilotEvent | null {
  let eventName = "";
  const dataLines: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event:")) eventName = line.slice(6).trim();
    else if (line.startsWith("data:")) dataLines.push(line.slice(5).trim());
  }
  if (!eventName) return null;
  try {
    const data = JSON.parse(dataLines.join("\n") || "{}");
    return { type: eventName, data } as CopilotEvent;
  } catch {
    return null;
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
