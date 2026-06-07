"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Bot,
  User as UserIcon,
  Wrench,
  Check,
  X,
  ChevronDown,
  AlertTriangle,
  Loader2,
  Plus,
} from "lucide-react";
import { FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/ui/markdown";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Composer } from "@/components/chat/composer";
import { ReasoningBlock } from "@/components/chat/reasoning-block";
import type { ReasoningEffort } from "@/components/chat/types";
import {
  streamCopilotChat,
  type CopilotMessage,
  type CopilotEvent,
  type CopilotImage,
} from "@/lib/copilot-client";
import {
  fileToImagePart,
  imagePartToDataUrl,
  fileToTextPart,
  isImageFile,
  isTextFile,
  type CopilotFilePart,
} from "@/lib/image-utils";

interface PendingAction {
  tool_call_id: string;
  name: string;
  method: string;
  path: string;
  body?: string;
  summary?: string;
  danger?: boolean;
}

// The copilot is intentionally stateless server-side (transcripts can carry user
// PII and are never persisted to the backend). To avoid losing an in-progress
// conversation on an accidental refresh, mirror it to sessionStorage — same-tab,
// cleared when the tab closes, never sent anywhere.
const CONVERSATION_STORAGE_KEY = "srapi.copilot.conversation";

function loadStoredConversation(): CopilotMessage[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.sessionStorage.getItem(CONVERSATION_STORAGE_KEY);
    const parsed = raw ? (JSON.parse(raw) as unknown) : null;
    return Array.isArray(parsed) ? (parsed as CopilotMessage[]) : [];
  } catch {
    return [];
  }
}

