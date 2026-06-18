import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminOutboxPage from "@/app/admin/ops/events/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { DomainEventOutbox } from "@/lib/sdk-types";

const storage = new Map<string, string>();
Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
});

const mocks = vi.hoisted(() => ({
  failed: {
    id: "evt-failed",
    event_id: "event-failed",
    event_type: "account.refresh.failed",
    event_version: "1",
    producer_module: "accounts",
    aggregate_type: "account",
    aggregate_id: "42",
    correlation_id: "corr-failed",
    causation_id: "cause-failed",
    idempotency_key: "idem-failed",
    payload: {},
    metadata: {},
    status: "failed",
    attempt_count: 3,
    last_error: "webhook endpoint returned 503",
    created_at: "2026-06-18T10:00:00Z",
  } satisfies DomainEventOutbox,
  pending: {
    id: "evt-pending",
    event_id: "event-pending",
    event_type: "account.refresh.requested",
    event_version: "1",
    producer_module: "accounts",
    aggregate_type: "account",
    aggregate_id: "43",
    correlation_id: "corr-pending",
    causation_id: "cause-pending",
    idempotency_key: "idem-pending",
    payload: {},
    metadata: {},
    status: "pending",
    attempt_count: 1,
    next_retry_at: "2026-06-18T10:05:00Z",
    created_at: "2026-06-18T10:00:00Z",
  } satisfies DomainEventOutbox,
}));

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@/hooks/admin-queries", () => ({
  useOutboxEvents: () => ({
    data: {
      data: [mocks.failed, mocks.pending],
      pagination: { page: 1, page_size: 20, total: 2, has_next: false },
    },
    isFetching: false,
  }),
}));

function wrap({ children }: PropsWithChildren) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LanguageProvider>{children}</LanguageProvider>
    </QueryClientProvider>
  );
}

describe("AdminOutboxPage", () => {
  beforeEach(() => {
    storage.clear();
    window.history.replaceState(null, "", "/admin/ops/events");
  });

  it("shows failed and pending delivery diagnostics in the list", () => {
    render(<AdminOutboxPage />, { wrapper: wrap });

    expect(screen.getByText("account.refresh.failed")).toBeInTheDocument();
    expect(screen.getByText("webhook endpoint returned 503")).toBeInTheDocument();
    expect(screen.getByText(/下次重试/)).toBeInTheDocument();
  });
});
