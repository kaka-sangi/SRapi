import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminOpsSystemLogsPage from "@/app/admin/ops/system-logs/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { OpsSystemLog, OpsSystemLogHealth } from "@/lib/sdk-types";

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
  log: {
    id: "sys-row",
    level: "error",
    source: "gateway",
    message: "gateway failed",
    request_id: "req-sys",
    trace_id: "trace-sys",
    metadata: {
      status_code: 502,
      error_class: "server_bad",
      provider_id: 9,
    },
    created_at: "2026-06-18T10:00:00Z",
  } satisfies OpsSystemLog,
  health: {
    storage_mode: "durable",
    writable: true,
    degraded: false,
    stale: false,
    total_count: 1,
    level_counts: { debug: 0, info: 0, warn: 0, error: 1 },
    last_log_at: "2026-06-18T10:00:00Z",
    last_error_at: "2026-06-18T10:00:00Z",
    last_error_source: "gateway",
    last_error_message: "gateway failed",
    checked_at: "2026-06-18T10:00:00Z",
  } satisfies OpsSystemLogHealth,
}));

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@/components/admin/ops-log-cleanup-dialog", () => ({
  OpsLogCleanupDialog: () => null,
}));

vi.mock("@/hooks/admin-queries", () => ({
  useOpsSystemLogs: () => ({
    data: {
      data: [mocks.log],
      pagination: { page: 1, page_size: 20, total: 1, has_next: false },
    },
    isFetching: false,
    refetch: vi.fn(),
  }),
  useOpsSystemLogHealth: () => ({
    data: mocks.health,
    isLoading: false,
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

describe("AdminOpsSystemLogsPage", () => {
  beforeEach(() => {
    storage.clear();
    window.history.replaceState(null, "", "/admin/ops/system-logs");
  });

  it("links each system log row to correlated error logs and request dumps", () => {
    render(<AdminOpsSystemLogsPage />, { wrapper: wrap });

    expect(screen.getAllByText("gateway failed").length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "错误日志" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=req-sys",
    );
    expect(screen.getByRole("link", { name: "请求转储" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-files&f_request_id=req-sys",
    );
  });
});
