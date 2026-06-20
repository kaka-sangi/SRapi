import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminGatewayResourcesPage from "@/app/admin/gateway-resources/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { AccountHealthSnapshot, ApiKey, Model, Provider, ProviderAccount } from "@/lib/sdk-types";

const storage = new Map<string, string>([["srapi_lang", "zh"]]);
Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
});

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@/hooks/admin-queries", () => ({
  useAdminProviders: () => query([provider({ id: "p1", display_name: "OpenAI" })]),
  useAdminAccounts: () => query([account({ id: "a1", provider_id: "p1", group_ids: ["g1"] })]),
  useAdminApiKeys: () => query([apiKey({ id: "k1", group_ids: ["g1"] })]),
  useAdminModels: () => query([model({ id: "m1" })]),
  useAccountsHealthSummary: () => ({
    data: [health({ account_id: "a1", provider_id: "p1" })],
    isLoading: false,
    isError: false,
  }),
}));

beforeEach(() => {
  storage.clear();
  storage.set("srapi_lang", "zh");
});

describe("AdminGatewayResourcesPage", () => {
  it("renders gateway resource readiness from existing admin resources", () => {
    renderPage();

    expect(screen.getByRole("heading", { name: "网关资源" })).toBeInTheDocument();
    expect(screen.getAllByText("1/1").length).toBeGreaterThan(0);
    expect(screen.getByText("OpenAI")).toBeInTheDocument();
    expect(screen.getByText("就绪")).toBeInTheDocument();
  });
});

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LanguageProvider>
        <AdminGatewayResourcesPage />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

function query<T>(data: T[]) {
  return {
    data: { data, pagination: { page: 1, page_size: 500, total: data.length, has_next: false } },
    isLoading: false,
    isError: false,
  };
}

function provider(overrides: Partial<Provider> = {}): Provider {
  return {
    id: "p1",
    name: "provider",
    display_name: "Provider",
    adapter_type: "openai-compatible",
    protocol: "openai-compatible",
    status: "active",
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function account(overrides: Partial<ProviderAccount> = {}): ProviderAccount {
  return {
    id: "a1",
    provider_id: "p1",
    name: "account",
    runtime_class: "api_key",
    status: "active",
    priority: 0,
    weight: 1,
    group_ids: [],
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function health(overrides: Partial<AccountHealthSnapshot> = {}): AccountHealthSnapshot {
  return {
    account_id: "a1",
    provider_id: "p1",
    runtime_class: "api_key",
    status: "healthy",
    success_rate: 1,
    error_rate: 0,
    latency_p50_ms: 10,
    latency_p95_ms: 20,
    quota_remaining_ratio: 1,
    quota_exhausted: false,
    rate_limit_count: 0,
    timeout_count: 0,
    circuit_state: "closed",
    snapshot_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function apiKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: "k1",
    name: "key",
    prefix: "sk-test",
    status: "active",
    scopes: ["gateway:invoke"],
    allowed_models: [],
    group_ids: [],
    allowed_ips: [],
    denied_ips: [],
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function model(overrides: Partial<Model> = {}): Model {
  return {
    id: "m1",
    canonical_name: "gpt-test",
    display_name: "GPT Test",
    status: "active",
    capabilities: [],
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}
