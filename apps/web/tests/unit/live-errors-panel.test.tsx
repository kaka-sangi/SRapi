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
        trace_id: "trace-live",
        provider_id: 9,
        provider_name: "provider-live",
        account_id: 42,
        account_name: "account-live",
        source_endpoint: "/v1/chat/completions",
        source_protocol: "openai-compatible",
        target_protocol: "anthropic-compatible",
        model: "canonical-live",
        upstream_model: "upstream-live",
        attempt_no: 3,
        upstream_request_id: "upstream-req-live",
        status_code: 502,
        error_class: "server_bad",
        error_phase: "upstream",
        error_owner: "provider",
        message: "live boom",
      });
    });

    expect(screen.getByText("live boom")).toBeInTheDocument();
    expect(screen.getByText("req-live")).toBeInTheDocument();
    expect(screen.getByText("provider-live #9")).toBeInTheDocument();
    expect(screen.getByText("account-live #42")).toBeInTheDocument();
    expect(screen.getByText("/v1/chat/completions")).toBeInTheDocument();
    expect(screen.getByText("openai-compatible -> anthropic-compatible")).toBeInTheDocument();
    expect(screen.getByText("upstream-live")).toBeInTheDocument();
    expect(screen.getByText("尝试 3 / upstream-req-live")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /错误日志/ })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=req-live",
    );
    expect(screen.getByRole("link", { name: /系统日志/ })).toHaveAttribute(
      "href",
      "/admin/ops/system-logs?f_request_id=req-live&f_trace_id=trace-live",
    );
    expect(screen.getByRole("link", { name: /请求转储/ })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-files&f_request_id=req-live",
    );
  });
});

function renderWithLanguage(children: React.ReactNode) {
  return render(<LanguageProvider>{children}</LanguageProvider>);
}
