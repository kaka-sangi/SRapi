"use client";

import { createContext, useContext, useRef, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  streamCopilotChat,
  createCopilotConversation,
  updateCopilotConversation,
  getCopilotConversation,
  type CopilotMessage,
  type CopilotEvent,
  type CopilotImage,
} from "@/lib/copilot-client";
import type { ReasoningEffort } from "@/components/chat/types";
import type { CopilotFilePart } from "@/lib/image-utils";

export interface PendingAction {
  tool_call_id: string;
  name: string;
  method: string;
  path: string;
  body?: string;
  summary?: string;
  danger?: boolean;
}

export interface TurnUsage {
  input: number;
  output: number;
}

interface CopilotSessionValue {
  messages: CopilotMessage[];
  running: boolean;
  pending: PendingAction | null;
  error: string | null;
  usage: TurnUsage | null;
  activeId: number | null;
  model: string;
  effort: ReasoningEffort;
  autoApprove: boolean;
  setModel: (m: string) => void;
  setEffort: (e: ReasoningEffort) => void;
  setAutoApprove: (v: boolean) => void;
  send: (text: string, images: CopilotImage[], files: CopilotFilePart[]) => void;
  resolvePending: (approved: boolean) => void;
  stop: () => void;
  newConversation: () => void;
  regenerate: () => void;
  loadConversation: (id: number) => Promise<void>;
  setActiveTitle: (id: number, title: string) => void;
  onActiveDeleted: (id: number) => void;
}

const CopilotSessionContext = createContext<CopilotSessionValue | null>(null);

// sessionStorage mirrors the active conversation so a hard reload restores it
// instantly (the live stream itself survives only client-side navigation, which
// keeps this provider mounted). Stored per-tab, cleared on tab close.
const STORAGE_KEY = "srapi.copilot.session";
// These two are per-operator preferences (not per-tab), so they live in
// localStorage and survive reloads / new tabs.
const EFFORT_KEY = "srapi.copilot.effort";
const YOLO_KEY = "srapi.copilot.yolo";

function loadStoredEffort(): ReasoningEffort {
  if (typeof window === "undefined") return "off";
  const raw = window.localStorage.getItem(EFFORT_KEY);
  return raw === "low" || raw === "medium" || raw === "high" || raw === "off" ? raw : "off";
}

function loadStoredYolo(): boolean {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(YOLO_KEY) === "1";
}

function loadStored(): { messages: CopilotMessage[]; activeId: number | null } {
  if (typeof window === "undefined") return { messages: [], activeId: null };
  try {
    const raw = window.sessionStorage.getItem(STORAGE_KEY);
    const parsed = raw ? (JSON.parse(raw) as { messages?: CopilotMessage[]; activeId?: number | null }) : null;
    return {
      messages: Array.isArray(parsed?.messages) ? parsed!.messages! : [],
      activeId: typeof parsed?.activeId === "number" ? parsed!.activeId! : null,
    };
  } catch {
    return { messages: [], activeId: null };
  }
}

