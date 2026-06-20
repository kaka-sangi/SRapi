import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminAccountsPage from "@/app/admin/accounts/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { ProviderAccount } from "@/lib/sdk-types";

const mocks = vi.hoisted(() => ({
  search: "",
  focusedAccount: undefined as ProviderAccount | undefined,
  accountDetailSheet: vi.fn(() => null),
  bulkUpdateMutateAsync: vi.fn(),
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

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(mocks.search),
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
  AccountDetailSheet: mocks.accountDetailSheet,
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
  group_ids: ["group-1"],
  proxy_id: "proxy-1",
  priority: 2,
  weight: 3,
  metadata: {
    base_url: "https://internal-upstream.example/v1",
    supported_models: ["gpt-5", "gpt-5-mini"],
  },
});

vi.mock("@/hooks/admin-queries", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/admin-queries")>(
    "@/hooks/admin-queries",
  );
  return {
    ...actual,
    useAdminAccounts: () => ({
      data: {
        data: [account],
        pagination: { page: 1, page_size: 20, total: 1, has_next: false },
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    }),
    useAdminAccount: () => ({
      data: mocks.focusedAccount,
      isLoading: false,
      isError: false,
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
    useBatchUpdateAccountConcurrency: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useBatchRefreshAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useBatchUpdateAccountCredentials: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useBatchUpdateAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useBulkUpdateAccounts: () => ({
      mutateAsync: mocks.bulkUpdateMutateAsync,
      isPending: false,
    }),
    useBatchQuotaFetchAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useAdminGroups: () => ({
      data: { data: [{ id: "group-1", name: "Pooled accounts" }] },
      isLoading: false,
      isError: false,
    }),
    useAccountsUsageTodayBatch: () => ({
      data: [
        {
          account_id: "acct-1",
          requests: 12,
          input_tokens: 1200,
          output_tokens: 300,
          total_tokens: 1500,
          cost: "0.42",
          currency: "USD",
          success_count: 11,
          error_count: 1,
          success_rate: 11 / 12,
        },
      ],
      isLoading: false,
      isError: false,
    }),
    useDeleteAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useDiscoverAccountModels: () => ({ mutateAsync: vi.fn() }),
    useExportAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
  };
});

describe("AdminAccountsPage", () => {
  beforeEach(() => {
    mocks.search = "";
    mocks.focusedAccount = undefined;
    mocks.accountDetailSheet.mockClear();
    mocks.bulkUpdateMutateAsync.mockReset();
    mocks.bulkUpdateMutateAsync.mockResolvedValue({
      updated_count: 1,
      updated_ids: ["acct-1"],
      errors: [],
    });
    window.history.replaceState(null, "", "/admin/accounts");
  });

  it("defaults to cards and can switch to the table list", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(screen.getByRole("heading", { name: "codex-main" })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "名称" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /列表/ }));

    expect(screen.getByRole("columnheader", { name: "名称" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "codex-main" })).not.toBeInTheDocument();
  });

  it("surfaces routing-critical account facts without exposing base URLs", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(screen.getByText("允许 2 个")).toBeInTheDocument();
    expect(screen.getByText("已绑定代理")).toBeInTheDocument();
    expect(screen.getByText("Pooled accounts")).toBeInTheDocument();
    expect(screen.getByText(/12.*请求/i)).toBeInTheDocument();
    expect(screen.queryByText("https://internal-upstream.example/v1")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /列表/ }));

    expect(screen.getByRole("columnheader", { name: "模型" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "代理" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "路由" })).toBeInTheDocument();
    expect(screen.getAllByText("允许 2 个").length).toBeGreaterThan(0);
    expect(screen.getByText("P2")).toBeInTheDocument();
    expect(screen.getByText("W3")).toBeInTheDocument();
    expect(screen.queryByText("https://internal-upstream.example/v1")).not.toBeInTheDocument();
  });

  it("opens the focused account detail sheet from a health deep link", async () => {
    mocks.search = "view=health&f_providerId=provider-1&f_accountId=acct-2";
    mocks.focusedAccount = providerAccount({
      id: "acct-2",
      name: "codex-fallback",
      provider_id: "provider-1",
    });
    window.history.replaceState(null, "", `/admin/accounts?${mocks.search}`);

    renderPage();

    expect(await screen.findByText("正在定位账号：codex-fallback（acct-2）")).toBeInTheDocument();
    await waitFor(() => {
      expect(mocks.accountDetailSheet).toHaveBeenLastCalledWith(
        expect.objectContaining({
          account: expect.objectContaining({ id: "acct-2", name: "codex-fallback" }),
        }),
        undefined,
      );
    });
  });

  it("adds an account group from selected bulk edit", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByLabelText("select row"));
    await user.click(screen.getByRole("button", { name: "批量编辑" }));
    await user.click(screen.getByLabelText("账号组"));
    await user.selectOptions(screen.getByDisplayValue("Pooled accounts"), "group-1");
    await user.click(screen.getByRole("button", { name: "应用" }));

    await waitFor(() => {
      expect(mocks.bulkUpdateMutateAsync).toHaveBeenCalledWith({
        account_ids: ["acct-1"],
        add_group_id: "group-1",
      });
    });
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
