import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";
import { describe, expect, it, vi } from "vitest";
import AdminAccountsPage from "@/app/admin/accounts/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { ProviderAccount } from "@/lib/sdk-types";

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

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/admin/account-form-dialog", () => ({
  AccountFormDialog: () => null,
}));

vi.mock("@/components/admin/bind-proxy-dialog", () => ({
  BindProxyDialog: () => null,
}));

vi.mock("@/components/admin/account-detail-sheet", () => ({
  AccountDetailSheet: () => null,
}));

vi.mock("@/components/features/account-test-dialog", () => ({
  AccountTestDialog: () => null,
}));

vi.mock("@/components/admin/confirm-dialog", () => ({
  ConfirmDialog: () => null,
}));

vi.mock("@/components/admin/account-import-dialog", () => ({
  AccountImportDialog: () => null,
}));

vi.mock("@/app/admin/accounts/bulk-add-dialog", () => ({
  BulkAddAccountsDialog: () => null,
}));

const account = providerAccount({
  id: "acct-1",
  name: "codex-main",
  provider_id: "provider-1",
  runtime_class: "oauth_refresh",
});

vi.mock("@/hooks/admin-queries", () => ({
  useAdminAccounts: () => ({
    data: {
      data: [account],
      pagination: { page: 1, page_size: 20, total: 1, has_next: false },
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useAdminModels: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useAdminProviders: () => ({
    data: { data: [{ id: "provider-1", name: "codex", display_name: "Codex" }] },
    isLoading: false,
    isError: false,
  }),
  useAdminProxies: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useAccountsHealthSummary: () => ({
    data: [
      {
        account_id: "acct-1",
        provider_id: "provider-1",
        runtime_class: "oauth_refresh",
        status: "healthy",
        success_rate: 1,
        error_rate: 0,
        latency_p50_ms: 0,
        latency_p95_ms: 0,
        quota_remaining_ratio: 0.72,
        quota_exhausted: false,
        rate_limit_count: 0,
        timeout_count: 0,
        circuit_state: "closed",
        snapshot_at: "2026-06-11T00:00:00Z",
      },
    ],
    isLoading: false,
    isError: false,
  }),
  useSetAccountStatus: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useTestAccount: () => ({ mutate: vi.fn(), reset: vi.fn(), isPending: false }),
  useCreateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useClearAccountError: () => ({ mutateAsync: vi.fn() }),
  useRecoverAccount: () => ({ mutateAsync: vi.fn() }),
  useRefreshAccount: () => ({ mutateAsync: vi.fn() }),
  useResetAccountQuota: () => ({ mutateAsync: vi.fn() }),
  useBatchActionAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useBatchDeleteAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useBatchUpdateAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useAdminGroups: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useAccountsUsageTodayBatch: () => ({ data: [], isLoading: false, isError: false }),
  useDeleteAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDiscoverAccountModels: () => ({ mutateAsync: vi.fn() }),
  useExportAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

describe("AdminAccountsPage", () => {
  it("defaults to cards and can switch to the table list", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(screen.getByRole("heading", { name: "codex-main" })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "名称" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /列表/ }));

    expect(screen.getByRole("columnheader", { name: "名称" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "codex-main" })).not.toBeInTheDocument();
  });
});

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <LanguageProvider>
        <AdminAccountsPage />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

function providerAccount(overrides: Partial<ProviderAccount> = {}): ProviderAccount {
  return {
    id: "acct-1",
    provider_id: "provider-1",
    name: "codex-main",
    runtime_class: "oauth_refresh",
    status: "active",
    priority: 0,
    weight: 1,
    group_ids: [],
    created_at: "2026-06-11T00:00:00Z",
    ...overrides,
  };
}
