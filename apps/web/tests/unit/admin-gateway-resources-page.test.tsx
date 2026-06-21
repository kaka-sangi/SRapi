import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminGatewayResourcesPage from "@/app/admin/gateway-resources/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { GatewayResourceSummary, Model, Provider } from "@/lib/sdk-types";

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
    refetch: vi.fn(),
  }),
  useAdminSettings: () => ({
    data: {
      gateway: {
        protocol_conversion_routes: [
          "chat_completions_to_responses",
          "responses_to_chat_completions",
          "messages_to_responses",
        ],
      },
    },
    isLoading: false,
    isError: false,
  }),
  useAdminPricingRulePresets: () => ({
    data: [{ model_family: "gpt-4.1" }],
    isLoading: false,
  }),
  useInstallPricingRulePresets: () => ({
    isPending: false,
    mutateAsync: vi.fn(),
  }),
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

beforeEach(() => {
  window.history.replaceState(null, "", "/admin/gateway-resources");
  storage.clear();
  storage.set("srapi_lang", "zh");
});

describe("AdminGatewayResourcesPage", () => {
  it("renders gateway resource readiness from existing admin resources", () => {
    renderPage();

    expect(screen.getByRole("heading", { name: "网关资源" })).toBeInTheDocument();
    expect(screen.getByText("可用代理")).toBeInTheDocument();
    expect(screen.getAllByText("模型映射").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1/1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("OpenAI").length).toBeGreaterThan(0);
    expect(screen.getByText("健康")).toBeInTheDocument();
    expect(screen.getByText("配额")).toBeInTheDocument();
    expect(screen.getAllByText("代理").length).toBeGreaterThan(1);
    expect(screen.getByRole("link", { name: "账号" })).toHaveAttribute(
      "href",
      "/admin/accounts?f_providerId=p1",
    );
    expect(screen.getByRole("link", { name: "代理" })).toHaveAttribute("href", "/admin/proxies");
    expect(
      screen
        .getAllByRole("link", { name: "证据" })
        .some(
          (link) =>
            link.getAttribute("href") === "/admin/logs?tab=request-evidence&f_provider_id=p1",
        ),
    ).toBe(true);
    expect(
      screen
        .getAllByRole("link", { name: "决策" })
        .some(
          (link) =>
            link.getAttribute("href") === "/admin/ops?tab=scheduler-decisions&f_provider_id=p1",
        ),
    ).toBe(true);
    expect(screen.getByRole("heading", { name: /端点总览/ })).toBeInTheDocument();
    expect(screen.getAllByText("路由").length).toBeGreaterThan(0);
    expect(
      screen
        .getAllByRole("link", { name: "路由" })
        .some(
          (link) =>
            link.getAttribute("href") ===
            "/admin/gateway-resources?f_scope=routes&q=%2Fv1%2Fresponses%2Fcompact",
        ),
    ).toBe(true);
    expect(
      screen
        .getAllByRole("link", { name: "证据" })
        .some(
          (link) =>
            link.getAttribute("href") ===
            "/admin/logs?tab=request-evidence&f_source_endpoint=%2Fv1%2Fresponses%2Fcompact",
        ),
    ).toBe(true);
    expect(
      screen
        .getAllByRole("link", { name: "决策" })
        .some(
          (link) =>
            link.getAttribute("href") ===
            "/admin/ops?tab=scheduler-decisions&f_source_endpoint=%2Fv1%2Fresponses%2Fcompact",
        ),
    ).toBe(true);
    expect(screen.getByText("模型可服务性")).toBeInTheDocument();
    expect(screen.getAllByText("gpt-4.1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("端点").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Chat").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Compact").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Image").length).toBeGreaterThan(0);
    expect(screen.getAllByText("RT WS").length).toBeGreaterThan(0);
    expect(screen.getByText("端点开关")).toBeInTheDocument();
    expect(screen.getAllByTitle(/Chat Completions 当前为 自动/).length).toBeGreaterThan(0);
    expect(screen.getAllByTitle(/Responses 当前为 强制开启/).length).toBeGreaterThan(0);
    expect(screen.getAllByTitle(/Messages 当前为 强制关闭/).length).toBeGreaterThan(0);
    expect(screen.getByText("协议转换路由")).toBeInTheDocument();
    expect(screen.getByText("3/6 已启用")).toBeInTheDocument();
    expect(screen.getByText("打开转换设置")).toHaveAttribute("href", "/admin/settings?tab=gateway");
    expect(screen.getByTitle(/Chat Completions → Responses 当前为 已启用/)).toHaveAttribute(
      "href",
      "/admin/settings?tab=gateway",
    );
    expect(screen.getByTitle(/Chat Completions → Messages 当前为 已关闭/)).toHaveAttribute(
      "href",
      "/admin/settings?tab=gateway",
    );
    expect(screen.getAllByText("计费").length).toBeGreaterThan(0);
    expect(screen.getAllByText("规则定价").length).toBeGreaterThan(0);
    expect(screen.getAllByText("3/3").length).toBeGreaterThan(0);
    expect(screen.getByText("路由明细")).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1-upstream")).toBeInTheDocument();
    expect(screen.getAllByText("就绪").length).toBeGreaterThan(0);
    expect(screen.getByText("优先修复")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("搜索供应商、模型、上游模型或端点…")).toBeInTheDocument();
    expect(screen.getByText("全部状态")).toBeInTheDocument();
    expect(screen.getByText("全部问题")).toBeInTheDocument();
    expect(screen.getByText("全部矩阵")).toBeInTheDocument();
    expect(screen.getByTitle("有 2 个账号或资源当前不可路由。")).toHaveAttribute(
      "href",
      "/admin/gateway-resources?f_reason=no_routable_accounts&f_scope=providers",
    );
    expect(screen.getByTitle("有 1 个账号的代理绑定需要处理。")).toHaveAttribute(
      "href",
      "/admin/gateway-resources?f_reason=proxy_attention&f_scope=providers",
    );
  });

  it("filters matrices from gateway resource fix links", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/gateway-resources?f_scope=providers&f_reason=proxy_attention",
    );

    renderPage();

    expect(screen.getByText("供应商可服务性")).toBeInTheDocument();
    expect(screen.queryByText("模型可服务性")).not.toBeInTheDocument();
    expect(screen.queryByText("路由明细")).not.toBeInTheDocument();
    expect(screen.getAllByText("OpenAI").length).toBeGreaterThan(0);
    expect(screen.queryByText("没有匹配的网关资源")).not.toBeInTheDocument();
  });

  it("filters endpoint summary from shared scope links", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/gateway-resources?f_scope=endpoints&f_status=blocked&q=compact",
    );

    renderPage();

    expect(screen.getByRole("heading", { name: /端点总览/ })).toBeInTheDocument();
    expect(screen.getByText("Responses Compact")).toBeInTheDocument();
    expect(screen.getByText("/v1/responses/compact")).toBeInTheDocument();
    expect(screen.queryByText("供应商可服务性")).not.toBeInTheDocument();
    expect(screen.queryByText("模型可服务性")).not.toBeInTheDocument();
    expect(screen.queryByText("路由明细")).not.toBeInTheDocument();
    expect(screen.queryByText("没有匹配的网关资源")).not.toBeInTheDocument();
  });

  it("filters default-zero routes from pricing uncovered fix links", () => {
    window.history.replaceState(
      null,
      "",
      "/admin/gateway-resources?f_scope=routes&f_reason=pricing_uncovered",
    );

    renderPage();

    expect(screen.getByText("路由明细")).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1-free-upstream")).toBeInTheDocument();
    expect(screen.getAllByText("默认零价").length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "定价" })).toHaveAttribute(
      "href",
      "/admin/channels/pricing?f_modelId=m2&f_providerId=p1",
    );
    expect(screen.getByRole("link", { name: "证据" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=request-evidence&f_provider_id=p1&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions&f_model=gpt-4.1-free",
    );
    expect(screen.getByRole("link", { name: "决策" })).toHaveAttribute(
      "href",
      "/admin/ops?tab=scheduler-decisions&f_provider_id=p1&f_model=gpt-4.1-free&f_source_endpoint=%2Fv1%2Fchat%2Fcompletions",
    );
    expect(screen.queryByText("gpt-4.1-upstream")).not.toBeInTheDocument();
    expect(screen.queryByText("没有匹配的网关资源")).not.toBeInTheDocument();
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
    proxy_attention_accounts: 1,
    scoped_api_keys: 1,
    fixes: [
      {
        severity: "critical",
        area: "accounts",
        reason: "no_routable_accounts",
        count: 2,
        href: "/admin/gateway-resources?f_reason=no_routable_accounts&f_scope=providers",
      },
      {
        severity: "warning",
        area: "proxies",
        reason: "proxy_attention",
        count: 1,
        href: "/admin/gateway-resources?f_reason=proxy_attention&f_scope=providers",
      },
      {
        severity: "warning",
        area: "pricing",
        reason: "pricing_uncovered",
        count: 1,
        href: "/admin/gateway-resources?f_reason=pricing_uncovered&f_scope=routes",
      },
    ],
    endpoint_rows: [
      endpointSummary("chat_completions", "/v1/chat/completions", 1, 1, 1, 1, 1, 1, "ready"),
      endpointSummary("responses", "/v1/responses", 1, 1, 1, 1, 1, 1, "ready"),
      endpointSummary("responses_websocket", "/v1/responses/ws", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary("responses_compact", "/v1/responses/compact", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary(
        "responses_input_items",
        "/v1/responses/{response_id}/input_items",
        0,
        1,
        0,
        1,
        0,
        1,
        "blocked",
        1,
      ),
      endpointSummary("messages", "/v1/messages", 1, 1, 1, 1, 1, 1, "ready"),
      endpointSummary(
        "anthropic_count_tokens",
        "/v1/messages/count_tokens",
        0,
        1,
        0,
        1,
        0,
        1,
        "blocked",
        1,
      ),
      endpointSummary(
        "gemini_generate_content",
        "/v1beta/models/{model}:generateContent",
        0,
        1,
        0,
        1,
        0,
        1,
        "blocked",
        1,
      ),
      endpointSummary(
        "gemini_count_tokens",
        "/v1beta/models/{model}:countTokens",
        0,
        1,
        0,
        1,
        0,
        1,
        "blocked",
        1,
      ),
      endpointSummary("embeddings", "/v1/embeddings", 1, 1, 1, 1, 1, 1, "ready"),
      endpointSummary("image_generations", "/v1/images/generations", 1, 1, 1, 1, 1, 1, "ready"),
      endpointSummary("image_edits", "/v1/images/edits", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary("image_variations", "/v1/images/variations", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary("videos", "/v1/videos", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary(
        "audio_transcriptions",
        "/v1/audio/transcriptions",
        0,
        1,
        0,
        1,
        0,
        1,
        "blocked",
        1,
      ),
      endpointSummary("audio_speech", "/v1/audio/speech", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary("moderations", "/v1/moderations", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary("rerank", "/v1/rerank", 0, 1, 0, 1, 0, 1, "blocked", 1),
      endpointSummary("realtime_websocket", "/v1/realtime", 0, 1, 0, 1, 0, 1, "blocked", 1),
    ],
    rows: [
      {
        provider: provider({
          id: "p1",
          name: "openai",
          display_name: "OpenAI",
          capabilities: {
            responses: true,
            messages: false,
          },
        }),
        total_accounts: 1,
        routable_accounts: 1,
        attention_accounts: 0,
        account_blockers: {
          inactive: 0,
          health: 1,
          quota: 1,
          proxy: 1,
        },
        proxied_accounts: 1,
        proxy_attention_accounts: 1,
        active_model_mappings: 1,
        api_key_count: 1,
        scoped_key_count: 1,
        status: "limited",
        reasons: ["proxy_attention"],
      },
    ],
    model_rows: [
      {
        model: model({
          id: "m1",
          canonical_name: "gpt-4.1",
          display_name: "GPT 4.1",
          family: "gpt-4.1",
        }),
        active_providers: 1,
        active_model_mappings: 1,
        routable_accounts: 1,
        endpoints: [
          endpoint("chat_completions", "/v1/chat/completions", 1, 1, "ready"),
          endpoint("responses", "/v1/responses", 1, 1, "ready"),
          endpoint("responses_websocket", "/v1/responses/ws", 0, 1, "blocked", 1),
          endpoint("responses_compact", "/v1/responses/compact", 0, 1, "blocked", 1),
          endpoint(
            "responses_input_items",
            "/v1/responses/{response_id}/input_items",
            0,
            1,
            "blocked",
            1,
          ),
          endpoint("messages", "/v1/messages", 1, 1, "ready"),
          endpoint("anthropic_count_tokens", "/v1/messages/count_tokens", 0, 1, "blocked", 1),
          endpoint(
            "gemini_generate_content",
            "/v1beta/models/{model}:generateContent",
            0,
            1,
            "blocked",
            1,
          ),
          endpoint("gemini_count_tokens", "/v1beta/models/{model}:countTokens", 0, 1, "blocked", 1),
          endpoint("embeddings", "/v1/embeddings", 1, 1, "ready"),
          endpoint("image_generations", "/v1/images/generations", 1, 1, "ready"),
          endpoint("image_edits", "/v1/images/edits", 0, 1, "blocked", 1),
          endpoint("image_variations", "/v1/images/variations", 0, 1, "blocked", 1),
          endpoint("videos", "/v1/videos", 0, 1, "blocked", 1),
          endpoint("audio_transcriptions", "/v1/audio/transcriptions", 0, 1, "blocked", 1),
          endpoint("audio_speech", "/v1/audio/speech", 0, 1, "blocked", 1),
          endpoint("moderations", "/v1/moderations", 0, 1, "blocked", 1),
          endpoint("rerank", "/v1/rerank", 0, 1, "blocked", 1),
          endpoint("realtime_websocket", "/v1/realtime", 0, 1, "blocked", 1),
        ],
        pricing: {
          status: "priced",
          source: "pricing_rule",
          pricing_rule_id: 1,
          priced_routes: 3,
          total_routes: 3,
          currency: "USD",
          billing_mode: "token",
        },
        api_key_count: 1,
        scoped_key_count: 1,
        status: "ready",
        reasons: [],
      },
    ],
    route_rows: [
      {
        model: model({
          id: "m1",
          canonical_name: "gpt-4.1",
          display_name: "GPT 4.1",
          family: "gpt-4.1",
        }),
        provider: provider({
          id: "p1",
          name: "openai",
          display_name: "OpenAI",
          capabilities: {
            responses: true,
            messages: false,
          },
        }),
        mapping_id: "mapping-1",
        upstream_model: "gpt-4.1-upstream",
        routable_accounts: 1,
        endpoints: [
          endpoint("chat_completions", "/v1/chat/completions", 1, 1, "ready"),
          endpoint("responses", "/v1/responses", 1, 1, "ready"),
          endpoint("responses_websocket", "/v1/responses/ws", 0, 1, "blocked", 1),
          endpoint("responses_compact", "/v1/responses/compact", 0, 1, "blocked", 1),
          endpoint(
            "responses_input_items",
            "/v1/responses/{response_id}/input_items",
            0,
            1,
            "blocked",
            1,
          ),
          endpoint("messages", "/v1/messages", 1, 1, "ready"),
          endpoint("anthropic_count_tokens", "/v1/messages/count_tokens", 0, 1, "blocked", 1),
          endpoint(
            "gemini_generate_content",
            "/v1beta/models/{model}:generateContent",
            0,
            1,
            "blocked",
            1,
          ),
          endpoint("gemini_count_tokens", "/v1beta/models/{model}:countTokens", 0, 1, "blocked", 1),
          endpoint("embeddings", "/v1/embeddings", 1, 1, "ready"),
          endpoint("image_generations", "/v1/images/generations", 1, 1, "ready"),
          endpoint("image_edits", "/v1/images/edits", 0, 1, "blocked", 1),
          endpoint("image_variations", "/v1/images/variations", 0, 1, "blocked", 1),
          endpoint("videos", "/v1/videos", 0, 1, "blocked", 1),
          endpoint("audio_transcriptions", "/v1/audio/transcriptions", 0, 1, "blocked", 1),
          endpoint("audio_speech", "/v1/audio/speech", 0, 1, "blocked", 1),
          endpoint("moderations", "/v1/moderations", 0, 1, "blocked", 1),
          endpoint("rerank", "/v1/rerank", 0, 1, "blocked", 1),
          endpoint("realtime_websocket", "/v1/realtime", 0, 1, "blocked", 1),
        ],
        pricing: {
          status: "priced",
          source: "pricing_rule",
          pricing_rule_id: 1,
          priced_routes: 3,
          total_routes: 3,
          currency: "USD",
          billing_mode: "token",
        },
        api_key_count: 1,
        scoped_key_count: 1,
        status: "ready",
        reasons: [],
      },
      {
        model: model({
          id: "m2",
          canonical_name: "gpt-4.1-free",
          display_name: "GPT 4.1 Free",
          family: "gpt-4.1",
        }),
        provider: provider({
          id: "p1",
          name: "openai",
          display_name: "OpenAI",
          capabilities: {
            responses: true,
            messages: false,
          },
        }),
        mapping_id: "mapping-2",
        upstream_model: "gpt-4.1-free-upstream",
        routable_accounts: 1,
        endpoints: [
          endpoint("chat_completions", "/v1/chat/completions", 1, 1, "ready"),
          endpoint("responses", "/v1/responses", 1, 1, "ready"),
          endpoint("messages", "/v1/messages", 1, 1, "ready"),
        ],
        pricing: {
          status: "estimated_zero",
          source: "default_zero",
          priced_routes: 0,
          total_routes: 3,
          currency: "USD",
          billing_mode: "token",
        },
        api_key_count: 1,
        scoped_key_count: 1,
        status: "ready",
        reasons: [],
      },
    ],
  };
}

function endpoint(
  key: NonNullable<GatewayResourceSummary["model_rows"][number]["endpoints"][number]["key"]>,
  sourceEndpoint: string,
  routableAccounts: number,
  candidateAccounts: number,
  status: "ready" | "limited" | "blocked",
  unsupportedAccounts = 0,
) {
  return {
    key,
    source_endpoint: sourceEndpoint,
    routable_accounts: routableAccounts,
    candidate_accounts: candidateAccounts,
    unsupported_accounts: unsupportedAccounts,
    unavailable_model_accounts: 0,
    status,
  };
}

function endpointSummary(
  key: GatewayResourceSummary["endpoint_rows"][number]["key"],
  sourceEndpoint: string,
  readyModels: number,
  models: number,
  readyRoutes: number,
  routes: number,
  routableAccountRoutes: number,
  candidateAccountRoutes: number,
  status: "ready" | "limited" | "blocked",
  unsupportedAccountRoutes = 0,
  unavailableModelAccountRoutes = 0,
) {
  return {
    key,
    source_endpoint: sourceEndpoint,
    ready_models: readyModels,
    models,
    ready_routes: readyRoutes,
    routes,
    routable_account_routes: routableAccountRoutes,
    candidate_account_routes: candidateAccountRoutes,
    unsupported_account_routes: unsupportedAccountRoutes,
    unavailable_model_account_routes: unavailableModelAccountRoutes,
    status,
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

function model(overrides: Partial<Model> = {}): Model {
  return {
    id: "m1",
    canonical_name: "model",
    display_name: "Model",
    status: "active",
    capabilities: [],
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}
