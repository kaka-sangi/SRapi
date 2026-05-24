import { beforeEach, describe, expect, it, vi } from "vitest";

const getCurrentUserMock = vi.fn();

vi.mock("../../../../packages/sdk/typescript/src/client.gen", () => ({
  client: {
    setConfig: vi.fn(),
  },
}));

vi.mock("../../../../packages/sdk/typescript/src/index", () => ({
  login: vi.fn(),
  logout: vi.fn(),
  getCurrentUser: getCurrentUserMock,
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  updateApiKey: vi.fn(),
  listAdminAccounts: vi.fn(),
  testAdminAccount: vi.fn(),
  getAdminOverview: vi.fn(),
  listAdminUsageLogs: vi.fn(),
  listAdminSchedulerDecisions: vi.fn(),
  listAdminOpsSlos: vi.fn(),
}));

describe("apiService.getLiveCurrentUser", () => {
  beforeEach(() => {
    localStorage.clear();
    getCurrentUserMock.mockReset();
  });

  it("preserves account balance, currency, and RPM limit from the live API", async () => {
    getCurrentUserMock.mockResolvedValue({
      data: {
        data: {
          id: "42",
          email: "user@srapi.local",
          name: "SRapi User",
          roles: ["user"],
          balance: "12.34560000",
          currency: "USD",
          rpm_limit: 120,
          last_login_at: "2026-05-24T00:00:00Z",
          created_at: "2026-05-23T00:00:00Z",
        },
      },
    });

    const { apiService } = await import("@/lib/api");

    const user = await apiService.getLiveCurrentUser();

    expect(user).toMatchObject({
      id: "42",
      email: "user@srapi.local",
      name: "SRapi User",
      role: "user",
      balance: "12.34560000",
      currency: "USD",
      rpm_limit: 120,
    });
    expect(JSON.parse(localStorage.getItem("srapi_user") || "{}")).toMatchObject({
      balance: "12.34560000",
      currency: "USD",
      rpm_limit: 120,
    });
  });
});
