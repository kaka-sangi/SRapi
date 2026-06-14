"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  Bot,
  User as UserIcon,
  Plus,
  AlertTriangle,
  Loader2,
  Zap,
  Hash,
  Cpu,
  Copy,
  Check,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Markdown } from "@/components/ui/markdown";
import { Composer } from "@/components/chat/composer";
import { ReasoningBlock } from "@/components/chat/reasoning-block";
import type { ReasoningEffort } from "@/components/chat/types";
import { useLanguage } from "@/context/LanguageContext";
import {
  streamPlaygroundChat,
  type PlaygroundMessage,
  type PlaygroundTurnMeta,
} from "@/lib/playground-client";
import { fileToImagePart, imagePartToDataUrl, type CopilotImagePart } from "@/lib/image-utils";
import {
  PlaygroundSettings,
  DEFAULT_PLAYGROUND_PARAMS,
  type PlaygroundParams,
} from "@/components/playground/playground-settings";
import { ViewAsCodeDialog } from "@/components/playground/view-as-code-dialog";
import type { CSSProperties } from "react";

const rise = (i: number) => ({ "--stagger-index": i }) as CSSProperties;

const SESSION_STORAGE_KEY = "srapi_playground_session_v1";

/** A playground message plus the per-turn gateway telemetry we attach to
 * assistant replies. `meta` persists in localStorage alongside the text. */
type ChatMessage = PlaygroundMessage & { meta?: PlaygroundTurnMeta };

interface PersistedSession {
  messages: ChatMessage[];
  model?: string;
  effort?: ReasoningEffort;
  params?: PlaygroundParams;
}

function readPersistedSession(): PersistedSession | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(SESSION_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as PersistedSession;
    if (!Array.isArray(parsed.messages)) return null;
    return parsed;
  } catch {
    return null;
  }
}

function writePersistedSession(session: PersistedSession) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session));
  } catch {
    // Quota exceeded (likely image attachments) — retry without image payloads
    // so the text history still survives a reload.
    try {
      window.localStorage.setItem(
        SESSION_STORAGE_KEY,
        JSON.stringify({
          ...session,
          messages: session.messages.map(({ images: _images, ...rest }) => rest),
        }),
      );
    } catch {
      // Storage unavailable — persistence is best-effort.
    }
  }
}


/** The 交界地 chat: a billed, session-authenticated user chat. Same surface as
 * the admin copilot minus all agentic/admin capability — it can only talk. */
