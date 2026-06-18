import { act, render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LiveErrorsPanel } from "@/app/admin/logs/_panels/live-errors-panel";
import { LanguageProvider } from "@/context/LanguageContext";

const eventSources: MockEventSource[] = [];

class MockEventSource {
  url: string;
  options?: EventSourceInit;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  private listeners = new Map<string, Array<(event: MessageEvent) => void>>();

  constructor(url: string, options?: EventSourceInit) {
    this.url = url;
    this.options = options;
    eventSources.push(this);
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    const fn =
      typeof listener === "function"
        ? (listener as (event: MessageEvent) => void)
        : ((event: MessageEvent) => listener.handleEvent(event)) as (event: MessageEvent) => void;
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
  }

  close() {}

  emit(type: string, data: unknown) {
    const event = new MessageEvent(type, { data: JSON.stringify(data) });
    for (const listener of this.listeners.get(type) ?? []) {
      listener(event);
    }
  }
}

Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: {
    getItem: () => null,
    setItem: vi.fn(),
    removeItem: vi.fn(),
    clear: vi.fn(),
  },
});

describe("LiveErrorsPanel", () => {
  const originalEventSource = globalThis.EventSource;

  afterEach(() => {
    eventSources.length = 0;
    vi.clearAllMocks();
    globalThis.EventSource = originalEventSource;
  });

  it("consumes gateway_error frames without treating connection errors as data", async () => {
    globalThis.EventSource = MockEventSource as unknown as typeof EventSource;
    renderWithLanguage(<LiveErrorsPanel />);

    const source = eventSources[0];
    expect(source.url).toBe("/api/v1/admin/error-stream");
    expect(source.options).toEqual({ withCredentials: true });

    act(() => {
      source.onerror?.();
    });
    expect(screen.queryByText("live boom")).not.toBeInTheDocument();

    act(() => {
      source.emit("gateway_error", {
        at_unix_ms: Date.UTC(2026, 5, 18, 10, 0),
        request_id: "req-live",
        status_code: 502,
        error_class: "server_bad",
        message: "live boom",
      });
    });

    expect(screen.getByText("live boom")).toBeInTheDocument();
    expect(screen.getByText("req-live")).toBeInTheDocument();
  });
});

function renderWithLanguage(children: React.ReactNode) {
  return render(<LanguageProvider>{children}</LanguageProvider>);
}
