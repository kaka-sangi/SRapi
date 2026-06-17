import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { BackupTab } from "@/app/admin/settings/backup-tab";
import { LanguageProvider } from "@/context/LanguageContext";

// LanguageProvider reads persisted locale from localStorage; jsdom in this
// setup doesn't ship one, so shim a minimal in-memory store (mirrors
// resource-form-suggestions.test.tsx).
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

vi.mock("@/context/ToastContext", () => ({ useToast: () => ({ toast: vi.fn() }) }));

// The backup tab pulls the snapshot history through the admin API. Mock it
// so the test exercises the render path (status badges, kind labels, size
// formatting) without the SDK / network.
const mocks = vi.hoisted(() => ({
  listBackupSnapshots: vi.fn(),
  getConfigSnapshot: vi.fn(),
  importConfigSnapshot: vi.fn(),
  triggerBackupSnapshot: vi.fn(),
  deleteBackupSnapshot: vi.fn(),
}));

vi.mock("@/lib/admin-api", () => ({
  adminApi: {
    listBackupSnapshots: mocks.listBackupSnapshots,
    getConfigSnapshot: mocks.getConfigSnapshot,
    importConfigSnapshot: mocks.importConfigSnapshot,
    triggerBackupSnapshot: mocks.triggerBackupSnapshot,
    deleteBackupSnapshot: mocks.deleteBackupSnapshot,
    downloadBackupSnapshot: vi.fn(),
  },
  adminErrorMessage: (err: unknown) => (err instanceof Error ? err.message : String(err)),
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

describe("BackupTab database backups section", () => {
  beforeEach(() => {
    mocks.listBackupSnapshots.mockReset();
    mocks.getConfigSnapshot.mockReset();
    storage.clear();
    storage.set("srapi_lang", "en");
  });

  it("renders one row per snapshot with the right status badge tone", async () => {
    mocks.listBackupSnapshots.mockResolvedValue({
      data: [
        {
          id: "10",
          kind: "scheduled",
          status: "success",
          started_at: "2026-06-16T09:00:00Z",
          completed_at: "2026-06-16T09:05:00Z",
          size_bytes: 2_500_000,
          sha256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
          file_path: "/srv/backups/srapi-20260616090000.dump",
          error_message: "",
          triggered_by_user_id: 0,
        },
        {
          id: "9",
          kind: "manual",
          status: "failed",
          started_at: "2026-06-15T09:00:00Z",
          completed_at: "2026-06-15T09:02:00Z",
          size_bytes: 0,
          sha256: "",
          file_path: "",
          error_message: "pg_dump: connection refused",
          triggered_by_user_id: 7,
        },
        {
          id: "8",
          kind: "scheduled",
          status: "superseded",
          started_at: "2026-06-01T09:00:00Z",
          completed_at: "2026-06-01T09:04:00Z",
          size_bytes: 1_000_000,
          sha256: "0000000000000000000000000000000000000000000000000000000000000000",
          file_path: "",
          error_message: "",
          triggered_by_user_id: 0,
        },
      ],
      pagination: { total: 3, offset: 0, limit: 50 },
    });
    mocks.getConfigSnapshot.mockResolvedValue({});

    render(<BackupTab />, { wrapper: wrap });
    expect(await screen.findByText("success")).toBeTruthy();
    expect(screen.getByText("failed")).toBeTruthy();
    expect(screen.getByText("superseded")).toBeTruthy();
    // Human-friendly size suffix renders somewhere in the table.
    expect(screen.getByText(/MB/i)).toBeTruthy();
  });

  it("shows the empty state when there are no snapshots", async () => {
    mocks.listBackupSnapshots.mockResolvedValue({
      data: [],
      pagination: { total: 0, offset: 0, limit: 50 },
    });
    mocks.getConfigSnapshot.mockResolvedValue({});

    render(<BackupTab />, { wrapper: wrap });
    expect(await screen.findByText("No backups recorded yet")).toBeTruthy();
  });
});