export function CopilotChat({ models, defaultModel }: { models: string[]; defaultModel: string }) {
  const { t } = useLanguage();
  // Lazily restore from sessionStorage. Safe as a lazy initializer because this
  // component only ever mounts client-side (it renders behind PageQueryState
  // once the config query resolves), so there is no SSR/hydration mismatch.
  const [messages, setMessages] = useState<CopilotMessage[]>(loadStoredConversation);
  const [input, setInput] = useState("");
  const [running, setRunning] = useState(false);
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [model, setModel] = useState(defaultModel || models[0] || "");
  const [effort, setEffort] = useState<ReasoningEffort>("off");
  const [images, setImages] = useState<CopilotImage[]>([]);
  const [files, setFiles] = useState<CopilotFilePart[]>([]);
  const { toast } = useToast();
  const abortRef = useRef<AbortController | null>(null);
  const endRef = useRef<HTMLDivElement | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const resultsById = useMemo(() => {
    const map = new Map<string, { content: string; is_error?: boolean }>();
    for (const m of messages) {
      if (m.role === "tool" && m.tool_results) {
        for (const r of m.tool_results) map.set(r.tool_call_id, { content: r.content, is_error: r.is_error });
      }
    }
    return map;
  }, [messages]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages, pending, running]);

  // Persist the conversation so a refresh doesn't lose it; "新对话" (reset → []) clears it.
  useEffect(() => {
    try {
      if (messages.length === 0) {
        window.sessionStorage.removeItem(CONVERSATION_STORAGE_KEY);
      } else {
        window.sessionStorage.setItem(CONVERSATION_STORAGE_KEY, JSON.stringify(messages));
      }
    } catch {
      // sessionStorage unavailable or over quota (e.g. large inline images) — the
      // in-memory conversation still works, it just won't survive a refresh.
    }
  }, [messages]);

  const runTurn = useCallback(
    async (history: CopilotMessage[], approval?: { tool_call_id: string; approved: boolean }) => {
      setRunning(true);
      setError(null);
      setPending(null);
      const controller = new AbortController();
      abortRef.current = controller;
      const working: CopilotMessage[] = [...history];
      let assistantIdx = -1;

      const ensureAssistant = () => {
        if (assistantIdx < 0) {
          working.push({ role: "assistant", content: "", tool_calls: [] });
          assistantIdx = working.length - 1;
        }
      };

      const onEvent = (event: CopilotEvent) => {
        switch (event.type) {
          case "assistant_reasoning": {
            ensureAssistant();
            working[assistantIdx] = {
              ...working[assistantIdx],
              reasoning: (working[assistantIdx].reasoning ?? "") + event.data.text,
            };
            setMessages([...working]);
            break;
          }
          case "assistant_delta": {
            ensureAssistant();
            working[assistantIdx] = {
              ...working[assistantIdx],
              content: (working[assistantIdx].content ?? "") + event.data.text,
            };
            setMessages([...working]);
            break;
          }
          case "tool_call": {
            if (assistantIdx < 0) {
              working.push({ role: "assistant", content: "", tool_calls: [] });
              assistantIdx = working.length - 1;
            }
            const current = working[assistantIdx];
            working[assistantIdx] = {
              ...current,
              tool_calls: [
                ...(current.tool_calls ?? []),
                { id: event.data.tool_call_id, name: event.data.name, arguments: event.data.arguments },
              ],
            };
            setMessages([...working]);
            break;
          }
          case "tool_result": {
            working.push({
              role: "tool",
              tool_results: [
                { tool_call_id: event.data.tool_call_id, content: event.data.content, is_error: event.data.is_error },
              ],
            });
            setMessages([...working]);
            break;
          }
          case "pending_action":
            setPending(event.data);
            break;
          case "done":
            setMessages(event.data.messages);
            setPending(null);
            break;
          case "error":
            setError(event.data.message);
            break;
        }
      };

      try {
        await streamCopilotChat({
          messages: history,
          approval,
          model: model || undefined,
          reasoningEffort: effort,
          signal: controller.signal,
          onEvent,
        });
      } catch (err) {
        if ((err as Error)?.name !== "AbortError") setError((err as Error)?.message ?? "Stream error");
      } finally {
        setRunning(false);
        abortRef.current = null;
      }
    },
    [model, effort],
  );

  function send(text?: string) {
    const content = (text ?? input).trim();
    if ((!content && images.length === 0 && files.length === 0) || running) return;
    const userMsg: CopilotMessage = { role: "user", content };
    if (images.length) userMsg.images = images;
    if (files.length) userMsg.files = files;
    const next = [...messages, userMsg];
    setMessages(next);
    setInput("");
    setImages([]);
    setFiles([]);
    void runTurn(next);
  }

  function resolvePending(approved: boolean) {
    if (!pending) return;
    void runTurn(messages, { tool_call_id: pending.tool_call_id, approved });
  }

  function stop() {
    abortRef.current?.abort();
  }

  function reset() {
    abortRef.current?.abort();
    setMessages([]);
    setPending(null);
    setError(null);
    setImages([]);
    setFiles([]);
  }

  async function onPickFiles(picked: FileList | null) {
    if (!picked?.length) return;
    const list = Array.from(picked);
    const imageFiles = list.filter(isImageFile);
    const textFiles = list.filter((f) => !isImageFile(f) && isTextFile(f));
    const rejected = list.filter((f) => !isImageFile(f) && !isTextFile(f));

    try {
      if (imageFiles.length) {
        const parts = await Promise.all(imageFiles.map(fileToImagePart));
        setImages((prev) => [...prev, ...parts]);
      }
      if (textFiles.length) {
        const settled = await Promise.allSettled(textFiles.map(fileToTextPart));
        const ok = settled.filter((r) => r.status === "fulfilled").map((r) => r.value);
        if (ok.length) setFiles((prev) => [...prev, ...ok]);
        if (settled.some((r) => r.status === "rejected")) {
          toast({ title: t("copilot.fileReadFailed"), tone: "error" });
        }
      }
    } catch {
      toast({ title: t("copilot.fileReadFailed"), tone: "error" });
    }
    if (rejected.length) {
      toast({
        title: t("copilot.fileUnsupported", { name: rejected[0].name }),
        tone: "error",
      });
    }
    if (fileRef.current) fileRef.current.value = "";
  }

  const empty = messages.length === 0 && !running;

  return (
    <div className="relative flex h-[calc(100vh-9rem)] min-h-[30rem] flex-col">
      {!empty ? (
        <div className="pointer-events-none absolute right-0 top-0 z-10">
          <Button variant="ghost" size="sm" className="pointer-events-auto" onClick={reset}>
            <Plus className="size-4" />
            {t("copilot.newChat")}
          </Button>
        </div>
      ) : null}

      <div className="flex-1 overflow-y-auto px-1 pb-4">
        {empty ? (
          <EmptyState onPick={(s) => send(s)} />
        ) : (
          <div className="mx-auto max-w-3xl space-y-5 py-4">
            {messages.map((message, i) => (
              <MessageRow key={i} message={message} resultsById={resultsById} />
            ))}
            {running && !pending ? (
              <div className="flex items-center gap-2 pl-9 text-sm text-srapi-text-tertiary">
                <Loader2 className="size-4 animate-spin" />
                {t("copilot.thinking")}
              </div>
            ) : null}
            {pending ? <PendingActionBanner action={pending} onResolve={resolvePending} disabled={running} /> : null}
            {error ? (
              <div className="ml-9 flex items-start gap-2 rounded-xl border border-srapi-error/30 bg-srapi-error/5 px-3 py-2 text-sm text-srapi-error">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                <span>{error}</span>
              </div>
            ) : null}
            <div ref={endRef} />
          </div>
        )}
      </div>

      <div className="mx-auto w-full max-w-3xl">
        <Composer
          input={input}
          setInput={setInput}
          onSend={() => send()}
          onStop={stop}
          running={running}
          models={models}
          model={model}
          setModel={setModel}
          effort={effort}
          setEffort={setEffort}
          images={images}
          removeImage={(idx) => setImages((prev) => prev.filter((_, i) => i !== idx))}
          files={files}
          removeFile={(idx) => setFiles((prev) => prev.filter((_, i) => i !== idx))}
          onAttach={() => fileRef.current?.click()}
          placeholder={t("copilot.placeholder")}
        />
        <input
          ref={fileRef}
          type="file"
          accept="image/*,text/*,.txt,.md,.markdown,.csv,.tsv,.json,.jsonl,.ndjson,.yaml,.yml,.xml,.html,.log,.ini,.conf,.cfg,.toml,.env,.sql,.sh,.bash,.ps1,.js,.mjs,.cjs,.jsx,.ts,.tsx,.go,.py,.rb,.rs,.c,.h,.cc,.cpp,.hpp,.cs,.java,.kt,.php,.css,.scss,.vue,.svelte,.graphql,.proto,.diff,.patch,.tf,.hcl,.dockerfile,.gradle"
          multiple
          hidden
          onChange={(e) => void onPickFiles(e.currentTarget.files)}
        />
        <p className="mt-1.5 px-1 text-center text-2xs text-srapi-text-tertiary">{t("copilot.egressWarning")}</p>
      </div>
    </div>
  );
}

