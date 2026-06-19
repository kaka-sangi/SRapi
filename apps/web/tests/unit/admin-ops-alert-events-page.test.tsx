import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
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
  alert: {
    id: "alert-1",
    rule_id: "rule.7",
    severity: "critical",
    status: "firing",
    fingerprint: "rule:7:error_rate:gt",
    summary: "Chat error rate gt 0.25",
    details: {
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
});
