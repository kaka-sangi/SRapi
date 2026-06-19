import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminOpsAlertEventsPage from "@/app/admin/ops/alert-events/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { OpsAlertEvent } from "@/lib/sdk-types";

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
  fingerprintQuery: vi.fn(),
  alert: {
    id: "alert-1",
    rule_id: "rule.7",
    severity: "critical",
    status: "firing",
    fingerprint: "rule:7:error_rate:gt",
    summary: "Chat error rate gt 0.25",
    details: {
      rule_name: "Chat error baseline",
      metric_type: "error_rate",
      operator: "gt",
      threshold: 0.25,
      observed_value: 0.5,
      total_requests: 40,
      good_requests: 20,
      bad_requests: 20,
      error_rate: 0.5,
      min_request_count: 10,
      request_id: "req-alert",
      account_id: "acct-1",
      provider_id: "provider-1",
      source_endpoint: "/v1/chat/completions",
      model: "gpt-ops",
      error_class: "timeout",
      window_start: "2026-06-18T09:55:00Z",
      window_end: "2026-06-18T10:00:00Z",
    },
    started_at: "2026-06-18T10:00:00Z",
    created_at: "2026-06-18T10:00:00Z",
    updated_at: "2026-06-18T10:01:00Z",
  } satisfies OpsAlertEvent,
  fingerprint: {
    fingerprint: "fp_timeout",
    count: 12,
    open_count: 7,
    investigating_count: 2,
    resolved_count: 3,
    muted_count: 0,
    first_occurred_at: "2026-06-18T09:56:00Z",
    last_occurred_at: "2026-06-18T09:59:30Z",
    example_error_log_id: "err-1",
    example_request_id: "req-alert",
    example_error_message: "upstream timeout after 30s",
    source_endpoint: "/v1/chat/completions",
    target_protocol: "openai",
    model: "gpt-ops",
    status_code: 504,
    status_class: "5xx",
    error_class: "timeout",
    error_phase: "upstream",
    error_owner: "provider",
    error_source: "gateway",
    message_pattern: "upstream timeout after {n}s",
  },
}));

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@/hooks/admin-queries", () => ({
  useOpsAlertEvents: () => ({
    data: {
      data: [mocks.alert],
      pagination: { page: 1, page_size: 20, total: 1, has_next: false },
    },
    isFetching: false,
  }),
  useAdminErrorLogFingerprints: (params: unknown, enabled = true) => {
    mocks.fingerprintQuery(params, enabled);
    return {
      data: {
        data: [mocks.fingerprint],
        meta: { total: 1, scanned: 12, truncated: false },
      },
      isLoading: false,
      isError: false,
    };
  },
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

describe("AdminOpsAlertEventsPage", () => {
  beforeEach(() => {
    storage.clear();
    mocks.fingerprintQuery.mockClear();
    window.history.replaceState(null, "", "/admin/ops/alert-events");
  });

  it("renders historical alert evidence links", () => {
    render(<AdminOpsAlertEventsPage />, { wrapper: wrap });

    expect(screen.getByText("Chat error rate gt 0.25")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "错误日志" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=%2Fv1%2Fchat%2Fcompletions&f_account=acct-1&f_provider=provider-1&f_error_class=timeout&f_model=gpt-ops",
    );
    expect(screen.getByRole("link", { name: "请求证据" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-evidence&f_request_id=req-alert&f_account_id=acct-1&f_provider_id=provider-1&f_error_class=timeout&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_model=gpt-ops&f_start=2026-06-18T09%3A55%3A00Z&f_end=2026-06-18T10%3A00%3A00Z",
    );
    expect(screen.getByRole("link", { name: "调度决策" })).toHaveAttribute(
      "href",
      "/admin/ops?tab=scheduler-decisions&f_request_id=req-alert&f_account_id=acct-1&f_provider_id=provider-1&f_model=gpt-ops&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_start=2026-06-18T09%3A55%3A00Z&f_end=2026-06-18T10%3A00%3A00Z",
    );
    expect(screen.getByRole("link", { name: "账号健康" })).toHaveAttribute(
      "href",
      "/admin/accounts?view=health&f_providerId=provider-1&f_accountId=acct-1",
    );
    expect(screen.getAllByText("处置路径").length).toBeGreaterThan(0);
    expect(screen.getByText("先看错误日志的 error_class、owner、upstream status 和 attempt。")).toBeInTheDocument();
    expect(screen.getByText("核对请求证据里的模型、端点、账号和上游响应。")).toBeInTheDocument();
    expect(screen.getByText("查看调度拒绝原因、score breakdown 和 fallback 链路。")).toBeInTheDocument();
  });

  it("shows a structured signal summary in the alert detail dialog", async () => {
    const user = userEvent.setup({ delay: null });
    render(<AdminOpsAlertEventsPage />, { wrapper: wrap });

    await user.click(screen.getByLabelText("操作"));
    await user.click(await screen.findByText("查看详情"));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("触发条件")).toBeInTheDocument();
    expect(screen.getByText("Chat error baseline")).toBeInTheDocument();
    expect(screen.getByText("error_rate gt 25.0%")).toBeInTheDocument();
    expect(screen.getAllByText("50.0%").length).toBeGreaterThan(0);
    expect(screen.getByText("流量样本")).toBeInTheDocument();
    expect(screen.getByText("40")).toBeInTheDocument();
    expect(screen.getAllByText("20")).toHaveLength(2);
    expect(screen.getByText("窗口")).toBeInTheDocument();
    expect(screen.getByText("2026-06-18T09:55:00Z")).toBeInTheDocument();
    expect(screen.getByText("范围")).toBeInTheDocument();
    expect(screen.getByText("/v1/chat/completions")).toBeInTheDocument();
    expect(screen.getByText("gpt-ops")).toBeInTheDocument();
    expect(screen.getByText("可能错误簇")).toBeInTheDocument();
    expect(screen.getByText("1 组 · 已扫描 12 行")).toBeInTheDocument();
    expect(screen.getByText("upstream timeout after {n}s")).toBeInTheDocument();
    expect(screen.getByText("/v1/chat/completions · gpt-ops · timeout · provider · 5xx")).toBeInTheDocument();
    expect(screen.getByText("12 次")).toBeInTheDocument();
    expect(screen.getByText("待处理 7")).toBeInTheDocument();
    expect(screen.getByText("调查中 2")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "打开样例" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&id=err-1",
    );
    expect(mocks.fingerprintQuery).toHaveBeenLastCalledWith(
      {
        account_id: "acct-1",
        provider_id: "provider-1",
        model: "gpt-ops",
        error_class: "timeout",
        source_endpoint: "/v1/chat/completions",
        start: "2026-06-18T09:55:00Z",
        end: "2026-06-18T10:00:00Z",
        limit: 5,
      },
      true,
    );
  });
});
