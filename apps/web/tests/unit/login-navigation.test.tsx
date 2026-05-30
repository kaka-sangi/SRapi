import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Home from "@/app/page";
import { LanguageProvider } from "@/context/LanguageContext";
import { QueryProvider } from "@/providers/query-provider";
import { apiService } from "@/lib/api";

const pushMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock }),
}));

vi.mock("@/lib/api", () => ({
  apiService: {
    getCurrentUser: vi.fn(() => null),
    getRuntimeStatus: vi.fn(() => Promise.resolve({ connected: true, mode: "live" })),
    login: vi.fn(),
  },
}));

const apiServiceMock = vi.mocked(apiService);
const runtimeStatus = {
  connected: true,
  mode: "live" as const,
  apiBaseUrl: "same-origin /api proxy -> http://127.0.0.1:8080",
  checkedAt: "2026-05-24T00:00:00.000Z",
};

describe("login navigation", () => {
  beforeEach(() => {
    localStorage.clear();
    pushMock.mockClear();
    apiServiceMock.getCurrentUser.mockReturnValue(null);
    apiServiceMock.getRuntimeStatus.mockResolvedValue(runtimeStatus);
    apiServiceMock.login.mockReset();
  });

  it("navigates immediately after successful admin login", async () => {
    apiServiceMock.login.mockResolvedValue({
      email: "admin@srapi.local",
      name: "Admin",
      role: "admin",
      balance: "0.00000000",
      currency: "USD",
    });

    render(
      <QueryProvider>
        <LanguageProvider>
          <Home />
        </LanguageProvider>
      </QueryProvider>,
    );

    const submit = await screen.findByRole("button", { name: /^(sign in|登录)$/i });
    await userEvent.type(screen.getByPlaceholderText("operator@srapi.local"), "admin@srapi.local");
    await userEvent.type(screen.getByPlaceholderText("••••••••••••"), "password123");
    await userEvent.click(submit);

    await waitFor(() => expect(apiServiceMock.login).toHaveBeenCalledWith("admin@srapi.local", "password123"));
    expect(pushMock).toHaveBeenCalledWith("/admin/dashboard");
  });
});