export function PlaygroundChat({ models, defaultModel }: { models: string[]; defaultModel: string }) {
  const { t } = useLanguage();
  // Lazy initializers restore the previous session (messages, model, params)
  // so a reload never loses the conversation. The component only mounts
  // client-side (behind AuthGate), so reading localStorage here is safe.
  const [messages, setMessages] = useState<ChatMessage[]>(
    () => readPersistedSession()?.messages ?? [],
  );
  const [input, setInput] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [model, setModel] = useState(() => {
    const persisted = readPersistedSession()?.model;
    if (persisted && (models.length === 0 || models.includes(persisted))) return persisted;
    return defaultModel || models[0] || "";
  });
  const [effort, setEffort] = useState<ReasoningEffort>(
    () => readPersistedSession()?.effort ?? "off",
  );
  const [params, setParams] = useState<PlaygroundParams>(
    () => readPersistedSession()?.params ?? DEFAULT_PLAYGROUND_PARAMS,
  );
  const [images, setImages] = useState<CopilotImagePart[]>([]);
  const abortRef = useRef<AbortController | null>(null);
  const endRef = useRef<HTMLDivElement | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages, running]);

  // Re-validate the selected model against the available list (the React-blessed
  // "adjust state during render" pattern): when the list finishes loading or a
  // model is revoked, snap an out-of-list pick back to the default so the
  // composer can't sit on a model the user can't actually use. The guard makes
  // it converge in one extra render.
  if (model && models.length > 0 && !models.includes(model)) {
    setModel(defaultModel && models.includes(defaultModel) ? defaultModel : models[0]);
  }

  // Persist after a turn settles (not on every streaming delta) and whenever
  // the picker/params change, so a reload restores the latest state. An empty
  // conversation still persists model/effort/params.
  useEffect(() => {
    if (running) return;
    writePersistedSession({ messages, model, effort, params });
  }, [messages, running, model, effort, params]);

  const runTurn = useCallback(
    async (history: ChatMessage[]) => {
      setRunning(true);
      setError(null);
      const controller = new AbortController();
      abortRef.current = controller;
      const working: ChatMessage[] = [...history];
      let assistantIdx = -1;
      const ensureAssistant = () => {
        if (assistantIdx < 0) {
          working.push({ role: "assistant", content: "", reasoning: "" });
          assistantIdx = working.length - 1;
        }
      };
      const temperature = Number.parseFloat(params.temperature);
      const maxTokens = Number.parseInt(params.maxTokens, 10);
      try {
        await streamPlaygroundChat({
          messages: history,
          model,
          reasoningEffort: effort,
          system: params.system.trim() || undefined,
          temperature: Number.isFinite(temperature) ? temperature : undefined,
          maxTokens: Number.isFinite(maxTokens) && maxTokens > 0 ? maxTokens : undefined,
          signal: controller.signal,
          onDelta: (kind, text) => {
            if (!text) return;
            ensureAssistant();
            working[assistantIdx] =
              kind === "reasoning"
                ? { ...working[assistantIdx], reasoning: (working[assistantIdx].reasoning ?? "") + text }
                : { ...working[assistantIdx], content: (working[assistantIdx].content ?? "") + text };
            setMessages([...working]);
          },
          onMeta: (meta) => {
            // Telemetry lands once the stream closes. Only attach it if the turn
            // actually produced an assistant message (it always should here).
            ensureAssistant();
            working[assistantIdx] = { ...working[assistantIdx], meta };
            setMessages([...working]);
          },
          onError: (msg) => setError(msg),
        });
      } catch (err) {
        if ((err as Error)?.name !== "AbortError") setError((err as Error)?.message ?? "Stream error");
      } finally {
        setRunning(false);
        abortRef.current = null;
      }
    },
    [model, effort, params],
  );

  function send(text?: string) {
    const content = (text ?? input).trim();
    if ((!content && images.length === 0) || running) return;
    const userMsg: PlaygroundMessage = { role: "user", content };
    if (images.length) userMsg.images = images;
    const next = [...messages, userMsg];
    setMessages(next);
    setInput("");
    setImages([]);
    void runTurn(next);
  }

  function stop() {
    abortRef.current?.abort();
  }

  function regenerate() {
    if (running) return;
    // Trim trailing assistant messages so the history ends at the last user
    // turn, then re-run from there. (Normally there's exactly one assistant
    // message to drop.)
    let cut = messages.length;
    while (cut > 0 && messages[cut - 1].role === "assistant") cut -= 1;
    if (cut === 0) return;
    const history = messages.slice(0, cut);
    setMessages(history);
    void runTurn(history);
  }

  function reset() {
    abortRef.current?.abort();
    setMessages([]);
    setError(null);
    setImages([]);
  }

  async function onPickFiles(files: FileList | null) {
    if (!files?.length) return;
    const parts = await Promise.all(Array.from(files).map(fileToImagePart));
    setImages((prev) => [...prev, ...parts]);
    if (fileRef.current) fileRef.current.value = "";
  }

  const empty = messages.length === 0 && !running;

  return (
    <div className="relative flex h-[calc(100vh-9rem)] min-h-[30rem] flex-col">
      {!empty ? (
        <div className="pointer-events-none absolute right-0 top-0 z-10">
          <Button variant="ghost" size="sm" className="pointer-events-auto" onClick={reset}>
            <Plus className="size-4" />
            {t("playground.newChat")}
          </Button>
        </div>
      ) : null}

      <div className="flex-1 overflow-y-auto px-1 pb-4">
        {empty ? (
          <EmptyState />
        ) : (
          <div className="mx-auto max-w-3xl space-y-5 py-4">
            {messages.map((message, i) => (
              <MessageRow
                key={i}
                message={message}
                isLastAssistant={message.role === "assistant" && i === messages.length - 1}
                running={running}
                onRegenerate={regenerate}
              />
            ))}
            {running ? (
              <div className="flex items-center gap-2 pl-9 text-sm text-srapi-text-tertiary">
                <Loader2 className="size-4 animate-spin" />
                {t("playground.thinking")}
              </div>
            ) : null}
            {error ? (
              <div className="ml-9 flex items-start gap-2 rounded-xl border border-srapi-error/30 bg-srapi-error/5 px-3 py-2 text-sm text-srapi-error">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                <span className="flex-1">{error}</span>
                <button
                  type="button"
                  onClick={regenerate}
                  disabled={running}
                  className="flex shrink-0 items-center gap-1 text-xs font-medium transition-colors hover:text-srapi-error/80 disabled:opacity-50"
                >
                  <RefreshCw className="size-3.5" />
                  {t("common.retry")}
                </button>
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
          onAttach={() => fileRef.current?.click()}
          placeholder={t("playground.placeholder")}
          extraControls={
            <>
              <PlaygroundSettings params={params} onChange={setParams} />
              <ViewAsCodeDialog model={model} params={params} messages={messages} />
            </>
          }
        />
        <input
          ref={fileRef}
          type="file"
          accept="image/*"
          multiple
          hidden
          onChange={(e) => void onPickFiles(e.currentTarget.files)}
        />
        <p className="mt-1.5 px-1 text-center text-2xs text-srapi-text-tertiary">{t("playground.billingHint")}</p>
      </div>
    </div>
  );
}

function EmptyState() {
  const { t } = useLanguage();
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-4 text-center">
      <div
        className="anim-rise flex size-14 items-center justify-center rounded-2xl bg-srapi-primary/10"
        style={rise(0)}
      >
        <Bot className="size-7 text-srapi-primary" />
      </div>
      <h2 className="anim-rise font-serif text-2xl text-srapi-text-primary" style={rise(1)}>
        {t("playground.greeting")}
      </h2>
    </div>
  );
}

function MessageRow({
  message,
  isLastAssistant,
  running,
  onRegenerate,
}: {
  message: ChatMessage;
  isLastAssistant: boolean;
  running: boolean;
  onRegenerate: () => void;
}) {
  if (message.role === "user") {
    return (
      <div className="anim-rise-sm flex justify-end gap-2">
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
    <div className="anim-rise-sm group flex gap-2">
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-srapi-primary/10">
        <Bot className="size-4 text-srapi-primary" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        {message.reasoning ? <ReasoningBlock text={message.reasoning} /> : null}
        {message.content ? <Markdown>{message.content}</Markdown> : null}
        {message.content ? (
          <AssistantFooter
            meta={message.meta}
            content={message.content}
            isLastAssistant={isLastAssistant}
            running={running}
            onRegenerate={onRegenerate}
          />
        ) : null}
      </div>
    </div>
  );
}

/** Quiet gateway telemetry + copy/regenerate actions under an assistant reply.
 * The telemetry reads as a single muted mono line; the actions appear on hover. */
function AssistantFooter({
  meta,
  content,
  isLastAssistant,
  running,
  onRegenerate,
}: {
  meta?: PlaygroundTurnMeta;
  content: string;
  isLastAssistant: boolean;
  running: boolean;
  onRegenerate: () => void;
}) {
  const { t } = useLanguage();
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard unavailable (insecure context / denied permission) — ignore.
    }
  };

  const latencyLabel =
    meta?.latencyMs === undefined
      ? null
      : meta.latencyMs < 1000
        ? `${Math.round(meta.latencyMs)}ms`
        : `${(meta.latencyMs / 1000).toFixed(1)}s`;

  return (
    <div className="flex min-h-5 items-center gap-2.5 pt-0.5">
      {meta ? (
        <div className="flex items-center gap-1.5 font-mono text-2xs text-srapi-text-tertiary">
          {meta.servedModel ? (
            <span className="flex items-center gap-1">
              <Cpu className="size-3" />
              <span className="truncate">{meta.servedModel}</span>
            </span>
          ) : null}
          {meta.totalTokens !== undefined ? (
            <>
              {meta.servedModel ? <span aria-hidden>·</span> : null}
              <span className="flex items-center gap-1">
                <Hash className="size-3" />
                {meta.totalTokens.toLocaleString()} tok
              </span>
            </>
          ) : null}
          {latencyLabel ? (
            <>
              {meta.servedModel || meta.totalTokens !== undefined ? <span aria-hidden>·</span> : null}
              <span className="flex items-center gap-1">
                <Zap className="size-3" />
                {latencyLabel}
              </span>
            </>
          ) : null}
        </div>
      ) : null}

      <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
        <button
          type="button"
          onClick={() => void copy()}
          aria-label={copied ? t("playground.copied") : t("playground.copy")}
          title={copied ? t("playground.copied") : t("playground.copy")}
          className="rounded-md p-1 text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary"
        >
          {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
        </button>
        {isLastAssistant ? (
          <button
            type="button"
            onClick={onRegenerate}
            disabled={running}
            aria-label={t("playground.regenerate")}
            title={t("playground.regenerate")}
            className="rounded-md p-1 text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary disabled:cursor-not-allowed disabled:opacity-40"
          >
            <RefreshCw className="size-3.5" />
          </button>
        ) : null}
      </div>
    </div>
  );
}
