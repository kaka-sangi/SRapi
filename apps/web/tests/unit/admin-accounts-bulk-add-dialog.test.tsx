import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { BulkAddAccountsDialog } from "@/app/admin/accounts/bulk-add-dialog";
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

const batchMutateAsync = vi.fn();

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useAdminProviders: () => ({
    data: {
      data: [
        { id: "1", name: "openai", display_name: "OpenAI" },
        { id: "2", name: "anthropic", display_name: "Anthropic" },
      ],
    },
    isLoading: false,
    isError: false,
  }),
  useAdminGroups: () => ({
    data: { data: [{ id: "10", name: "group-a" }] },
    isLoading: false,
    isError: false,
  }),
  useAdminProxies: () => ({
    data: { data: [] },
    isLoading: false,
    isError: false,
  }),
  useBatchCreateAccounts: () => ({
    mutateAsync: batchMutateAsync,
    isPending: false,
    reset: vi.fn(),
  }),
}));

beforeEach(() => {
  batchMutateAsync.mockReset();
});

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LanguageProvider>
        <BulkAddAccountsDialog open onOpenChange={() => {}} defaultProviderId="1" />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

describe("BulkAddAccountsDialog", () => {
  it("renders empty state with submit disabled until items are pasted", () => {
    renderDialog();
    const submit = screen.getByTestId("bulk-submit");
    expect(submit).toBeDisabled();
    expect(screen.getByTestId("bulk-counts")).toHaveTextContent(/0/);
  });

  it("parses name,api_key lines and updates the live valid count", () => {
    renderDialog();
    const textarea = screen.getByTestId("bulk-items-textarea") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "alpha,sk-a1\nbeta,sk-b2\nsk-c3" } });
    const counts = screen.getByTestId("bulk-counts");
    expect(counts.textContent ?? "").toMatch(/3/);
    expect(screen.getByTestId("bulk-submit")).not.toBeDisabled();
  });

  it("renders a mixed success / failure result panel after submit", async () => {
    const user = userEvent.setup({ delay: null });
    batchMutateAsync.mockResolvedValueOnce({
      results: [
        { index: 0, name: "ok-row", account_id: "123" },
        { index: 1, name: "bad-row", error: "duplicate name" },
      ],
      succeeded: 1,
      failed: 1,
    });
    renderDialog();
    const textarea = screen.getByTestId("bulk-items-textarea") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "ok-row,sk-ok\nbad-row,sk-bad" } });
    await user.click(screen.getByTestId("bulk-submit"));

    const list = await screen.findByTestId("bulk-result-list");
    const okRow = within(list).getByText(/ok-row/);
    expect(okRow).toBeInTheDocument();
    expect(within(list).getByText(/duplicate name/)).toBeInTheDocument();
    // Retry button visible because there is at least one failed row. The
    // default LanguageProvider locale is en, so match the English label.
    expect(screen.getByRole("button", { name: /重试失败行/ })).toBeInTheDocument();
  });

  it("strips comment lines when the toggle is on", () => {
    renderDialog();
    const textarea = screen.getByTestId("bulk-items-textarea") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "# comment\nactive,sk-1" } });
    const counts = screen.getByTestId("bulk-counts");
    expect(counts.textContent ?? "").toMatch(/1/);
  });
});
