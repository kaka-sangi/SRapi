import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminGatewayResourcesPage from "@/app/admin/gateway-resources/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { GatewayResourceSummary, Provider } from "@/lib/sdk-types";

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
  useAdminGatewayResources: () => ({
    data: summary(),
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
    expect(screen.getByText("可用代理")).toBeInTheDocument();
    expect(screen.getByText("模型映射")).toBeInTheDocument();
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

function summary(): GatewayResourceSummary {
  return {
    providers: 1,
    active_providers: 1,
    active_models: 1,
    active_model_mappings: 1,
    active_api_keys: 1,
    active_accounts: 1,
    routable_accounts: 1,
    active_proxies: 1,
    available_proxies: 1,
    expired_proxies: 0,
    proxied_accounts: 1,
    proxy_attention_accounts: 0,
    scoped_api_keys: 1,
    rows: [
      {
        provider: provider({ id: "p1", display_name: "OpenAI" }),
        total_accounts: 1,
        routable_accounts: 1,
        attention_accounts: 0,
        proxied_accounts: 1,
        proxy_attention_accounts: 0,
        active_model_mappings: 1,
        api_key_count: 1,
        scoped_key_count: 1,
        status: "ready",
        reasons: [],
      },
    ],
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
