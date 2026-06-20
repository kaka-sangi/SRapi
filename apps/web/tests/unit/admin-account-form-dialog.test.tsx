import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AccountFormDialog, type AccountProviderOption } from "@/components/admin/account-form-dialog";
import { LanguageProvider } from "@/context/LanguageContext";

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

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useTlsProfiles: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useTestAccount: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
}));

vi.mock("@/components/admin/account-oauth-authorize-dialog", () => ({
  AccountOAuthAuthorizeDialog: () => null,
}));

const providerOptions: AccountProviderOption[] = [
  {
    value: "codex-provider",
    label: "Codex",
    authMethods: ["cli_client_token"],
    adapterType: "reverse-proxy-codex-cli",
  },
];

const submit = vi.fn();

beforeEach(() => {
  submit.mockReset();
  submit.mockResolvedValue(undefined);
});

function renderDialog(options: AccountProviderOption[] = providerOptions, defaultProviderId = "codex-provider") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LanguageProvider>
        <AccountFormDialog
          open
          mode="create"
          onOpenChange={() => {}}
          providerOptions={options}
          defaultProviderId={defaultProviderId}
          submit={submit}
        />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

describe("AccountFormDialog", () => {
  it("applies provider defaults and auto-generates a name when creating", async () => {
    const user = userEvent.setup({ delay: null });
    renderDialog();

    fireEvent.change(screen.getByLabelText("访问令牌"), {
      target: { value: "codex-clientjwt" },
    });
    await user.click(screen.getByRole("button", { name: "保存" }));

    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        provider_id: "codex-provider",
        name: "codex-lientjwt",
        runtime_class: "cli_client_token",
        upstream_client: "codex_cli",
        credential: { access_token: "codex-clientjwt" },
        metadata: { base_url: "https://chatgpt.com/backend-api/codex" },
      }),
    );
  });

  it("splits pasted sub2api credential JSON into OAuth fields and metadata", async () => {
    const user = userEvent.setup({ delay: null });
    renderDialog([
      {
        value: "codex-oauth-provider",
        label: "Codex OAuth",
        authMethods: ["oauth_refresh"],
        adapterType: "reverse-proxy-codex-cli",
      },
    ], "codex-oauth-provider");

    const pasteData = {
      getData: () =>
        JSON.stringify({
          name: "alice@example.test",
          credentials: {
            access_token: "access-token",
            refresh_token: "refresh-token",
            id_token: "id-token",
            email: "alice@example.test",
            chatgpt_account_id: "workspace-1",
            chatgpt_user_id: "user-a",
            organization_id: "org-a",
            plan_type: "k12",
          },
          extra: {
            source: "sub_bundle_input",
            codex_5h_used_percent: 1,
          },
        }),
    };
    fireEvent.paste(screen.getByLabelText("访问令牌"), {
      clipboardData: pasteData,
    });
    await user.click(screen.getByRole("button", { name: "保存" }));

    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "alice@example.test",
        runtime_class: "oauth_refresh",
        credential: {
          access_token: "access-token",
          refresh_token: "refresh-token",
          id_token: "id-token",
        },
        metadata: expect.objectContaining({
          base_url: "https://chatgpt.com/backend-api/codex",
          email: "alice@example.test",
          chatgpt_account_id: "workspace-1",
          chatgpt_user_id: "user-a",
          organization_id: "org-a",
          plan_type: "k12",
          source: "sub_bundle_input",
          codex_5h_used_percent: 1,
        }),
      }),
    );
  });
});
