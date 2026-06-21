import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ChannelPricingPage from "@/app/admin/channels/pricing/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { PricingRule, PricingRulePreset } from "@/lib/sdk-types";

const mocks = vi.hoisted(() => ({
  installPresets: vi.fn(),
  pricingRulesParams: vi.fn(),
  toast: vi.fn(),
}));

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

vi.mock("@/components/admin/resource-form-dialog", () => ({
  ResourceFormDialog: () => null,
}));

vi.mock("@/components/ui/column-toggle", () => ({
  ColumnToggle: () => null,
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useAdminPricingRules: (params?: {
    page?: number;
    page_size?: number;
    q?: string;
    model_id?: string;
    provider_id?: string;
  }) => {
    mocks.pricingRulesParams(params);
    const all = [
      pricingRule({
        id: "rule-1",
        model_family: "gpt-4.1",
      }),
    ];
    const pageSize = params?.page_size ?? all.length;
    return {
      data: {
        data: all.slice(0, pageSize),
        pagination: { page: 1, page_size: pageSize, total: all.length, has_next: false },
      },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
    };
  },
  useAdminPricingRulePresets: () => ({
    data: [
      pricingPreset({ model_family: "gpt-4.1" }),
      pricingPreset({ model_family: "claude-sonnet-4-5", input_price_per_million_tokens: "3" }),
      pricingPreset({ model_family: "gemini-2.5-pro", input_price_per_million_tokens: "1.25" }),
    ],
    isLoading: false,
    isError: false,
  }),
  useAdminModels: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useAdminProviders: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useCreatePricingRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdatePricingRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useBulkImportPricingRules: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useInstallPricingRulePresets: () => ({
    mutateAsync: mocks.installPresets,
    isPending: false,
  }),
  useDeletePricingRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

describe("ChannelPricingPage", () => {
  beforeEach(() => {
    mocks.installPresets.mockReset();
    mocks.pricingRulesParams.mockReset();
    mocks.installPresets.mockResolvedValue({
      created: 2,
      requested: 2,
      validated: 2,
      dry_run: false,
      errors: [],
      rules: [],
    });
    mocks.toast.mockReset();
    storage.clear();
    storage.set("srapi_lang", "zh");
    window.history.replaceState(null, "", "/admin/channels/pricing");
  });

  it("shows built-in price preset coverage and installs only missing families", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(screen.getByText("默认价格预设")).toBeInTheDocument();
    expect(screen.getByText("1/3 已覆盖")).toBeInTheDocument();
    expect(screen.getByText("缺失示例：claude-sonnet-4-5, gemini-2.5-pro")).toBeInTheDocument();
    expect(screen.getAllByText("gpt-4.1").length).toBeGreaterThan(0);

    await user.click(screen.getByRole("button", { name: "安装缺失预设" }));

    await waitFor(() => {
      expect(mocks.installPresets).toHaveBeenCalledWith({
        families: ["claude-sonnet-4-5", "gemini-2.5-pro"],
      });
    });
    expect(mocks.toast).toHaveBeenCalledWith({
      title: "已安装 2 条默认价格规则",
      description: "内置模型系列 2 个，命中 2 个。",
      tone: "success",
    });
  });

  it("restores gateway resource pricing filters from the URL", async () => {
    window.history.replaceState(
      null,
      "",
      "/admin/channels/pricing?q=gpt-4.1&f_modelId=m2&f_providerId=p1",
    );

    renderPage();

    await waitFor(() => {
      expect(mocks.pricingRulesParams).toHaveBeenCalledWith(
        expect.objectContaining({
          q: "gpt-4.1",
          model_id: "m2",
          provider_id: "p1",
        }),
      );
    });
    expect(screen.getByDisplayValue("gpt-4.1")).toBeInTheDocument();
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
        <ChannelPricingPage />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

function pricingPreset(overrides: Partial<PricingRulePreset> = {}): PricingRulePreset {
  return {
    model_family: "gpt-4.1",
    billing_mode: "token",
    input_price_per_million_tokens: "2",
    output_price_per_million_tokens: "8",
    cache_read_price_per_million_tokens: "0.5",
    cache_write_price_per_million_tokens: "2",
    per_request_price: "0",
    currency: "USD",
    source: "built_in_litellm_fallback",
    ...overrides,
  };
}

function pricingRule(overrides: Partial<PricingRule> = {}): PricingRule {
  return {
    id: "rule-1",
    model_id: "0",
    model_family: "gpt-4.1",
    provider_id: "0",
    billing_mode: "token",
    input_price_per_million_tokens: "2",
    output_price_per_million_tokens: "8",
    cache_read_price_per_million_tokens: "0.5",
    cache_write_price_per_million_tokens: "2",
    per_request_price: "0",
    intervals: [],
    currency: "USD",
    created_at: "2026-06-21T00:00:00Z",
    updated_at: "2026-06-21T00:00:00Z",
    ...overrides,
  };
}
