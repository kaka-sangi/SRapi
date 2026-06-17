import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useApiKeyNameLookup } from "@/hooks/use-api-key-name-lookup";

// useAdminApiKeys is invoked with {page: 1, page_size: 200}; mock the
// underlying admin-api method so the hook resolves without HTTP.
const mocks = vi.hoisted(() => ({
  listAdminApiKeys: vi.fn(),
}));

vi.mock("@/lib/admin-api", () => ({
  adminApi: { listAdminApiKeys: mocks.listAdminApiKeys },
}));

describe("useApiKeyNameLookup", () => {
  beforeEach(() => {
    mocks.listAdminApiKeys.mockReset();
  });

  function wrap(client: QueryClient) {
    return function Wrapper({ children }: PropsWithChildren) {
      return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    };
  }

  it('returns "name (prefix)" for known keys and "#<id>" for unknown', async () => {
    mocks.listAdminApiKeys.mockResolvedValue({
      data: [
        { id: "11", name: "default", prefix: "sk_abc" },
        { id: "12", name: "ci-runner", prefix: "" },
      ],
      pagination: { page: 1, page_size: 200, total: 2, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useApiKeyNameLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    // The point of the iter-57 design: "name (prefix)" so an operator can tell
    // apart two same-named keys from a legacy migration.
    expect(result.current.get("11")).toBe("default (sk_abc)");
    // No prefix → just the name, no trailing " ()".
    expect(result.current.get("12")).toBe("ci-runner");
    // Past the window.
    expect(result.current.get("999")).toBe("#999");
  });

  it("renders an em-dash for null / undefined / empty", async () => {
    mocks.listAdminApiKeys.mockResolvedValue({
      data: [],
      pagination: { page: 1, page_size: 200, total: 0, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useApiKeyNameLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    expect(result.current.get(null)).toBe("—");
    expect(result.current.get(undefined)).toBe("—");
    expect(result.current.get("")).toBe("—");
  });
});
