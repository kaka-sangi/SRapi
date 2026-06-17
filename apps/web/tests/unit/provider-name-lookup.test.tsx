import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";

const mocks = vi.hoisted(() => ({
  listProviders: vi.fn(),
}));

vi.mock("@/lib/admin-api", () => ({
  adminApi: { listProviders: mocks.listProviders },
}));

describe("useProviderNameLookup", () => {
  beforeEach(() => {
    mocks.listProviders.mockReset();
  });

  function wrap(client: QueryClient) {
    return function Wrapper({ children }: PropsWithChildren) {
      return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    };
  }

  it("prefers display_name and falls back to slug name", async () => {
    mocks.listProviders.mockResolvedValue({
      data: [
        { id: "3", name: "openai-compatible", display_name: "OpenAI Compatible" },
        // slug-only — early-bootstrap providers ship without display_name.
        { id: "4", name: "anthropic", display_name: "" },
      ],
      pagination: { page: 1, page_size: 200, total: 2, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useProviderNameLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    expect(result.current.get("3")).toBe("OpenAI Compatible");
    // Blank display_name → fall through to the slug name. Operators have
    // historically named providers like "anthropic" so a slug is recognisable.
    expect(result.current.get("4")).toBe("anthropic");
    // Past-window fallback (same shape as iter-56's account hook).
    expect(result.current.get("999")).toBe("#999");
  });

  it("renders an em-dash for null / undefined / empty", async () => {
    mocks.listProviders.mockResolvedValue({
      data: [],
      pagination: { page: 1, page_size: 200, total: 0, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useProviderNameLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    expect(result.current.get(null)).toBe("—");
    expect(result.current.get(undefined)).toBe("—");
    expect(result.current.get("")).toBe("—");
  });
});
