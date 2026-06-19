import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ErrorLogsPanel } from "@/app/admin/logs/_panels/error-logs-panel";
import { LanguageProvider } from "@/context/LanguageContext";
import type { OpsErrorLog } from "@/lib/sdk-types";

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
  useAdminErrorLogs: vi.fn(),
  log: {
    id: "err-row",
    occurred_at: "2026-06-18T10:00:00Z",
    created_at: "2026-06-18T10:00:00Z",
    updated_at: "2026-06-18T10:00:00Z",
    request_id: "req-row",
    trace_id: "trace-row",
    account_id: "12",
    provider_id: "3",
    user_id: "4",
    api_key_id: "5",
    source_protocol: "openai-compatible",
    target_protocol: "anthropic-compatible",
    source_endpoint: "/v1/chat/completions",
    model: "gpt-4o-mini",
    status_code: 503,
    latency_ms: 891,
    attempt_no: 1,
    error_class: "server_bad",
    error_phase: "upstream",
    error_owner: "provider",
    error_message: "upstream failed",
    resolution: "open",
  } satisfies OpsErrorLog,
}));

vi.mock("@/hooks/admin-queries", () => ({
  downloadAdminRequestLogFileText: vi.fn(),
  useAdminErrorLog: () => ({
    data: undefined,
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useAdminErrorLogs: mocks.useAdminErrorLogs,
  useAdminRequestLogFileDownload: () => ({
    data: undefined,
    isError: false,
  }),
  useAdminRequestLogFiles: () => ({
    data: { data: [], pagination: { page: 1, page_size: 3, total: 0, has_next: false } },
    isFetching: false,
  }),
  useAdminModels: () => ({
    data: { data: [{ canonical_name: "gpt-4o-mini" }] },
  }),
  useUpdateErrorLogResolution: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
}));

vi.mock("@/hooks/use-account-name-lookup", () => ({
  useAccountNameLookup: () => ({
    query: { data: { data: [{ id: "12", name: "account-12" }] } },
    get: (id?: string | null) => (id ? `account-${id}` : "—"),
  }),
}));
vi.mock("@/hooks/use-api-key-name-lookup", () => ({
  useApiKeyNameLookup: () => ({ get: (id?: string | null) => (id ? `key-${id}` : "—") }),
}));
vi.mock("@/hooks/use-provider-name-lookup", () => ({
  useProviderNameLookup: () => ({
    query: { data: { data: [{ id: "3", name: "provider-3", display_name: "Provider 3" }] } },
    get: (id?: string | null) => (id ? `provider-${id}` : "—"),
  }),
}));
vi.mock("@/hooks/use-user-email-lookup", () => ({
  useUserEmailLookup: () => ({
    query: { data: { data: [{ id: "4", email: "user-4@example.test" }] } },
    get: (id?: string | null) => (id ? `user-${id}@example.test` : "—"),
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

describe("ErrorLogsPanel", () => {
  beforeEach(() => {
    storage.clear();
    mocks.useAdminErrorLogs.mockReturnValue({
      data: {
        data: [mocks.log],
        pagination: { page: 1, page_size: 20, total: 1, has_next: false },
      },
      isFetching: false,
      refetch: vi.fn(),
    });
    window.history.replaceState(null, "", "/admin/logs?tab=error");
  });

  it("links each error row to correlated system logs and request dumps", () => {
    render(<ErrorLogsPanel />, { wrapper: wrap });

    expect(screen.getByText("server_bad")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /系统日志/ })).toHaveAttribute(
      "href",
      "/admin/ops/system-logs?f_request_id=req-row&f_trace_id=trace-row",
    );
    expect(screen.getByRole("link", { name: /全部转储/ })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-files&f_request_id=req-row",
    );
  });

  it("maps provider and error-class URL filters to exact backend query params", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/logs?tab=error&f_provider=3&f_error_class=server_bad",
    );

    render(<ErrorLogsPanel />, { wrapper: wrap });

    expect(mocks.useAdminErrorLogs).toHaveBeenCalledWith(
      expect.objectContaining({
        provider_id: "3",
        error_class: "server_bad",
      }),
    );
  });
});