function EmptyState({ onPick }: { onPick: (s: string) => void }) {
  const { t } = useLanguage();
  const examples = [t("copilot.example1"), t("copilot.example2"), t("copilot.example3")];
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-4 text-center">
      <div className="flex size-14 items-center justify-center rounded-2xl bg-srapi-primary/10">
        <Bot className="size-7 text-srapi-primary" />
      </div>
      <div className="space-y-1.5">
        <h2 className="font-serif text-2xl text-srapi-text-primary">{t("copilot.greeting")}</h2>
        <p className="mx-auto max-w-md text-sm text-srapi-text-secondary">{t("copilot.emptyHint")}</p>
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        {examples.map((ex) => (
          <button
            key={ex}
            type="button"
            onClick={() => onPick(ex)}
            className="rounded-full border border-srapi-border bg-srapi-card px-3.5 py-1.5 text-xs text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            {ex}
          </button>
        ))}
      </div>
    </div>
  );
}

function MessageRow({
  message,
  resultsById,
}: {
  message: CopilotMessage;
  resultsById: Map<string, { content: string; is_error?: boolean }>;
}) {
  if (message.role === "tool") return null;

  if (message.role === "user") {
    return (
      <div className="flex justify-end gap-2">
        <div className="max-w-[80%] space-y-2">
          {message.images?.length ? (
            <div className="flex flex-wrap justify-end gap-1.5">
              {message.images.map((img, i) => (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  key={i}
                  src={imagePartToDataUrl(img)}
                  alt=""
                  className="size-20 rounded-lg border border-srapi-border object-cover"
                />
              ))}
            </div>
          ) : null}
          {message.files?.length ? (
            <div className="flex flex-wrap justify-end gap-1.5">
              {message.files.map((file, i) => (
                <span
                  key={i}
                  title={file.name}
                  className="flex max-w-52 items-center gap-1.5 rounded-lg border border-srapi-border bg-srapi-card-muted px-2 py-1 text-2xs text-srapi-text-secondary"
                >
                  <FileText className="size-3.5 shrink-0 text-srapi-text-tertiary" />
                  <span className="min-w-0 truncate">{file.name}</span>
                </span>
              ))}
            </div>
          ) : null}
          {message.content ? (
            <div className="rounded-2xl rounded-tr-sm bg-srapi-invert px-3.5 py-2 text-sm text-srapi-invert-fg">
              {message.content}
            </div>
          ) : null}
        </div>
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-srapi-card-muted">
          <UserIcon className="size-4 text-srapi-text-tertiary" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex gap-2">
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-srapi-primary/10">
        <Bot className="size-4 text-srapi-primary" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        {message.reasoning ? <ReasoningBlock text={message.reasoning} /> : null}
        {message.content ? <Markdown>{message.content}</Markdown> : null}
        {(message.tool_calls ?? []).map((call) => (
          <ToolCallCard key={call.id} call={call} result={resultsById.get(call.id)} />
        ))}
      </div>
    </div>
  );
}

function ToolCallCard({
  call,
  result,
}: {
  call: { id: string; name: string; arguments: string };
  result?: { content: string; is_error?: boolean };
}) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const { method, path } = describeCall(call);

  return (
    <div className="overflow-hidden rounded-xl border border-srapi-border bg-srapi-card-muted/40 text-xs">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left transition-colors hover:bg-srapi-card-muted"
      >
        <Wrench className="size-3.5 shrink-0 text-srapi-text-tertiary" />
        {method ? (
          <Badge variant="neutral" className="font-mono text-2xs">
            {method}
          </Badge>
        ) : null}
        <span className="min-w-0 flex-1 truncate font-mono text-srapi-text-secondary">{path || call.name}</span>
        {result ? (
          result.is_error ? (
            <Badge variant="danger" className="text-2xs">
              {t("copilot.failed")}
            </Badge>
          ) : (
            <Badge variant="success" className="text-2xs">
              {t("copilot.ok")}
            </Badge>
          )
        ) : (
          <Loader2 className="size-3.5 animate-spin text-srapi-text-tertiary" />
        )}
        <ChevronDown className={`size-3.5 shrink-0 text-srapi-text-tertiary transition-transform ${open ? "rotate-180" : ""}`} />
      </button>
      {open ? (
        <div className="space-y-2 border-t border-srapi-border px-3 py-2">
          <div>
            <div className="mb-1 text-2xs uppercase tracking-wide text-srapi-text-tertiary">{t("copilot.arguments")}</div>
            <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-2xs text-srapi-text-secondary">
              {prettyJSON(call.arguments)}
            </pre>
          </div>
          {result ? (
            <div>
              <div className="mb-1 text-2xs uppercase tracking-wide text-srapi-text-tertiary">{t("copilot.result")}</div>
              <ToolResultView content={result.content} />
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** Renders a tool result: a compact table for `{data:[…]}` list responses, else
 * pretty JSON, else raw text. */
function ToolResultView({ content }: { content: string }) {
  const parsed = useMemo(() => parseResult(content), [content]);
  if (parsed.rows) {
    return (
      <div className="overflow-x-auto rounded-lg border border-srapi-border">
        <table className="w-full text-2xs">
          <thead>
            <tr className="border-b border-srapi-border bg-srapi-card-muted/50 text-left text-srapi-text-tertiary">
              {parsed.columns.map((c) => (
                <th key={c} className="px-2 py-1 font-medium">
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {parsed.rows.map((row, i) => (
              <tr key={i} className="border-b border-srapi-border/60 last:border-0">
                {parsed.columns.map((c) => (
                  <td key={c} className="max-w-[12rem] truncate px-2 py-1 font-mono text-srapi-text-secondary">
                    {row[c]}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        {parsed.more ? <div className="px-2 py-1 text-2xs text-srapi-text-tertiary">+{parsed.more}…</div> : null}
      </div>
    );
  }
  return (
    <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-2xs text-srapi-text-secondary">
      {parsed.text}
    </pre>
  );
}

function PendingActionBanner({
  action,
  onResolve,
  disabled,
}: {
  action: PendingAction;
  onResolve: (approved: boolean) => void;
  disabled: boolean;
}) {
  const { t } = useLanguage();
  const [confirmText, setConfirmText] = useState("");
  const danger = !!action.danger;
  const confirmed = !danger || confirmText.trim() === action.path;
  return (
    <div
      className={`ml-9 rounded-2xl border p-3 ${
        danger ? "border-srapi-error/40 bg-srapi-error/5" : "border-srapi-primary/40 bg-srapi-primary/5"
      }`}
    >
      <div className="flex items-start gap-2">
        <AlertTriangle className={`mt-0.5 size-4 shrink-0 ${danger ? "text-srapi-error" : "text-srapi-primary"}`} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium text-srapi-text-primary">
            {danger ? t("copilot.dangerTitle") : t("copilot.approvalTitle")}
          </div>
          {action.summary ? <div className="mt-0.5 text-xs text-srapi-text-secondary">{action.summary}</div> : null}
          <div className="mt-2 flex items-center gap-2 font-mono text-xs">
            <Badge variant={danger ? "danger" : "neutral"}>{action.method}</Badge>
            <span className="min-w-0 truncate text-srapi-text-secondary">{action.path}</span>
          </div>
          {action.body ? (
            <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-md bg-srapi-bg/60 p-2 font-mono text-2xs text-srapi-text-secondary">
              {prettyJSON(action.body)}
            </pre>
          ) : null}
          {danger ? (
            <div className="mt-2">
              <p className="mb-1 text-2xs text-srapi-error">{t("copilot.dangerConfirmHint")}</p>
              <input
                value={confirmText}
                onChange={(e) => setConfirmText(e.target.value)}
                placeholder={action.path}
                spellCheck={false}
                className="w-full rounded-md border border-srapi-error/40 bg-srapi-bg px-2 py-1.5 font-mono text-xs text-srapi-text-primary outline-none focus:border-srapi-error"
              />
            </div>
          ) : null}
        </div>
      </div>
      <div className="mt-3 flex justify-end gap-2">
        <Button variant="outline" size="sm" onClick={() => onResolve(false)} disabled={disabled}>
          <X className="size-4" />
          {t("copilot.deny")}
        </Button>
        <Button
          variant={danger ? "danger" : "primary"}
          size="sm"
          onClick={() => onResolve(true)}
          disabled={disabled || !confirmed}
        >
          <Check className="size-4" />
          {t("copilot.approve")}
        </Button>
      </div>
    </div>
  );
}

function describeCall(call: { name: string; arguments: string }): { method: string; path: string } {
  if (call.name !== "call_admin_api") return { method: "", path: call.name };
  try {
    const args = JSON.parse(call.arguments || "{}") as { method?: string; path?: string };
    return { method: (args.method ?? "").toUpperCase(), path: args.path ?? "" };
  } catch {
    return { method: "", path: call.name };
  }
}

function prettyJSON(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

interface ParsedResult {
  text: string;
  columns: string[];
  rows: Array<Record<string, string>> | null;
  more: number;
}

/** Parse a "HTTP <status>\n<json>" tool result into a table when the body is a
 * `{data:[…objects]}` list, else fall back to pretty text. */
function parseResult(content: string): ParsedResult {
  let body = content;
  const nl = content.indexOf("\n");
  if (content.startsWith("HTTP ") && nl >= 0) body = content.slice(nl + 1);
  try {
    const json = JSON.parse(body) as unknown;
    const data = (json as { data?: unknown })?.data;
    if (Array.isArray(data) && data.length > 0 && typeof data[0] === "object" && data[0] !== null) {
      const columns = Object.keys(data[0] as Record<string, unknown>).slice(0, 5);
      const max = 8;
      const rows = data.slice(0, max).map((item) => {
        const out: Record<string, string> = {};
        for (const c of columns) {
          const v = (item as Record<string, unknown>)[c];
          out[c] = v == null ? "" : typeof v === "object" ? JSON.stringify(v) : String(v);
        }
        return out;
      });
      return { text: content, columns, rows, more: Math.max(0, data.length - max) };
    }
    return { text: JSON.stringify(json, null, 2), columns: [], rows: null, more: 0 };
  } catch {
    return { text: content, columns: [], rows: null, more: 0 };
  }
}
