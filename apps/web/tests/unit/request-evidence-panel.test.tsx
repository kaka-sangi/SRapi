import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
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
    has_system_log: true,
    has_scheduler_decision: true,
    request_dump_count: 1,
    request_dump_error_count: 1,
    system_log_count: 1,
    scheduler_decision_count: 1,
    scheduler_decision_id: "77",
    scheduler_candidate_count: 4,
    scheduler_rejected_count: 1,
    scheduler_strategy: "latency_first",
    scheduler_selection_rationale: "lowest latency healthy account",
    latest_request_dump_name: "error-1780000000000-req-evidence.log",
    latest_request_dump_created_at: "2026-06-19T08:00:01Z",
  } satisfies RequestEvidenceRow,
  refetch: vi.fn(),
  useOpsRequestEvidence: vi.fn(),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useOpsRequestEvidence: (params: unknown) => mocks.useOpsRequestEvidence(params),
  useOpsRequestEvidenceDetail: (requestID?: string) => ({
    data: requestID
      ? {
          evidence_request_id: "req-evidence",
          summary: {
            kind: "error",
            primary_source: "usage",
            attempt_count: 1,
            usage_log_count: 1,
            ops_error_log_count: 1,
            request_dump_count: 1,
            request_dump_error_count: 1,
            scheduler_decision_count: 1,
            has_usage_log: true,
            has_ops_error_log: true,
            has_request_dump: true,
            has_scheduler_decision: true,
            scheduler_decision_id: "77",
            scheduler_candidate_count: 4,
            scheduler_rejected_count: 1,
            scheduler_strategy: "latency_first",
            scheduler_selection_rationale: "lowest latency healthy account",
            latency_ms: 891,
            total_tokens: 30,
            status_code: 503,
            error_class: "server_bad",
            error_message: "upstream failed",
          },
          attempts: [mocks.row],
          request_dumps: [
            {
              name: "error-1780000000000-req-evidence.log",
              created_at: "2026-06-19T08:00:01Z",
              size_bytes: 512,
              request_id: "req-evidence",
              is_error_only: true,
              attempt_count: 1,
              response_count: 1,
              has_summary: true,
            },
          ],
          system_log_summary: {
            total_count: 1,
            level_counts: { warn: 1 },
            latest_level: "warn",
            latest_message: "scheduler fallback selected secondary account",
            latest_source: "gateway.scheduler",
            latest_at: "2026-06-19T08:00:02Z",
          },
          system_logs: [
            {
              id: "41",
              level: "warn",
              message: "scheduler fallback selected secondary account",
              source: "gateway.scheduler",
              request_id: "req-evidence",
              trace_id: "trace-evidence",
              created_at: "2026-06-19T08:00:02Z",
            },
          ],
          first_seen_at: "2026-06-19T08:00:00Z",
          last_seen_at: "2026-06-19T08:00:01Z",
          request_id: "req_detail_correlation",
        }
      : undefined,
    isLoading: false,
    isError: false,
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
    mocks.useOpsRequestEvidence.mockReset();
    mocks.useOpsRequestEvidence.mockReturnValue({
      data: {
        data: [mocks.row],
        pagination: { page: 1, page_size: 50, total: 1, has_next: false },
      },
      isFetching: false,
      refetch: mocks.refetch,
    });
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
    expect(screen.getByRole("link", { name: /调度/ })).toHaveAttribute(
      "href",
      "/admin/ops?tab=scheduler-decisions&f_request_id=req-evidence",
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

  it("opens a request investigation from the request id", () => {
    render(<RequestEvidencePanel />, { wrapper: wrap });

    fireEvent.click(screen.getByRole("button", { name: "req-evidence" }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("请求调查")).toBeInTheDocument();
    expect(screen.getByText("尝试")).toBeInTheDocument();
    expect(screen.getByText("U 1 / S 1 / E 1 / D 1")).toBeInTheDocument();
    expect(screen.getByText("1 条 · 警告 1 · 错误 0")).toBeInTheDocument();
    expect(screen.getByText("scheduler fallback selected secondary account")).toBeInTheDocument();
    expect(screen.getByText("error-1780000000000-req-evidence.log")).toBeInTheDocument();
  });

  it("passes URL investigation filters to the request evidence query", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/logs?tab=request-evidence&f_account_id=9&f_provider_id=3&f_error_class=server_bad&f_model=gpt-4.1&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_start=2026-06-19T07%3A55%3A00Z&f_end=2026-06-19T08%3A00%3A00Z&f_sort=latency_desc&f_min_latency_ms=500&f_max_latency_ms=1200",
    );

    render(<RequestEvidencePanel />, { wrapper: wrap });

    expect(screen.getByText("范围")).toBeInTheDocument();
    expect(screen.getByTitle("账号:9")).toBeInTheDocument();
    expect(screen.getByTitle("供应商:3")).toBeInTheDocument();
    expect(screen.getByTitle("端点:/v1/chat/completions")).toBeInTheDocument();
    expect(screen.getByTitle("分类:server_bad")).toBeInTheDocument();
    expect(
      screen.getByTitle("窗口:2026-06-19T07:55:00Z → 2026-06-19T08:00:00Z"),
    ).toBeInTheDocument();
    expect(mocks.useOpsRequestEvidence).toHaveBeenCalledWith(
      expect.objectContaining({
        account_id: "9",
        provider_id: "3",
        error_class: "server_bad",
        model: "gpt-4.1",
        source_endpoint: "/v1/chat/completions",
        sort: "latency_desc",
        min_latency_ms: 500,
        max_latency_ms: 1200,
        start: "2026-06-19T07:55:00Z",
        end: "2026-06-19T08:00:00Z",
      }),
    );
  });

  it("clears scoped investigation chips without dropping unrelated filters", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/logs?tab=request-evidence&f_account_id=9&f_provider_id=3&f_error_class=server_bad&f_start=2026-06-19T07%3A55%3A00Z&f_end=2026-06-19T08%3A00%3A00Z&f_sort=latency_desc",
    );

    const { rerender } = render(<RequestEvidencePanel />, { wrapper: wrap });

    fireEvent.click(screen.getByRole("button", { name: "清除 账号" }));
    rerender(<RequestEvidencePanel />);

    expect(mocks.useOpsRequestEvidence).toHaveBeenLastCalledWith(
      expect.objectContaining({
        account_id: undefined,
        provider_id: "3",
        error_class: "server_bad",
        sort: "latency_desc",
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "清除 窗口" }));
    rerender(<RequestEvidencePanel />);

    expect(mocks.useOpsRequestEvidence).toHaveBeenLastCalledWith(
      expect.objectContaining({
        start: expect.any(String),
        end: undefined,
      }),
    );
    expect(mocks.useOpsRequestEvidence.mock.calls.at(-1)?.[0].start).not.toBe(
      "2026-06-19T07:55:00Z",
    );
  });

  it("maps latency controls into request evidence filters", () => {
    const { rerender } = render(<RequestEvidencePanel />, { wrapper: wrap });

    fireEvent.change(screen.getByPlaceholderText("最小 ms"), { target: { value: "750" } });
    fireEvent.change(screen.getByPlaceholderText("最大 ms"), { target: { value: "1500" } });
    rerender(<RequestEvidencePanel />);

    expect(mocks.useOpsRequestEvidence).toHaveBeenLastCalledWith(
      expect.objectContaining({
        min_latency_ms: 750,
        max_latency_ms: 1500,
      }),
    );
  });
});
