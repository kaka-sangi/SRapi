import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { describe, expect, it, vi } from "vitest";
import LandingPage from "@/app/page";
import { LanguageProvider } from "@/context/LanguageContext";

vi.mock("@/components/visual/ambient-canvas", () => ({
  AmbientCanvas: () => null,
}));

vi.mock("@/components/auth/first-run-redirect", () => ({
  FirstRunRedirect: () => null,
}));

vi.mock("@/components/auth/login-form", () => ({
  LoginForm: () => <div>Login form</div>,
}));

vi.mock("@/components/layout/theme-toggle", () => ({
  ThemeToggle: () => <button type="button">theme</button>,
}));

vi.mock("@/components/layout/language-toggle", () => ({
  LanguageToggle: () => <button type="button">language</button>,
}));

vi.mock("@/hooks/queries", () => ({
  useSiteConfig: () => ({
    data: {
      site_name: "Operator Gateway",
      site_subtitle: "Private AI workspace",
      logo_url: "",
      version_label: "v2026.06",
      contact_info: "support@example.com",
      doc_url: "https://docs.example.com",
      custom_menus: [],
      user_agreement: "",
      privacy_policy: "",
    },
  }),
}));

const storage = new Map<string, string>([["srapi_lang", "en"]]);
Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
  },
});

describe("LandingPage", () => {
  it("renders public site branding from site config", () => {
    render(<LandingPage />, { wrapper: wrap });

    expect(screen.getByText("Operator Gateway")).toBeInTheDocument();
    expect(screen.getByText("v2026.06")).toBeInTheDocument();
    expect(screen.getByText("Private AI workspace")).toBeInTheDocument();
    expect(screen.getByText("support@example.com")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Docs" })).toHaveAttribute("href", "https://docs.example.com");
  });
});

function wrap({ children }: PropsWithChildren) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
  return (
    <QueryClientProvider client={client}>
      <LanguageProvider>{children}</LanguageProvider>
    </QueryClientProvider>
  );
}
