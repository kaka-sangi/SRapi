import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import GatewayPoliciesAdminPage from "@/app/admin/gateway-policies/page";
import { LanguageProvider } from "@/context/LanguageContext";
import type { TlsProfile } from "@/lib/sdk-types";

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  searchParams: new URLSearchParams("tab=tls-profiles"),
  updateTlsProfile: vi.fn(),
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
  useErrorPassthroughRules: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useCreateErrorPassthroughRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateErrorPassthroughRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteErrorPassthroughRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  usePayloadRules: () => ({ data: { data: [] }, isLoading: false, isError: false }),
  useCreatePayloadRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdatePayloadRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeletePayloadRule: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useTlsProfiles: () => ({
    data: { data: [tlsProfile({ id: 1, enabled: true })] },
    isLoading: false,
    isError: false,
  }),
  useCreateTlsProfile: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateTlsProfile: () => ({ mutateAsync: mocks.updateTlsProfile, isPending: false }),
  useDeleteTlsProfile: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

describe("GatewayPoliciesAdminPage", () => {
  beforeEach(() => {
    mocks.replace.mockReset();
    mocks.updateTlsProfile.mockReset();
    mocks.updateTlsProfile.mockResolvedValue(tlsProfile({ id: 1, enabled: false }));
    mocks.toast.mockReset();
    mocks.searchParams = new URLSearchParams("tab=tls-profiles");
    storage.clear();
    storage.set("srapi_lang", "zh");
    window.history.replaceState(null, "", "/admin/gateway-policies?tab=tls-profiles");
  });

  it("toggles TLS profiles inline without opening the edit form", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("button", { name: "停用档案" }));

    await waitFor(() => {
      expect(mocks.updateTlsProfile).toHaveBeenCalledWith({
        id: "1",
        body: { enabled: false },
      });
    });
    expect(mocks.toast).toHaveBeenCalledWith({ title: "TLS 档案已停用", tone: "success" });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
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
        <GatewayPoliciesAdminPage />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

function tlsProfile(overrides: Partial<TlsProfile> = {}): TlsProfile {
  return {
    id: 1,
    name: "chrome-egress",
    tls_template: "chrome_auto",
    http_version_policy: "prefer_h2",
    user_agent: "SRapi Test UA",
    extra_headers: {},
    enabled: true,
    created_at: "2026-06-20T00:00:00Z",
    updated_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}