export function CopilotSessionProvider({ children }: { children: ReactNode }) {
  const stored = loadStored();
  const queryClient = useQueryClient();
  const [messages, setMessages] = useState<CopilotMessage[]>(stored.messages);
  const [running, setRunning] = useState(false);
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [usage, setUsage] = useState<TurnUsage | null>(null);
  const [activeId, setActiveId] = useState<number | null>(stored.activeId);
  const [model, setModel] = useState("");
  const [effort, setEffortState] = useState<ReasoningEffort>(loadStoredEffort);
  const setEffort = (e: ReasoningEffort) => {
    setEffortState(e);
    try {
      window.localStorage.setItem(EFFORT_KEY, e);
    } catch {
      // localStorage unavailable/over quota — the in-session value still works.
    }
  };
  // Yolo / auto-approve: when on, tool calls that would normally pause for an
  // approval prompt are continued automatically. Persisted, with a ref so the
  // long-lived stream handler always reads the current value.
  const [autoApprove, setAutoApproveState] = useState<boolean>(loadStoredYolo);
  const autoApproveRef = useRef(autoApprove);
  const setAutoApprove = (v: boolean) => {
    autoApproveRef.current = v;
    setAutoApproveState(v);
    try {
      window.localStorage.setItem(YOLO_KEY, v ? "1" : "0");
    } catch {
      // localStorage unavailable/over quota — the in-session value still works.
    }
  };

  const abortRef = useRef<AbortController | null>(null);
  // Model + effort captured at the START of the active turn, so changing the
  // picker mid-turn (e.g. before approving a pending tool call) does not alter
  // the in-flight turn — resolvePending continues with the same settings.
  const activeTurnRef = useRef<{ model: string; effort: ReasoningEffort }>({ model: "", effort: "off" });
  const flushRef = useRef<number>(0);
  // Refs the streaming "done" handler reads (avoids stale closures).
  const activeIdRef = useRef<number | null>(stored.activeId);
  const titleRef = useRef<string>("");

  function persistSession(msgs: CopilotMessage[], id: number | null) {
    activeIdRef.current = id;
    setActiveId(id);
    setMessages(msgs);
    try {
      if (msgs.length === 0) window.sessionStorage.removeItem(STORAGE_KEY);
      else window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ messages: msgs, activeId: id }));
    } catch {
      // sessionStorage unavailable/over quota — in-memory state still works.
    }
  }

  async function saveToServer(msgs: CopilotMessage[]) {
    if (msgs.length === 0) return;
    try {
      if (activeIdRef.current == null) {
        const created = await createCopilotConversation("", msgs);
        activeIdRef.current = created.id;
        titleRef.current = created.title;
        setActiveId(created.id);
        try {
          window.sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ messages: msgs, activeId: created.id }));
        } catch {
          // ignore
        }
      } else {
        await updateCopilotConversation(activeIdRef.current, titleRef.current, msgs);
      }
      void queryClient.invalidateQueries({ queryKey: ["copilot-conversations"] });
    } catch {
      // Saving is best-effort: the chat keeps working even if persistence fails.
    }
  }

  async function runTurn(history: CopilotMessage[], approval?: { tool_call_id: string; approved: boolean }) {
    setRunning(true);
    setError(null);
    setPending(null);
    setUsage(null);
    const controller = new AbortController();
    abortRef.current = controller;
    const working: CopilotMessage[] = [...history];
    let assistantIdx = -1;
    let turnInput = 0;
    let turnOutput = 0;
    let receivedTerminal = false;

    const ensureAssistant = () => {
      if (assistantIdx < 0) {
        working.push({ role: "assistant", content: "", tool_calls: [] });
        assistantIdx = working.length - 1;
      }
    };

    const scheduleFlush = () => {
      if (flushRef.current) return;
      flushRef.current = requestAnimationFrame(() => {
        flushRef.current = 0;
        setMessages([...working]);
      });
    };

    const cancelFlush = () => {
      if (flushRef.current) {
        cancelAnimationFrame(flushRef.current);
        flushRef.current = 0;
      }
    };

    const onEvent = (event: CopilotEvent) => {
      switch (event.type) {
        case "usage":
          turnInput += event.data.input_tokens;
          turnOutput += event.data.output_tokens;
          setUsage({ input: turnInput, output: turnOutput });
          break;
        case "assistant_reasoning":
          ensureAssistant();
          working[assistantIdx] = {
            ...working[assistantIdx],
            reasoning: (working[assistantIdx].reasoning ?? "") + event.data.text,
          };
          scheduleFlush();
          break;
        case "assistant_delta":
          ensureAssistant();
          working[assistantIdx] = {
            ...working[assistantIdx],
            content: (working[assistantIdx].content ?? "") + event.data.text,
          };
          scheduleFlush();
          break;
        case "tool_call": {
          ensureAssistant();
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
        case "tool_result":
          working.push({
            role: "tool",
            tool_results: [
              { tool_call_id: event.data.tool_call_id, content: event.data.content, is_error: event.data.is_error },
            ],
          });
          setMessages([...working]);
          break;
        case "pending_action":
          receivedTerminal = true;
          cancelFlush();
          persistSession([...working], activeIdRef.current);
          if (autoApproveRef.current) {
            // Yolo mode: skip the approval prompt and continue immediately.
            void runTurn([...working], { tool_call_id: event.data.tool_call_id, approved: true });
          } else {
            setPending(event.data);
          }
          break;
        case "done":
          receivedTerminal = true;
          cancelFlush();
          persistSession(event.data.messages, activeIdRef.current);
          setPending(null);
          void saveToServer(event.data.messages);
          break;
        case "error":
          receivedTerminal = true;
          cancelFlush();
          setError(event.data.message);
          break;
      }
    };

    try {
      await streamCopilotChat({
        messages: history,
        approval,
        model: activeTurnRef.current.model || undefined,
        reasoningEffort: activeTurnRef.current.effort,
        signal: controller.signal,
        onEvent,
      });
      if (!receivedTerminal) {
        setError("Connection lost — the response may be incomplete");
        if (working.length > history.length) persistSession([...working], activeIdRef.current);
      }
    } catch (err) {
      if ((err as Error)?.name !== "AbortError") setError((err as Error)?.message ?? "Stream error");
    } finally {
      cancelFlush();
      setRunning(false);
      abortRef.current = null;
    }
  }

  function send(text: string, images: CopilotImage[], files: CopilotFilePart[]) {
    const content = text.trim();
    if ((!content && images.length === 0 && files.length === 0) || running) return;
    const userMsg: CopilotMessage = { role: "user", content };
    if (images.length) userMsg.images = images;
    if (files.length) userMsg.files = files;
    const next = [...messages, userMsg];
    persistSession(next, activeIdRef.current);
    activeTurnRef.current = { model, effort };
    void runTurn(next);
  }

  function resolvePending(approved: boolean) {
    if (!pending) return;
    void runTurn(messages, { tool_call_id: pending.tool_call_id, approved });
  }

  function regenerate() {
    if (running) return;
    let lastUser = -1;
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role === "user") {
        lastUser = i;
        break;
      }
    }
    if (lastUser < 0) return;
    const history = messages.slice(0, lastUser + 1);
    persistSession(history, activeIdRef.current);
    activeTurnRef.current = { model, effort };
    void runTurn(history);
  }

  function stop() {
    abortRef.current?.abort();
  }

  function newConversation() {
    abortRef.current?.abort();
    titleRef.current = "";
    persistSession([], null);
    setPending(null);
    setError(null);
    setUsage(null);
  }

  async function loadConversation(id: number) {
    abortRef.current?.abort();
    setPending(null);
    setError(null);
    setUsage(null);
    try {
      const conv = await getCopilotConversation(id);
      titleRef.current = conv.title;
      persistSession(conv.messages ?? [], conv.id);
    } catch (err) {
      // Clear the previously-loaded conversation so a failed switch doesn't leave
      // the old messages on screen, misattributed under the new error.
      persistSession([], null);
      setError((err as Error)?.message ?? "Failed to load conversation");
    }
  }

  function setActiveTitle(id: number, title: string) {
    if (activeIdRef.current === id) titleRef.current = title;
  }

  function onActiveDeleted(id: number) {
    if (activeIdRef.current === id) newConversation();
  }

  const value: CopilotSessionValue = {
    messages,
    running,
    pending,
    error,
    usage,
    activeId,
    model,
    effort,
    autoApprove,
    setModel,
    setEffort,
    setAutoApprove,
    send,
    resolvePending,
    stop,
    newConversation,
    regenerate,
    loadConversation,
    setActiveTitle,
    onActiveDeleted,
  };

  return <CopilotSessionContext.Provider value={value}>{children}</CopilotSessionContext.Provider>;
}

export function useCopilotSession(): CopilotSessionValue {
  const ctx = useContext(CopilotSessionContext);
  if (!ctx) throw new Error("useCopilotSession must be used within CopilotSessionProvider");
  return ctx;
}
