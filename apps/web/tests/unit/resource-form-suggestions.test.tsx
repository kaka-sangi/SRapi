import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { LanguageProvider } from "@/context/LanguageContext";

vi.mock("@/context/ToastContext", () => ({ useToast: () => ({ toast: vi.fn() }) }));

// LanguageProvider reads persisted locale from localStorage; jsdom in this setup
// doesn't provide it, so shim a minimal in-memory store (mirrors the accounts test).
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

type Draft = { modelName: string };

function renderDialog(fields: FieldConfig<Draft>[]) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <LanguageProvider>
        <ResourceFormDialog<Draft, Draft>
          open
          onOpenChange={() => {}}
          title="New model"
          fields={fields}
          initial={{ modelName: "" }}
          buildBody={(d) => d}
          submit={async () => {}}
          successMessage="ok"
        />
      </LanguageProvider>
    </QueryClientProvider>,
  );
}

describe("ResourceFormDialog suggestions", () => {
  it("renders a <datalist> of options when a field provides suggestions", () => {
    renderDialog([{ name: "modelName", label: "Model name", suggestions: ["gpt-4o", "claude-sonnet-4-6"] }]);
    const datalist = document.querySelector("datalist");
    expect(datalist).not.toBeNull();
    const values = Array.from(datalist!.querySelectorAll("option")).map((o) => o.value);
    expect(values).toEqual(["gpt-4o", "claude-sonnet-4-6"]);
    // The input must point at the datalist via its `list` attribute.
    const input = document.getElementById("field-modelName") as HTMLInputElement | null;
    expect(input?.getAttribute("list")).toBe(datalist!.id);
  });

  it("renders no <datalist> for a plain text field without suggestions", () => {
    renderDialog([{ name: "modelName", label: "Model name" }]);
    expect(document.querySelector("datalist")).toBeNull();
  });
});
