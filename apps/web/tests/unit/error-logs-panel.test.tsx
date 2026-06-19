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
  schedulerLog: {
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
    error_class: "no_available_account",
    error_phase: "routing",
    error_owner: "scheduler",
    error_source: "gateway",
    error_message:
      "no available account: 3 candidate(s) rejected [capability_mismatch:responses(2), cooldown_active(1)]",
    error_body_excerpt: JSON.stringify({
      response_status: 503,
      scheduler_decision_id: 77,
      scheduler_candidate_count: 3,
      scheduler_rejected_count: 3,
      scheduler_primary_reject_reason: "capability_mismatch:responses",
      scheduler_primary_reject_count: 2,
      scheduler_operator_action: "check_model_capabilities_or_mapping",
      scheduler_reject_reason_counts: {
        "capability_mismatch:responses": 2,
        cooldown_active: 1,
      },
      scheduler_selection_rationale: "no account satisfied responses capability",
    }),
    resolution: "open",
  } satisfies OpsErrorLog,
  upstreamLog: {
    id: "err-upstream",
    occurred_at: "2026-06-18T10:00:00Z",
    created_at: "2026-06-18T10:00:00Z",
    updated_at: "2026-06-18T10:00:00Z",
    request_id: "req-upstream",
    trace_id: "trace-upstream",
    account_id: "12",
    provider_id: "3",
    user_id: "4",
    api_key_id: "5",
    source_protocol: "openai-compatible",
    target_protocol: "openai-compatible",
    source_endpoint: "/v1/chat/completions",
    model: "gpt-4o-mini",
    status_code: 429,
    latency_ms: 201,
    attempt_no: 1,
    error_class: "rate_limited",
    error_phase: "upstream",
    error_owner: "provider",
    error_source: "upstream_http",
    error_message: "quota exceeded",
    error_body_excerpt:
      "class=rate_limited | status=429 | type=rate_limit_error | code=too_many_requests | message=quota exceeded",
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
        data: [mocks.schedulerLog],
        pagination: { page: 1, page_size: 20, total: 1, has_next: false },
      },
      isFetching: false,
      refetch: vi.fn(),
    });
    window.history.replaceState(null, "", "/admin/logs?tab=error");
  });

  it("links each error row to correlated system logs and request dumps", () => {
    render(<ErrorLogsPanel />, { wrapper: wrap });

    expect(screen.getByText("no_available_account")).toBeInTheDocument();
    expect(screen.getByText("capability_mismatch:responses")).toBeInTheDocument();
    expect(screen.getByText("check_model_capabilities_or_mapping")).toBeInTheDocument();
    expect(screen.getByText("系统日志").closest("a")).toHaveAttribute(
      "href",
      "/admin/ops/system-logs?f_request_id=req-row&f_trace_id=trace-row",
    );
    expect(screen.getByText("全部转储").closest("a")).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-files&f_request_id=req-row",
    );
  });

  it("maps provider and error-class URL filters to exact backend query params", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/logs?tab=error&f_provider=3&f_error_class=no_available_account",
    );

    render(<ErrorLogsPanel />, { wrapper: wrap });

    expect(mocks.useAdminErrorLogs).toHaveBeenCalledWith(
      expect.objectContaining({
        provider_id: "3",
        error_class: "no_available_account",
      }),
    );
  });

  it("maps diagnostic URL filters to backend phase owner and status params", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/logs?tab=error&f_error_phase=upstream&f_error_owner=provider&f_status=5xx",
    );

    render(<ErrorLogsPanel />, { wrapper: wrap });

    expect(mocks.useAdminErrorLogs).toHaveBeenCalledWith(
      expect.objectContaining({
        error_phase: "upstream",
        error_owner: "provider",
        status_min: 500,
        status_max: 599,
      }),
    );
  });

  it("summarizes upstream error excerpts in the message column", () => {
    mocks.useAdminErrorLogs.mockReturnValue({
      data: {
        data: [mocks.upstreamLog],
        pagination: { page: 1, page_size: 20, total: 1, has_next: false },
      },
      isFetching: false,
      refetch: vi.fn(),
    });

    render(<ErrorLogsPanel />, { wrapper: wrap });

    expect(screen.getAllByText("rate_limited").length).toBeGreaterThan(0);
    expect(screen.getByText("rate_limit_error")).toBeInTheDocument();
    expect(screen.getByText("too_many_requests")).toBeInTheDocument();
    expect(screen.getByText("quota exceeded")).toBeInTheDocument();
  });
});
