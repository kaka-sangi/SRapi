import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RequestEvidencePanel } from "@/app/admin/logs/_panels/request-evidence-panel";
import { LanguageProvider } from "@/context/LanguageContext";
import type { RequestEvidenceRow } from "@/lib/sdk-types";

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
  row: {
    kind: "error",
    evidence_source: "usage",
    created_at: "2026-06-19T08:00:00Z",
    request_id: "req-evidence",
    usage_log_id: "11",
    ops_error_log_id: "22",
    user_id: "42",
    api_key_id: "7",
    account_id: "9",
    provider_id: "3",
    source_protocol: "openai-compatible",
    source_endpoint: "/v1/chat/completions",
    target_protocol: "openai",
    model: "gpt-4.1",
    status_code: 503,
    success: false,
    error_class: "server_bad",
    error_message: "upstream failed",
    error_phase: "upstream",
    error_owner: "provider",
    error_source: "upstream_http",
    upstream_request_id: "up-req",
    attempt_no: 1,
    latency_ms: 891,
    input_tokens: 10,
    output_tokens: 20,
    total_tokens: 30,
    usage_estimated: true,
    resolution: "open",
    has_usage_log: true,
    has_ops_error_log: true,
    has_request_dump: true,
    request_dump_count: 1,
    request_dump_error_count: 1,
    latest_request_dump_name: "error-1780000000000-req-evidence.log",
    latest_request_dump_created_at: "2026-06-19T08:00:01Z",
  } satisfies RequestEvidenceRow,
  refetch: vi.fn(),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useOpsRequestEvidence: () => ({
    data: {
      data: [mocks.row],
      pagination: { page: 1, page_size: 50, total: 1, has_next: false },
    },
    isFetching: false,
    refetch: mocks.refetch,
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

describe("RequestEvidencePanel", () => {
  beforeEach(() => {
    storage.clear();
    mocks.refetch.mockReset();
    window.history.replaceState(null, "", "/admin/logs?tab=request-evidence");
  });

  it("renders request-level evidence and correlated links", () => {
    render(<RequestEvidencePanel />, { wrapper: wrap });

    expect(screen.getByText("请求证据")).toBeInTheDocument();
    expect(screen.getByText("req-evidence")).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1")).toBeInTheDocument();
    expect(screen.getByText("server_bad")).toBeInTheDocument();
    expect(screen.getByText("891ms")).toBeInTheDocument();
    expect(screen.getByText("30")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /错误/ })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=req-evidence",
    );
    expect(screen.getByRole("link", { name: /系统/ })).toHaveAttribute(
      "href",
      "/admin/ops/system-logs?f_request_id=req-evidence",
    );
    expect(screen.getByRole("link", { name: /转储/ })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-files&f_request_id=req-evidence",
    );
  });
});
