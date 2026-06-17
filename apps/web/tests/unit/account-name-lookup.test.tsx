import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";

// useAdminAccounts(page=1, page_size=200) — mock only what the hook touches.
const mocks = vi.hoisted(() => ({
  listAccounts: vi.fn(),
}));

vi.mock("@/lib/admin-api", () => ({
  adminApi: { listAccounts: mocks.listAccounts },
}));

describe("useAccountNameLookup", () => {
  beforeEach(() => {
    mocks.listAccounts.mockReset();
  });

  function wrap(client: QueryClient) {
    return function Wrapper({ children }: PropsWithChildren) {
      return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    };
  }

  it("returns the account name and falls back to #<id> past the window", async () => {
    mocks.listAccounts.mockResolvedValue({
      data: [{ id: "7", name: "primary-openai" }],
      pagination: { page: 1, page_size: 200, total: 1, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useAccountNameLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    expect(result.current.get("7")).toBe("primary-openai");
    // Crucial: the fallback for this hook is "#<id>" (matches diagnostics
    // page convention), NOT bare id. Differs from useUserEmailLookup.
    expect(result.current.get("999")).toBe("#999");
  });

  it("renders an em-dash for null / undefined / empty", async () => {
    mocks.listAccounts.mockResolvedValue({
      data: [],
      pagination: { page: 1, page_size: 200, total: 0, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useAccountNameLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    expect(result.current.get(null)).toBe("—");
    expect(result.current.get(undefined)).toBe("—");
    expect(result.current.get("")).toBe("—");
  });

  it("accepts number ids too — UsageLog.account_id is *int on the wire", async () => {
    mocks.listAccounts.mockResolvedValue({
      data: [{ id: "42", name: "backup-anthropic" }],
      pagination: { page: 1, page_size: 200, total: 1, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useAccountNameLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    expect(result.current.get(42)).toBe("backup-anthropic");
    expect(result.current.get("42")).toBe("backup-anthropic");
  });
});
