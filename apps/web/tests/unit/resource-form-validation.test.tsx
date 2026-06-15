import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { LanguageProvider } from "@/context/LanguageContext";

vi.mock("@/context/ToastContext", () => ({ useToast: () => ({ toast: vi.fn() }) }));

// LanguageProvider reads persisted locale from localStorage; the happy-dom in
// this setup doesn't provide it, so shim a minimal in-memory store (mirrors the
// suggestions test).
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

// The default locale is "zh", so pin the language to "en" up front; the
// assertions below match the English copy ("Required", "Save").
storage.set("srapi_lang", "en");

type Draft = { headers: Record<string, string> };

function renderDialog(initial: Draft, submit: (body: Draft) => Promise<void>) {
  const fields: FieldConfig<Draft>[] = [
    { name: "headers", label: "Headers", type: "keyvalue", required: true },
  ];
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <LanguageProvider>
        <ResourceFormDialog<Draft, Draft>
          open
          onOpenChange={() => {}}
          title="New resource"
          fields={fields}
          initial={initial}
          buildBody={(d) => d}
          submit={submit}
          successMessage="ok"
        />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

describe("ResourceFormDialog required validation", () => {
  it("blocks submit and surfaces the inline error for an empty {} object value", async () => {
    const user = userEvent.setup();
    const submit = vi.fn(async () => {});
    // The validateFields fix treats `{}` (no keys) as empty for required fields.
    renderDialog({ headers: {} }, submit);

    await user.click(screen.getByRole("button", { name: "Save" }));

    // Inline required error renders under the field, and submit never runs.
    const msg = await screen.findByText("Required");
    expect(msg).toBeInTheDocument();
    expect(msg.closest("#field-headers-msg")).not.toBeNull();
    expect(submit).not.toHaveBeenCalled();
  });

  it("passes validation and submits when the object value is non-empty", async () => {
    const user = userEvent.setup();
    const submit = vi.fn(async () => {});
    renderDialog({ headers: { Authorization: "Bearer xyz" } }, submit);

    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
    expect(submit).toHaveBeenCalledWith({ headers: { Authorization: "Bearer xyz" } });
    // No inline required error when the value is present.
    expect(screen.queryByText("Required")).toBeNull();
  });
});
