import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AccountDetailSheet } from "@/components/admin/account-detail-sheet";
import { LanguageProvider } from "@/context/LanguageContext";
import type { ProviderAccount } from "@/lib/sdk-types";

const mocks = vi.hoisted(() => ({
  dailyUsage: [
    {
      date: "2026-05-18",
      requests: 0,
      input_tokens: 0,
      output_tokens: 0,
      cost: "0.00",
      currency: "USD",
    },
    {
      date: "2026-06-10",
      requests: 0,
      input_tokens: 0,
      output_tokens: 0,
      cost: "0.00",
      currency: "USD",
    },
    {
      date: "2026-06-12",
      requests: 4,
      input_tokens: 40,
      output_tokens: 8,
      cost: "0.04",
      currency: "USD",
    },
    {
      date: "2026-06-13",
      requests: 0,
      input_tokens: 0,
      output_tokens: 0,
      cost: "0.00",
      currency: "USD",
    },
  ],
}));

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

vi.mock("@/hooks/admin-queries", () => ({
  useAccountHealth: () => ({
    data: {
      account_id: "acct-1",
      provider_id: "provider-1",
      runtime_class: "oauth_refresh",
      status: "healthy",
      success_rate: 1,
      error_rate: 0,
      latency_p50_ms: 80,
      latency_p95_ms: 120,
      quota_remaining_ratio: 0.8,
      quota_exhausted: false,
      rate_limit_count: 0,
      timeout_count: 0,
      circuit_state: "closed",
      snapshot_at: "2026-06-21T00:00:00Z",
    },
    isLoading: false,
    isError: false,
  }),
  useAccountQuota: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useFetchAccountQuota: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAccountRpmStatus: () => ({
    data: { rpm_used: 2, rpm_limit: 10, window_seconds: 60 },
    isLoading: false,
    isError: false,
  }),
  useAccountProxyQuality: () => ({
    data: {
      proxy_id: "proxy-1",
      success_rate: 1,
      latency_p95_ms: 95,
      sample_count: 3,
    },
    isLoading: false,
    isError: false,
  }),
  useAccountUsageWindows: () => ({
    data: {
      account_id: "acct-1",
      windows: [
        {
          window: "5h",
          requests: 1,
          input_tokens: 10,
          output_tokens: 5,
          total_tokens: 15,
          cost: "0.01",
          currency: "USD",
          success_count: 1,
          error_count: 0,
          first_request_at: "2026-06-12T08:01:00Z",
          last_request_at: "2026-06-12T08:03:00Z",
        },
        {
          window: "7d",
          requests: 4,
          input_tokens: 40,
          output_tokens: 8,
          total_tokens: 48,
          cost: "0.04",
          currency: "USD",
          success_count: 4,
          error_count: 0,
          first_request_at: "2026-06-12T08:01:00Z",
          last_request_at: "2026-06-12T08:03:00Z",
        },
      ],
    },
    isLoading: false,
    isError: false,
  }),
  useAccountUsageToday: () => ({
    data: {
      requests: 1,
      input_tokens: 10,
      output_tokens: 5,
      total_tokens: 15,
      cost: "0.01",
      currency: "USD",
      success_count: 1,
      error_count: 0,
      success_rate: 1,
    },
    isLoading: false,
    isError: false,
  }),
  ACCOUNT_USAGE_DAILY_MAX_DAYS: 365,
  useAccountUsageDaily: vi.fn((_id: string | null, days?: number) => ({
    data: days === 365 ? mocks.dailyUsage : [],
    isLoading: false,
    isError: false,
  })),
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

describe("AccountDetailSheet", () => {
  beforeEach(() => {
    mocks.dailyUsage = [
      {
        date: "2026-05-18",
        requests: 0,
        input_tokens: 0,
        output_tokens: 0,
        cost: "0.00",
        currency: "USD",
      },
      {
        date: "2026-06-10",
        requests: 0,
        input_tokens: 0,
        output_tokens: 0,
        cost: "0.00",
        currency: "USD",
      },
      {
        date: "2026-06-12",
        requests: 4,
        input_tokens: 40,
        output_tokens: 8,
        cost: "0.04",
        currency: "USD",
      },
      {
        date: "2026-06-13",
        requests: 0,
        input_tokens: 0,
        output_tokens: 0,
        cost: "0.00",
        currency: "USD",
      },
    ];
  });

  it("shows only active usage dates and keeps identity compact", () => {
    renderSheet();

    expect(screen.queryByText("https://internal-upstream.example/v1")).not.toBeInTheDocument();
    expect(screen.getByText("Codex")).toBeInTheDocument();
    expect(screen.getAllByText("ada@example.com").length).toBeGreaterThan(0);
    expect(screen.getByText("允许 1 个")).toBeInTheDocument();
    expect(screen.getByText("Pooled accounts")).toBeInTheDocument();
    expect(screen.getAllByText("已绑定代理").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/最近使用/).length).toBeGreaterThan(0);
    expect(screen.getByText("活跃区间")).toBeInTheDocument();
    expect(screen.getByText("活跃天数")).toBeInTheDocument();
    expect(screen.getByText("1 天")).toBeInTheDocument();
    expect(screen.getAllByText(/08:03|8:03|16:03|4:03/).length).toBeGreaterThan(0);
    expect(screen.getByText("端点覆盖")).toBeInTheDocument();
    expect(screen.getByText("Resp: 强关 Msg: 强开")).toBeInTheDocument();
    expect(screen.getAllByText(/2026年6月12日|Jun 12, 2026/).length).toBeGreaterThan(0);
    const usageTable = screen.getByRole("table");
    expect(within(usageTable).queryByText(/2026年5月18日|May 18, 2026/)).not.toBeInTheDocument();
    expect(within(usageTable).queryByText(/2026年6月10日|Jun 10, 2026/)).not.toBeInTheDocument();
    expect(within(usageTable).getByText(/2026年6月12日|Jun 12, 2026/)).toBeInTheDocument();
    expect(within(usageTable).queryByText(/2026年6月13日|Jun 13, 2026/)).not.toBeInTheDocument();
  });

  it("does not treat account creation as usage when the daily series is empty", () => {
    mocks.dailyUsage = [
      {
        date: "2026-05-18",
        requests: 0,
        input_tokens: 0,
        output_tokens: 0,
        cost: "0.00",
        currency: "USD",
      },
      {
        date: "2026-06-10",
        requests: 0,
        input_tokens: 0,
        output_tokens: 0,
        cost: "0.00",
        currency: "USD",
      },
    ];

    renderSheet();

    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.getAllByText("暂无数据。").length).toBeGreaterThan(0);
  });
});

function renderSheet() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const account: ProviderAccount = {
    id: "acct-1",
    provider_id: "provider-1",
    name: "codex-main",
    runtime_class: "oauth_refresh",
    status: "active",
    priority: 2,
    weight: 3,
    group_ids: ["group-1"],
    proxy_id: "proxy-1",
    metadata: {
      base_url: "https://internal-upstream.example/v1",
      email: "ada@example.com",
      plan_type: "pro",
      organization_id: "org-main",
      max_concurrency: 4,
      supported_models: ["gpt-5"],
      capability_responses: false,
      capability_messages: true,
    },
    concurrency: 4,
    schedulable: true,
    created_at: "2026-06-10T00:00:00Z",
  };

  return render(
    <QueryClientProvider client={client}>
      <LanguageProvider>
        <AccountDetailSheet
          account={account}
          providerName="Codex"
          groupNameById={new Map([["group-1", "Pooled accounts"]])}
          onOpenChange={vi.fn()}
        />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}
