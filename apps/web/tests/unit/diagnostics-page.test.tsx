import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AdminDiagnosticsPage from "@/app/admin/ops/diagnostics/page";
import { LanguageProvider } from "@/context/LanguageContext";

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

vi.mock("@/components/layout/admin-shell", () => ({
  AdminShell: ({ children }: PropsWithChildren) => <>{children}</>,
}));

vi.mock("@/context/ToastContext", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/use-account-name-lookup", () => ({
  useAccountNameLookup: () => ({ get: (id?: string | number | null) => `account-${id}` }),
}));

vi.mock("@/hooks/use-admin-events", () => ({
  useAdminEventStream: () => ({ connected: true }),
}));

vi.mock("@/hooks/admin-queries", () => ({
  useAdminCircuitBreakers: () => ({
    data: [
      {
        account_id: 1,
        state: "open",
        requests: 20,
        total_successes: 5,
        total_failures: 15,
        consecutive_successes: 0,
        consecutive_failures: 6,
        success_rate: 0.25,
      },
    ],
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
  useResetCircuitBreaker: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useAdminCacheStats: () => ({
    data: [
      {
        name: "models",
        hits: 10,
        misses: 30,
        evictions: 8,
        size: 12,
        hit_rate: "25%",
      },
    ],
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
  useClearCache: () => ({
    mutateAsync: vi.fn().mockResolvedValue({ cleared: 4 }),
    isPending: false,
  }),
}));

function wrap({ children }: PropsWithChildren) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LanguageProvider>{children}</LanguageProvider>
    </QueryClientProvider>
  );
}

describe("AdminDiagnosticsPage", () => {
  beforeEach(() => {
    storage.clear();
  });

  it("shows compact breaker and cache diagnostics", () => {
    render(<AdminDiagnosticsPage />, { wrapper: wrap });

    expect(screen.getByText("account-1")).toBeInTheDocument();
    expect(screen.getByText("断开")).toBeInTheDocument();
    expect(screen.getByText("已熔断")).toBeInTheDocument();
    expect(screen.getByText(/req:20 ok:5 fail:15 streak:\+0\/-6/)).toBeInTheDocument();
    expect(screen.getByText("models")).toBeInTheDocument();
    expect(screen.getByText("缓存抖动")).toBeInTheDocument();
    expect(screen.getByText(/size:12 hits:10 misses:30 evictions:8/)).toBeInTheDocument();
  });
});
