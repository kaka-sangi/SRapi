import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../../packages/sdk/typescript/src/client.gen", () => ({
  client: {
    setConfig: vi.fn(),
  },
}));

vi.mock("../../../../packages/sdk/typescript/src/index", () => ({
  login: vi.fn(),
  logout: vi.fn(),
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  updateApiKey: vi.fn(),
  listAdminAccounts: vi.fn(),
  getAdminOverview: vi.fn(),
  listAdminUsageLogs: vi.fn(() =>
    Promise.resolve({
      data: {
        data: [
          { model: "gpt-4o-mini", success: true, source_endpoint: "/v1/chat/completions" },
          { model: "gpt-4o-mini", success: true, source_endpoint: "/v1/responses" },
          { model: "gpt-4o-mini", success: true, source_endpoint: "/v1/messages" },
        ],
      },
    }),
  ),
  listAdminSchedulerDecisions: vi.fn(() =>
    Promise.resolve({
      data: { data: [] },
    }),
  ),
  listAdminOpsSlos: vi.fn(),
}));

describe("apiService.getSmokeStatus", () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem("srapi_user", JSON.stringify({ role: "admin" }));

    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/srapi-health")) {
          return Promise.resolve(new Response("{}", { status: 200 }));
        }
        if (url.includes("/api/v1/admin/models")) {
          return Promise.resolve(
            Response.json({ data: [{ canonical_name: "gpt-4o-mini" }] }),
          );
        }
        if (url.includes("/api/v1/admin/accounts")) {
          return Promise.resolve(
            Response.json({
              data: [
                {
                  id: "1",
                  metadata: { base_url: "http://127.0.0.1:8080/mock" },
                },
              ],
            }),
          );
        }
        return Promise.resolve(new Response("{}", { status: 404 }));
      }),
    );
  });

  it("passes local gateway smoke when required endpoints have successful usage", async () => {
    const { apiService } = await import("@/lib/api");

    const status = await apiService.getSmokeStatus();

    expect(status.model_exists).toBe(true);
    expect(status.active_account_count).toBe(1);
    expect(status.public_https_upstream_account_count).toBe(0);
    expect(status.missing_usage_endpoints).toEqual([]);
    expect(status.v0_1_smoke_evidence_complete).toBe(true);
  });
});
