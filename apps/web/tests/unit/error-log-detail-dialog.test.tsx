import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ErrorLogDetailDialog } from "@/components/admin/error-log-detail-dialog";
import { LanguageProvider } from "@/context/LanguageContext";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { OpsErrorLog, OpsSystemLog, RequestLogFileDescriptor } from "@/lib/sdk-types";

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
  detail: {
    id: "err-1",
    occurred_at: "2026-06-18T10:00:00Z",
    created_at: "2026-06-18T10:00:00Z",
    updated_at: "2026-06-18T10:05:00Z",
    request_id: "req-detail",
    trace_id: "trace-detail",
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
    input_tokens: 10,
    output_tokens: 0,
    usage_estimated: false,
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
  file: {
    name: "error-1780000000000-req-detail.log",
    size: 512,
    created_at: "2026-06-18T10:00:01Z",
    request_id: "req-detail",
    is_error_only: true,
  } satisfies RequestLogFileDescriptor,
  systemLog: {
    id: "sys-1",
    level: "warn",
    message: "gateway retry exhausted",
    source: "gateway",
    request_id: "req-detail",
    trace_id: "trace-detail",
    created_at: "2026-06-18T10:00:02Z",
  } satisfies OpsSystemLog,
  updateResolution: vi.fn(),
}));

vi.mock("@/hooks/admin-queries", () => ({
  downloadAdminRequestLogFileText: vi.fn(),
  useAdminErrorLog: () => ({
    data: mocks.detail,
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useAdminRequestLogFileDownload: () => ({
    data: `=== REQUEST INFO ===
Request-ID: req-detail
Account-ID: 12
Source-Protocol: openai-compatible
Source-Endpoint: /v1/chat/completions

=== REQUEST 1 ===
POST https://upstream.invalid/v1/chat/completions

=== RESPONSE 1 ===
Status: 503

=== SUMMARY ===
Success: false
Error-Class: server_bad
Status: 503
Latency-MS: 891
`,
    isError: false,
  }),
  useAdminRequestLogFiles: () => ({
    data: {
      data: [mocks.file],
      pagination: { page: 1, page_size: 3, total: 1, has_next: false },
    },
    isFetching: false,
  }),
  useOpsSystemLogs: () => ({
    data: {
      data: [mocks.systemLog],
      pagination: { page: 1, page_size: 5, total: 1, has_next: false },
    },
    isFetching: false,
  }),
  useUpdateErrorLogResolution: () => ({
    mutate: mocks.updateResolution,
    isPending: false,
  }),
}));

vi.mock("@/hooks/use-account-name-lookup", () => ({
  useAccountNameLookup: () => ({ get: (id?: string | null) => (id ? `account-${id}` : "—") }),
}));
vi.mock("@/hooks/use-api-key-name-lookup", () => ({
  useApiKeyNameLookup: () => ({ get: (id?: string | null) => (id ? `key-${id}` : "—") }),
}));
vi.mock("@/hooks/use-provider-name-lookup", () => ({
  useProviderNameLookup: () => ({ get: (id?: string | null) => (id ? `provider-${id}` : "—") }),
}));
vi.mock("@/hooks/use-user-email-lookup", () => ({
  useUserEmailLookup: () => ({ get: (id?: string | null) => (id ? `user-${id}@example.test` : "—") }),
}));

function wrap({ children }: PropsWithChildren) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LanguageProvider>
        <TooltipProvider>{children}</TooltipProvider>
      </LanguageProvider>
    </QueryClientProvider>
  );
}

describe("ErrorLogDetailDialog request dump evidence", () => {
  beforeEach(() => {
    storage.clear();
    mocks.updateResolution.mockReset();
  });

  it("shows the parsed request dump summary in the evidence preview", () => {
    render(
      <ErrorLogDetailDialog errorLogId="err-1" open onOpenChange={() => undefined} />,
      { wrapper: wrap },
    );

    fireEvent.click(screen.getByRole("button", { name: "预览" }));

    expect(screen.getByText("诊断摘要")).toBeInTheDocument();
    expect(screen.getByText("失败")).toBeInTheDocument();
    expect(screen.getAllByText("503").length).toBeGreaterThan(0);
    expect(screen.getAllByText("server_bad").length).toBeGreaterThan(0);
    expect(screen.getAllByText("891ms").length).toBeGreaterThan(0);
    expect(screen.getByText("1 请求 / 1 响应")).toBeInTheDocument();
  });

  it("surfaces scheduler diagnostics from no-account evidence", () => {
    render(
      <ErrorLogDetailDialog errorLogId="err-1" open onOpenChange={() => undefined} />,
      { wrapper: wrap },
    );

    expect(screen.getByText("调度诊断")).toBeInTheDocument();
    expect(screen.getAllByText("capability_mismatch:responses").length).toBeGreaterThan(0);
    expect(screen.getByText("check_model_capabilities_or_mapping")).toBeInTheDocument();
    expect(screen.getByText("77")).toBeInTheDocument();
    expect(screen.getAllByText("3").length).toBeGreaterThan(0);
    expect(screen.getByText("no account satisfied responses capability")).toBeInTheDocument();
  });

  it("shows related system logs and request evidence links", () => {
    render(
      <ErrorLogDetailDialog errorLogId="err-1" open onOpenChange={() => undefined} />,
      { wrapper: wrap },
    );

    expect(screen.getByText("系统日志上下文")).toBeInTheDocument();
    expect(screen.getByText("gateway retry exhausted")).toBeInTheDocument();
    expect(screen.getAllByText("gateway").length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "系统日志" })).toHaveAttribute(
      "href",
      "/admin/ops/system-logs?f_request_id=req-detail",
    );
    expect(screen.getByRole("link", { name: "请求证据" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-evidence&f_request_id=req-detail",
    );
  });
});
