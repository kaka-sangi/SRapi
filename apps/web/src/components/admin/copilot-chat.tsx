"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
  Copy,
  RefreshCw,
  MessageSquare,
  Pencil,
  Trash2,
  Zap,
} from "lucide-react";
import { FileText } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Markdown } from "@/components/ui/markdown";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { useCopilotSession, type PendingAction } from "@/context/CopilotSessionContext";
import { Composer } from "@/components/chat/composer";
import { ReasoningBlock } from "@/components/chat/reasoning-block";
import {
  listCopilotConversations,
  renameCopilotConversation,
  deleteCopilotConversation,
  type CopilotMessage,
  type CopilotImage,
  type CopilotConversationSummary,
} from "@/lib/copilot-client";
import {
  fileToImagePart,
  imagePartToDataUrl,
  fileToTextPart,
  isImageFile,
  isTextFile,
  type CopilotFilePart,
} from "@/lib/image-utils";

/** Derives a stable React list key for a copilot message. The store (and the
 * `CopilotMessage` type) lives outside this component, so we can't stamp an id
 * at construction — instead we key off server-assigned ids that already exist
 * on the messages that actually move under regenerate/slice: tool calls and
 * tool results carry a unique `tool_call_id`. Plain user/assistant-text turns
 * have no inherent id; they form the retained prefix on regenerate, so a
 * role+position key is stable for them. Strictly better than a bare array
 * index, which remaps tool messages onto unrelated content. */
function messageKey(message: CopilotMessage, index: number): string {
  if (message.tool_calls?.length) return `call:${message.tool_calls[0].id}`;
  if (message.tool_results?.length) return `result:${message.tool_results[0].tool_call_id}`;
  return `${message.role}:${index}`;
}

