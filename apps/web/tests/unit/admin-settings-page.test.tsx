import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminSettingsPage from "@/app/admin/settings/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type {
  AdminSettings,
  CaptchaSettings,
  CaptchaSettingsWritable,
} from "../../../../packages/sdk/typescript/src/types.gen";

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  searchParams: new URLSearchParams("tab=gateway"),
  updateSettings: vi.fn(),
  captchaSettings: {
    managed: false,
    enabled: false,
    provider: "turnstile",
    site_key: "",
    secret_key_configured: false,
    verify_url: "",
  } as CaptchaSettings,
  updateCaptchaSettings: vi.fn(),
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

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mocks.replace }),
  useSearchParams: () => mocks.searchParams,
}));

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useAdminSettings: () => ({
    data: settings(),
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useAdminCaptchaSettings: () => ({
    data: mocks.captchaSettings,
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useUpdateSettings: () => ({
    mutateAsync: mocks.updateSettings,
    isPending: false,
  }),
  useUpdateCaptchaSettings: () => ({
    mutateAsync: mocks.updateCaptchaSettings,
    isPending: false,
  }),
  useAdminModels: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
}));

describe("AdminSettingsPage", () => {
  beforeEach(() => {
    mocks.replace.mockReset();
    mocks.updateSettings.mockReset();
    mocks.updateCaptchaSettings.mockReset();
    mocks.toast.mockReset();
    mocks.searchParams = new URLSearchParams("tab=gateway");
    mocks.captchaSettings = {
      managed: false,
      enabled: false,
      provider: "turnstile",
      site_key: "",
      secret_key_configured: false,
      verify_url: "",
    } as CaptchaSettings;
    storage.clear();
    storage.set("srapi_lang", "zh");
    window.history.replaceState(null, "", "/admin/settings?tab=gateway");
  });

  it("opens the gateway settings tab from the tab query parameter", () => {
    renderPage();

    expect(screen.getByRole("tab", { name: "网关" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("协议转换开关")).toBeInTheDocument();
    expect(screen.getByText("Responses → Messages")).toBeInTheDocument();
  });

  it("keeps tab changes in the settings URL", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("tab", { name: "安全" }));

    expect(mocks.replace).toHaveBeenCalledWith("/admin/settings?tab=security", {
      scroll: false,
    });
  });

  it("edits custom sidebar links without raw JSON", async () => {
    const user = userEvent.setup();
    mocks.searchParams = new URLSearchParams("");
    mocks.updateSettings.mockImplementation(async (body: AdminSettings) => body);
    renderPage();

    await user.click(screen.getByRole("button", { name: "添加链接" }));
    await user.type(screen.getByLabelText("名称"), "Docs");
    await user.type(screen.getByLabelText("URL"), "https://docs.example.com");
    await user.type(screen.getByLabelText("ID"), "docs");

    await user.click(screen.getByRole("button", { name: "保存更改" }));

    await waitFor(() => expect(mocks.updateSettings).toHaveBeenCalled());
    const submitted = mocks.updateSettings.mock.calls[0][0] as AdminSettings;
    expect(submitted.general.custom_menus).toEqual([
      {
        id: "docs",
        label: "Docs",
        url: "https://docs.example.com",
        visibility: "user",
        sort_order: 0,
      },
    ]);
  });

  it("edits public site branding fields", async () => {
    const user = userEvent.setup();
    mocks.searchParams = new URLSearchParams("");
    mocks.updateSettings.mockImplementation(async (body: AdminSettings) => body);
    renderPage();

    await user.clear(screen.getByLabelText("站点副标题"));
    await user.type(screen.getByLabelText("站点副标题"), "Private AI workspace");
    await user.type(screen.getByLabelText("联系方式"), "support@example.com");
    await user.type(screen.getByLabelText("文档链接"), "https://docs.example.com");
    await user.click(screen.getByRole("button", { name: "保存更改" }));

    await waitFor(() => expect(mocks.updateSettings).toHaveBeenCalled());
    const submitted = mocks.updateSettings.mock.calls[0][0] as AdminSettings;
    expect(submitted.general).toMatchObject({
      site_subtitle: "Private AI workspace",
      contact_info: "support@example.com",
      doc_url: "https://docs.example.com",
    });
  });

  it("edits security allowlist and non-secret oauth provider configs", async () => {
    const user = userEvent.setup();
    mocks.searchParams = new URLSearchParams("tab=security");
    mocks.updateSettings.mockImplementation(async (body: AdminSettings) => ({
      ...body,
      security: {
        ...body.security,
        registration_email_suffix_allowlist: ["@example.com"],
        oauth_provider_configs: [
          {
            provider: "oidc",
            provider_key: "issuer-main",
            display_name: "Main issuer",
            client_id: "client-123",
            authorize_url: "https://idp.example/authorize",
            token_url: "https://idp.example/token",
            userinfo_url: "https://idp.example/userinfo",
            token_auth_method: "none",
            redirect_uri: "https://api.example/api/v1/auth/oauth/oidc/callback",
            scopes: ["openid", "email"],
          },
        ],
      },
    }));
    renderPage();

    await user.type(screen.getByLabelText("注册邮箱后缀白名单"), "@example.com{Enter}");
    await user.click(screen.getByRole("button", { name: "添加提供方配置" }));

    await user.type(screen.getByLabelText("实例标识"), "issuer-main");
    await user.type(screen.getByLabelText("显示名称"), "Main issuer");
    await user.type(screen.getByLabelText("Client ID"), "client-123");
    await user.type(screen.getByLabelText("授权 URL"), "https://idp.example/authorize");
    await user.type(
      screen.getByLabelText("回调 URI"),
      "https://api.example/api/v1/auth/oauth/oidc/callback",
    );
    await user.type(screen.getByLabelText("Token URL"), "https://idp.example/token");
    await user.type(screen.getByLabelText("UserInfo URL"), "https://idp.example/userinfo");
    await user.type(screen.getByPlaceholderText("openid, email, profile"), "openid email{Enter}");

    await user.click(screen.getByRole("button", { name: "保存更改" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByRole("textbox"), "SAVE SECURITY SETTINGS");
    await user.click(within(dialog).getByRole("button", { name: "保存" }));

    await waitFor(() => expect(mocks.updateSettings).toHaveBeenCalled());
    const submitted = mocks.updateSettings.mock.calls[0][0] as AdminSettings;
    expect(submitted.security.registration_email_suffix_allowlist).toEqual(["@example.com"]);
    expect(submitted.security.oauth_provider_configs[0]).toMatchObject({
      provider: "oidc",
      provider_key: "issuer-main",
      client_id: "client-123",
      token_auth_method: "none",
      scopes: ["openid", "email"],
    });
    expect(await screen.findByText("已保存")).toBeInTheDocument();
  });

  it("edits captcha runtime settings through the dedicated security control", async () => {
    const user = userEvent.setup();
    mocks.searchParams = new URLSearchParams("tab=security");
    mocks.updateCaptchaSettings.mockImplementation(async (body: CaptchaSettingsWritable) => ({
      managed: body.managed,
      enabled: body.enabled,
      provider: body.provider,
      site_key: body.site_key,
      secret_key_configured: Boolean(body.secret_key),
      verify_url: body.verify_url,
    }));
    renderPage();

    expect(screen.getByText("CAPTCHA")).toBeInTheDocument();
    await user.click(screen.getByLabelText("由控制台托管"));
    await user.click(screen.getByLabelText("要求验证"));
    await user.type(screen.getByLabelText("Site key"), "site-123");
    await user.type(screen.getByLabelText("Secret key"), "secret-123");
    await user.type(screen.getByLabelText("Verify URL"), "https://captcha.example/siteverify");

    expect(screen.getByText("CAPTCHA 有未保存更改")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "保存 CAPTCHA" }));

    await waitFor(() => expect(mocks.updateCaptchaSettings).toHaveBeenCalled());
    expect(mocks.updateCaptchaSettings).toHaveBeenCalledWith({
      managed: true,
      enabled: true,
      provider: "turnstile",
      site_key: "site-123",
      secret_key: "secret-123",
      verify_url: "https://captcha.example/siteverify",
    });
    expect(mocks.updateSettings).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByLabelText("Secret key")).toHaveValue(""));
    expect(screen.getByText("CAPTCHA 已保存")).toBeInTheDocument();
  });

  it("uses the normalized settings returned by the server after save", async () => {
    const user = userEvent.setup();
    mocks.updateSettings.mockImplementation(async (body: AdminSettings) => ({
      ...body,
      gateway: {
        ...body.gateway,
        overload_cooldown_seconds: 75,
      },
    }));
    renderPage();

    const cooldown = screen.getByLabelText("过载冷却（秒）");
    await user.clear(cooldown);
    await user.type(cooldown, "61");
    expect(screen.getByText("有未保存更改")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "保存更改" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByRole("textbox"), "SAVE GATEWAY SETTINGS");
    await user.click(within(dialog).getByRole("button", { name: "保存" }));

    await waitFor(() => expect(cooldown).toHaveValue(75));
    expect(screen.getByText("已保存")).toBeInTheDocument();
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
        <AdminSettingsPage />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

function settings(): AdminSettings {
  return {
    general: {
      site_name: "SRapi",
      site_subtitle: "AI gateway control plane",
      logo_url: "",
      version_label: "",
      contact_info: "",
      doc_url: "",
      custom_menus: [],
    },
    agreement: {
      user_agreement: "",
      privacy_policy: "",
    },
    features: {
      enabled_channels: [],
      channel_monitoring_enabled: true,
      invitation_rebate_enabled: false,
      payments_enabled: false,
    },
    security: {
      admin_api_key: { configured: true },
      registration_enabled: true,
      registration_email_suffix_allowlist: [],
      oauth_enabled: false,
      oauth_providers: [],
      oauth_provider_configs: [],
    },
    users: {
      default_balance: "0",
      default_group: "default",
      user_self_delete_enabled: false,
      rpm_limit_default: 0,
    },
    gateway: {
      overload_cooldown_seconds: 60,
      rate_limit_cooldown_seconds: 30,
      stream_timeout_seconds: 300,
      request_shaper_enabled: false,
      scheduler_strategy_rollout_enabled: false,
      scheduler_strategy_shadow_strategy: "balanced",
      scheduler_strategy_rollout_percent: 0,
      scheduler_strategy_rollout_models: [],
      scheduler_strategy_rollout_api_key_hashes: [],
      protocol_conversion_routes: [
        "chat_completions_to_responses",
        "responses_to_chat_completions",
        "responses_to_messages",
      ],
      retry_count: 3,
      max_retry_credentials: 0,
      max_retry_interval_ms: 5000,
      passthrough_upstream_headers: false,
      passthrough_header_allowlist: [],
    },
    payment: {
      enabled: false,
      providers: [],
      subscription_plans_enabled: false,
    },
    email: {
      smtp_configured: false,
      templates: {},
    },
    backup: {
      enabled: false,
      retention_days: 7,
    },
    copilot: {
      enabled: false,
      source: "account",
      provider_account_id: 0,
      model: "",
      models: [],
      dedicated_protocol: "openai-compatible",
      dedicated_base_url: "",
      dedicated_api_key_configured: false,
      owner_only: true,
      auto_run_reads: true,
      web_search_enabled: false,
      web_search_provider: "tavily",
      web_search_base_url: "",
      web_search_api_key_configured: false,
    },
    maintenance: {
      enabled: false,
      message: "",
    },
  };
}
