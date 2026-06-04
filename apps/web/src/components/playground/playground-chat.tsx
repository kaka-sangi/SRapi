"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Bot, User as UserIcon, Plus, AlertTriangle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Markdown } from "@/components/ui/markdown";
import { Composer } from "@/components/chat/composer";
import { ReasoningBlock } from "@/components/chat/reasoning-block";
import type { ReasoningEffort } from "@/components/chat/types";
import { useLanguage } from "@/context/LanguageContext";
import { streamPlaygroundChat, type PlaygroundMessage } from "@/lib/playground-client";
import { fileToImagePart, imagePartToDataUrl, type CopilotImagePart } from "@/lib/image-utils";
import type { CSSProperties } from "react";

const rise = (i: number) => ({ "--stagger-index": i }) as CSSProperties;

/** The 交界地 chat: a billed, session-authenticated user chat. Same surface as
 * the admin copilot minus all agentic/admin capability — it can only talk. */
export function PlaygroundChat({ models, defaultModel }: { models: string[]; defaultModel: string }) {
  const { t } = useLanguage();
  const [messages, setMessages] = useState<PlaygroundMessage[]>([]);
  const [input, setInput] = useState("");
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [model, setModel] = useState(defaultModel || models[0] || "");
  const [effort, setEffort] = useState<ReasoningEffort>("off");
  const [images, setImages] = useState<CopilotImagePart[]>([]);
  const abortRef = useRef<AbortController | null>(null);
  const endRef = useRef<HTMLDivElement | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages, running]);

  const runTurn = useCallback(
    async (history: PlaygroundMessage[]) => {
      setRunning(true);
      setError(null);
      const controller = new AbortController();
      abortRef.current = controller;
      const working: PlaygroundMessage[] = [...history];
      let assistantIdx = -1;
      const ensureAssistant = () => {
        if (assistantIdx < 0) {
          working.push({ role: "assistant", content: "", reasoning: "" });
          assistantIdx = working.length - 1;
        }
      };
      try {
        await streamPlaygroundChat({
          messages: history,
          model,
          reasoningEffort: effort,
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
          onError: (msg) => setError(msg),
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
          <EmptyState onPick={(s) => send(s)} />
        ) : (
          <div className="mx-auto max-w-3xl space-y-5 py-4">
            {messages.map((message, i) => (
              <MessageRow key={i} message={message} />
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
          onAttach={() => fileRef.current?.click()}
          placeholder={t("playground.placeholder")}
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

function EmptyState({ onPick }: { onPick: (s: string) => void }) {
  const { t } = useLanguage();
  const examples = [t("playground.example1"), t("playground.example2"), t("playground.example3")];
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-4 text-center">
      <div
        className="anim-rise flex size-14 items-center justify-center rounded-2xl bg-srapi-primary/10"
        style={rise(0)}
      >
        <Bot className="size-7 text-srapi-primary" />
      </div>
      <div className="anim-rise space-y-1.5" style={rise(1)}>
        <h2 className="font-serif text-2xl text-srapi-text-primary">{t("playground.greeting")}</h2>
        <p className="mx-auto max-w-md text-sm text-srapi-text-secondary">{t("playground.emptyHint")}</p>
      </div>
      <div className="anim-rise flex flex-wrap justify-center gap-2" style={rise(2)}>
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

function MessageRow({ message }: { message: PlaygroundMessage }) {
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
    <div className="anim-rise-sm flex gap-2">
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-srapi-primary/10">
        <Bot className="size-4 text-srapi-primary" />
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        {message.reasoning ? <ReasoningBlock text={message.reasoning} /> : null}
        {message.content ? <Markdown>{message.content}</Markdown> : null}
      </div>
    </div>
  );
}
