import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";

// useAdminUsers wraps the listUsers SDK call; mock it directly so we don't
// need to spin up an HTTP layer or auth flow. The hook calls it with
// {page: 1, page_size: 200} — we don't assert the call args here, just the
// downstream get(id) / map / query exposed to consumers.
const mocks = vi.hoisted(() => ({
  listUsers: vi.fn(),
}));

vi.mock("@/lib/admin-api", () => ({
  adminApi: { listUsers: mocks.listUsers },
}));

describe("useUserEmailLookup", () => {
  beforeEach(() => {
    mocks.listUsers.mockReset();
  });

  function wrap(client: QueryClient) {
    return function Wrapper({ children }: PropsWithChildren) {
      return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    };
  }

  it("falls back to the stringified id when the user is past the lookup window", async () => {
    mocks.listUsers.mockResolvedValue({
      data: [{ id: "1", email: "alice@example.com" }],
      pagination: { page: 1, page_size: 200, total: 1, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useUserEmailLookup(), { wrapper: wrap(client) });

    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    expect(result.current.get("1")).toBe("alice@example.com");
    // Unknown id (past the page-size-200 window) — falls back to the id itself,
    // never blank, never undefined. Caller decides how to render.
    expect(result.current.get("99999")).toBe("99999");
  });

  it("renders an em-dash for null / undefined / empty string", async () => {
    mocks.listUsers.mockResolvedValue({
      data: [],
      pagination: { page: 1, page_size: 200, total: 0, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useUserEmailLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    // The dialog rendering surfaces use these directly — empty inputs must
    // come out as the "—" placeholder, not the literal string "null".
    expect(result.current.get(null)).toBe("—");
    expect(result.current.get(undefined)).toBe("—");
    expect(result.current.get("")).toBe("—");
  });

  it("accepts both number and string ids interchangeably", async () => {
    mocks.listUsers.mockResolvedValue({
      data: [{ id: "42", email: "bob@example.com" }],
      pagination: { page: 1, page_size: 200, total: 1, has_next: false },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useUserEmailLookup(), { wrapper: wrap(client) });
    await waitFor(() => expect(result.current.query.isSuccess).toBe(true));

    // Some surfaces pass numbers (e.g. ErrorLog.user_id is a number on the
    // wire), others pass strings (e.g. PaymentOrder.user_id arrives as Id =
    // string). Both must hit.
    expect(result.current.get(42)).toBe("bob@example.com");
    expect(result.current.get("42")).toBe("bob@example.com");
    expect(result.current.map.get("42")).toBe("bob@example.com");
  });
});
