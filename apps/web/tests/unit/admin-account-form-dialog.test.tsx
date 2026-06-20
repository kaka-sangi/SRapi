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

function renderDialog() {
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
          providerOptions={providerOptions}
          defaultProviderId="codex-provider"
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
});
