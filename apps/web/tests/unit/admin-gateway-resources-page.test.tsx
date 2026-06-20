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
    expect(screen.getAllByText("模型映射").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1/1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("OpenAI").length).toBeGreaterThan(0);
    expect(screen.getByText("健康")).toBeInTheDocument();
    expect(screen.getByText("配额")).toBeInTheDocument();
    expect(screen.getAllByText("代理").length).toBeGreaterThan(1);
    expect(screen.getByText("模型可服务性")).toBeInTheDocument();
    expect(screen.getAllByText("gpt-4.1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("端点").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Chat").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Compact").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Image").length).toBeGreaterThan(0);
    expect(screen.getAllByText("RT WS").length).toBeGreaterThan(0);
    expect(screen.getAllByText("计费").length).toBeGreaterThan(0);
    expect(screen.getAllByText("规则定价").length).toBeGreaterThan(0);
    expect(screen.getAllByText("3/3").length).toBeGreaterThan(0);
    expect(screen.getByText("路由明细")).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1-upstream")).toBeInTheDocument();
    expect(screen.getAllByText("就绪").length).toBeGreaterThan(0);
    expect(screen.getByText("优先修复")).toBeInTheDocument();
    expect(screen.getByTitle("有 2 个账号或资源当前不可路由。")).toHaveAttribute(
      "href",
      "/admin/accounts?view=health",
    );
    expect(screen.getByTitle("有 1 个账号的代理绑定需要处理。")).toHaveAttribute(
      "href",
      "/admin/proxies",
    );
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
    fixes: [
      {
        severity: "critical",
        area: "accounts",
        reason: "no_routable_accounts",
        count: 2,
        href: "/admin/accounts?view=health",
      },
      {
        severity: "warning",
        area: "proxies",
        reason: "proxy_attention",
        count: 1,
        href: "/admin/proxies",
      },
    ],
    rows: [
      {
        provider: provider({ id: "p1", display_name: "OpenAI" }),
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
        proxy_attention_accounts: 0,
        active_model_mappings: 1,
        api_key_count: 1,
        scoped_key_count: 1,
        status: "ready",
        reasons: [],
      },
    ],
    model_rows: [
      {
        model: model({ id: "m1", canonical_name: "gpt-4.1", display_name: "GPT 4.1" }),
        active_providers: 1,
        active_model_mappings: 1,
        routable_accounts: 1,
        endpoints: [
          endpoint("chat_completions", "/v1/chat/completions", 1, 1, "ready"),
          endpoint("responses", "/v1/responses", 1, 1, "ready"),
          endpoint("responses_websocket", "/v1/responses/ws", 0, 1, "blocked", 1),
          endpoint("responses_compact", "/v1/responses/compact", 0, 1, "blocked", 1),
          endpoint("responses_input_items", "/v1/responses/{response_id}/input_items", 0, 1, "blocked", 1),
          endpoint("messages", "/v1/messages", 1, 1, "ready"),
          endpoint("anthropic_count_tokens", "/v1/messages/count_tokens", 0, 1, "blocked", 1),
          endpoint("gemini_generate_content", "/v1beta/models/{model}:generateContent", 0, 1, "blocked", 1),
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
        model: model({ id: "m1", canonical_name: "gpt-4.1", display_name: "GPT 4.1" }),
        provider: provider({ id: "p1", display_name: "OpenAI" }),
        mapping_id: "mapping-1",
        upstream_model: "gpt-4.1-upstream",
        routable_accounts: 1,
        endpoints: [
          endpoint("chat_completions", "/v1/chat/completions", 1, 1, "ready"),
          endpoint("responses", "/v1/responses", 1, 1, "ready"),
          endpoint("responses_websocket", "/v1/responses/ws", 0, 1, "blocked", 1),
          endpoint("responses_compact", "/v1/responses/compact", 0, 1, "blocked", 1),
          endpoint("responses_input_items", "/v1/responses/{response_id}/input_items", 0, 1, "blocked", 1),
          endpoint("messages", "/v1/messages", 1, 1, "ready"),
          endpoint("anthropic_count_tokens", "/v1/messages/count_tokens", 0, 1, "blocked", 1),
          endpoint("gemini_generate_content", "/v1beta/models/{model}:generateContent", 0, 1, "blocked", 1),
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
