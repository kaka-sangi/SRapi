import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSetAccountStatus } from "@/hooks/admin-queries";
import { queryKeys } from "@/lib/query-keys";
import type { AccountHealthSnapshot, ProviderAccount } from "@/lib/sdk-types";

const mocks = vi.hoisted(() => ({
  setAccountStatus: vi.fn(),
}));

vi.mock("@/lib/admin-api", () => ({
  adminApi: {
    setAccountStatus: mocks.setAccountStatus,
  },
}));

describe("useSetAccountStatus", () => {
  beforeEach(() => {
    mocks.setAccountStatus.mockReset();
  });

  it("only optimistically updates account list caches", async () => {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const account = providerAccount({ id: "1", status: "active" });
    const disabled = providerAccount({ id: "1", status: "disabled" });
    const healthSummary = [accountHealth({ account_id: "1" })];

    mocks.setAccountStatus.mockResolvedValue(disabled);
    client.setQueryData(queryKeys.admin.accounts({ page: 1 }), {
      data: [account],
      pagination: { page: 1, page_size: 20, total: 1, has_next: false },
    });
    client.setQueryData(queryKeys.admin.accountsHealthSummary(), healthSummary);

    const wrapper = ({ children }: PropsWithChildren) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useSetAccountStatus(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({ id: "1", status: "disabled" });
    });

    expect(client.getQueryData(queryKeys.admin.accountsHealthSummary())).toBe(healthSummary);
    expect(client.getQueryData<{ data: ProviderAccount[] }>(queryKeys.admin.accounts({ page: 1 }))?.data[0]?.status).toBe(
      "disabled",
    );
  });
});

function providerAccount(overrides: Partial<ProviderAccount> = {}): ProviderAccount {
  return {
    id: "1",
    provider_id: "2",
    name: "account",
    runtime_class: "oauth_refresh",
    status: "active",
    priority: 0,
    weight: 1,
    group_ids: [],
    concurrency: 3,
    schedulable: true,
    created_at: "2026-06-11T00:00:00Z",
    ...overrides,
  };
}

function accountHealth(overrides: Partial<AccountHealthSnapshot> = {}): AccountHealthSnapshot {
  return {
    account_id: "1",
    provider_id: "2",
    runtime_class: "oauth_refresh",
    status: "healthy",
    success_rate: 1,
    error_rate: 0,
    latency_p50_ms: 0,
    latency_p95_ms: 0,
    quota_remaining_ratio: 1,
    quota_exhausted: false,
    rate_limit_count: 0,
    timeout_count: 0,
    circuit_state: "closed",
    snapshot_at: "2026-06-11T00:00:00Z",
    ...overrides,
  };
}