export function CopilotChat({ models, defaultModel }: { models: string[]; defaultModel: string }) {
  const session = useCopilotSession();
  const {
    messages,
    running,
    pending,
    error,
    usage,
    model,
    effort,
    autoApprove,
    setModel,
    setEffort,
    setAutoApprove,
    send,
    resolvePending,
    stop,
    regenerate,
  } = session;
  const { t } = useLanguage();
  const [input, setInput] = useState("");
  const [images, setImages] = useState<CopilotImage[]>([]);
  const [files, setFiles] = useState<CopilotFilePart[]>([]);
  const { toast } = useToast();
  const endRef = useRef<HTMLDivElement | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  // Seed the per-turn model from the configured default. Re-seed when the
  // current model is empty OR no longer offered by the config (e.g. the admin
  // changed the copilot's model) so the picker never sticks on a stale value
  // that the dropdown doesn't even list. A still-valid manual pick is kept.
  useEffect(() => {
    if (!model || (models.length > 0 && !models.includes(model))) {
      setModel(defaultModel || models[0] || "");
    }
  }, [model, defaultModel, models, setModel]);

  const resultsById = useMemo(() => {
    const map = new Map<string, { content: string; is_error?: boolean }>();
    for (const m of messages) {
      if (m.role === "tool" && m.tool_results) {
        for (const r of m.tool_results) map.set(r.tool_call_id, { content: r.content, is_error: r.is_error });
      }
    }
    return map;
  }, [messages]);

  const lastAssistantIdx = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === "assistant") return i;
    }
    return -1;
  }, [messages]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: running ? "instant" : "smooth", block: "end" });
  }, [messages, pending, running]);

  function doSend(text?: string) {
    const content = (text ?? input).trim();
    if ((!content && images.length === 0 && files.length === 0) || running) return;
    send(content, images, files);
    setInput("");
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
      toast({ title: t("copilot.fileUnsupported", { name: rejected[0].name }), tone: "error" });
    }
    if (fileRef.current) fileRef.current.value = "";
  }

  const empty = messages.length === 0 && !running;

  return (
    <div className="flex h-full min-h-0 gap-4">
      <ConversationSidebar />

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex-1 overflow-y-auto px-1 pb-4">
          {empty ? (
            <EmptyState />
          ) : (
            <div className="mx-auto max-w-3xl space-y-5 py-4">
              {messages.map((message, i) => (
                <MessageRow
                  key={messageKey(message, i)}
                  message={message}
                  resultsById={resultsById}
                  isLast={i === lastAssistantIdx}
                  running={running}
                  onRegenerate={regenerate}
                />
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
                  <span className="flex-1">{error}</span>
                  <button
                    type="button"
                    onClick={regenerate}
                    disabled={running}
                    className="flex shrink-0 items-center gap-1 text-xs font-medium transition-colors hover:text-srapi-error/80 disabled:opacity-50"
                  >
                    <RefreshCw className="size-3.5" />
                    {t("copilot.retry")}
                  </button>
                </div>
              ) : null}
              {usage ? (
                <div className="pl-9 text-2xs text-srapi-text-tertiary">
                  {t("copilot.usageTokens", { input: usage.input, output: usage.output })}
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
            onSend={() => doSend()}
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
            extraControls={
              <Button
                type="button"
                variant={autoApprove ? "primary" : "ghost"}
                size="sm"
                className="h-8 shrink-0 gap-1 px-2"
                onClick={() => setAutoApprove(!autoApprove)}
                aria-pressed={autoApprove}
                title={t("copilot.yoloHint")}
              >
                <Zap className="size-3.5" />
                <span className="hidden text-2xs sm:inline">{t("copilot.yolo")}</span>
              </Button>
            }
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
    </div>
  );
}

/** Left rail listing the admin's saved conversations, with reopen / rename /
 * delete. Backed by the DB so conversations survive reloads and sessions. */
function ConversationSidebar() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const { activeId, loadConversation, newConversation, setActiveTitle, onActiveDeleted } = useCopilotSession();
  const [editing, setEditing] = useState<{ id: number; value: string } | null>(null);

  const list = useQuery({
    queryKey: ["copilot-conversations"],
    queryFn: listCopilotConversations,
  });

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ["copilot-conversations"] });

  async function commitRename(item: CopilotConversationSummary) {
    if (!editing) return;
    const value = editing.value.trim();
    setEditing(null);
    if (!value || value === item.title) return;
    try {
      await renameCopilotConversation(item.id, value);
      setActiveTitle(item.id, value);
      invalidate();
    } catch {
      toast({ title: t("copilot.saveFailed"), tone: "error" });
    }
  }

  async function remove(item: CopilotConversationSummary) {
    if (!window.confirm(t("copilot.deleteConfirm", { title: item.title }))) return;
    try {
      await deleteCopilotConversation(item.id);
      onActiveDeleted(item.id);
      invalidate();
    } catch {
      toast({ title: t("copilot.saveFailed"), tone: "error" });
    }
  }

  const items = list.data ?? [];

  return (
    <div className="hidden w-60 shrink-0 flex-col rounded-2xl border border-srapi-border bg-srapi-card/40 md:flex">
      <div className="p-2">
        <Button variant="outline" size="sm" className="w-full justify-start" onClick={newConversation}>
          <Plus className="size-4" />
          {t("copilot.newChat")}
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
        {list.isPending ? (
          <div className="flex items-center justify-center px-2 py-6">
            <Loader2 className="size-4 animate-spin text-srapi-text-tertiary" />
          </div>
        ) : items.length === 0 ? (
          <p className="px-2 py-6 text-center text-2xs text-srapi-text-tertiary">{t("copilot.noConversations")}</p>
        ) : (
          <ul className="space-y-0.5">
            {items.map((item) => (
              <li key={item.id} className="group relative">
                {editing?.id === item.id ? (
                  <input
                    autoFocus
                    value={editing.value}
                    onChange={(e) => setEditing({ id: item.id, value: e.target.value })}
                    onBlur={() => void commitRename(item)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void commitRename(item);
                      if (e.key === "Escape") setEditing(null);
                    }}
                    className="w-full rounded-md border border-srapi-primary/40 bg-srapi-bg px-2 py-1.5 text-xs text-srapi-text-primary outline-none"
                  />
                ) : (
                  <button
                    type="button"
                    onClick={() => void loadConversation(item.id)}
                    className={`flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs transition-colors ${
                      activeId === item.id
                        ? "bg-srapi-card-muted text-srapi-text-primary"
                        : "text-srapi-text-secondary hover:bg-srapi-card-muted/60"
                    }`}
                  >
                    <MessageSquare className="size-3.5 shrink-0 text-srapi-text-tertiary" />
                    <span className="min-w-0 flex-1 truncate">{item.title || t("copilot.untitled")}</span>
                  </button>
                )}
                {editing?.id !== item.id ? (
                  <div className="absolute right-1 top-1/2 hidden -translate-y-1/2 items-center gap-0.5 group-hover:flex">
                    <button
                      type="button"
                      aria-label={t("copilot.rename")}
                      onClick={() => setEditing({ id: item.id, value: item.title })}
                      className="rounded p-1 text-srapi-text-tertiary hover:bg-srapi-card hover:text-srapi-text-primary"
                    >
                      <Pencil className="size-3" />
                    </button>
                    <button
                      type="button"
                      aria-label={t("copilot.delete")}
                      onClick={() => void remove(item)}
                      className="rounded p-1 text-srapi-text-tertiary hover:bg-srapi-card hover:text-srapi-error"
                    >
                      <Trash2 className="size-3" />
                    </button>
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function EmptyState() {
  const { t } = useLanguage();
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-4 text-center">
      <div className="flex size-14 items-center justify-center rounded-2xl bg-srapi-primary/10">
        <Bot className="size-7 text-srapi-primary" />
      </div>
      <h2 className="font-serif text-2xl text-srapi-text-primary">{t("copilot.greeting")}</h2>
    </div>
  );
}

function MessageRow({
  message,
  resultsById,
  isLast,
  running,
  onRegenerate,
}: {
  message: CopilotMessage;
  resultsById: Map<string, { content: string; is_error?: boolean }>;
  isLast?: boolean;
  running?: boolean;
  onRegenerate?: () => void;
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
        {message.content ? (
          <MessageActions
            text={message.content}
            canRegenerate={!!isLast && !running && !!onRegenerate}
            onRegenerate={onRegenerate}
          />
        ) : null}
      </div>
    </div>
  );
}

/** Copy / regenerate actions shown under an assistant message. */
function MessageActions({
  text,
  canRegenerate,
  onRegenerate,
}: {
  text: string;
  canRegenerate?: boolean;
  onRegenerate?: () => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const [copied, setCopied] = useState(false);
  const copy = () => {
    void navigator.clipboard?.writeText(text).then(() => {
      setCopied(true);
      toast({ title: t("copilot.copied") });
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <div className="flex items-center gap-3 pt-0.5 text-srapi-text-tertiary">
      <button
        type="button"
        onClick={copy}
        className="flex items-center gap-1 text-2xs transition-colors hover:text-srapi-text-secondary"
      >
        {copied ? <Check className="size-3" /> : <Copy className="size-3" />}
        {t("copilot.copy")}
      </button>
      {canRegenerate && onRegenerate ? (
        <button
          type="button"
          onClick={onRegenerate}
          className="flex items-center gap-1 text-2xs transition-colors hover:text-srapi-text-secondary"
        >
          <RefreshCw className="size-3" />
          {t("copilot.regenerate")}
        </button>
      ) : null}
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
          <span className="flex shrink-0 items-center gap-1 text-2xs text-srapi-text-tertiary">
            <Loader2 className="size-3.5 animate-spin" />
            {t("copilot.toolRunning")}
          </span>
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
              <div className="mb-1 text-2xs uppercase tracking-wide text-srapi-text-tertiary">
                {call.name === "web_search" ? t("copilot.sources") : t("copilot.result")}
              </div>
              {call.name === "web_search" ? (
                <SearchResults content={result.content} />
              ) : (
                <ToolResultView content={result.content} />
              )}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** Renders web_search results as a clickable sources list (title → url +
 * snippet). Falls back to raw text for error messages. */
function SearchResults({ content }: { content: string }) {
  const { t } = useLanguage();
  const items = useMemo(() => {
    try {
      const parsed = JSON.parse(content) as {
        data?: Array<{ title?: string; url?: string; snippet?: string }>;
      };
      return Array.isArray(parsed.data) ? parsed.data : null;
    } catch {
      return null;
    }
  }, [content]);

  if (items === null) {
    return (
      <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words font-mono text-2xs text-srapi-text-secondary">
        {content}
      </pre>
    );
  }
  if (items.length === 0) {
    return <p className="text-2xs text-srapi-text-tertiary">{t("adminCommon.noResults")}</p>;
  }
  return (
    <ul className="space-y-2">
      {items.map((it, i) => (
        <li key={i} className="rounded-lg border border-srapi-border bg-srapi-card-muted/30 p-2">
          <a
            href={it.url}
            target="_blank"
            rel="noreferrer"
            className="text-xs font-medium text-srapi-primary underline underline-offset-2"
          >
            {it.title || it.url}
          </a>
          {it.url ? <div className="truncate text-2xs text-srapi-text-tertiary">{it.url}</div> : null}
          {it.snippet ? <p className="mt-1 text-2xs text-srapi-text-secondary">{it.snippet}</p> : null}
        </li>
      ))}
    </ul>
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
  const danger = !!action.danger;
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
            <p className="mt-2 text-2xs text-srapi-error">{t("copilot.dangerConfirmHint")}</p>
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
          disabled={disabled}
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
