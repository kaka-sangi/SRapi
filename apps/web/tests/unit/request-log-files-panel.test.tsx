import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RequestLogFilesPanel } from "@/app/admin/logs/_panels/request-log-files-panel";
import { LanguageProvider } from "@/context/LanguageContext";
import type { RequestLogFileDescriptor } from "@/lib/admin-api/request-log-files";

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

const mocks = vi.hoisted(() => ({
  file: {
    name: "error-1780000000000-req-dump.log",
    size: 512,
    created_at: "2026-06-18T10:00:00Z",
    request_id: "req-dump",
    is_error_only: true,
  } satisfies RequestLogFileDescriptor,
  deleteFile: vi.fn(),
  downloadFile: vi.fn(),
}));

vi.mock("@/hooks/admin-queries", () => ({
  downloadAdminRequestLogFileText: mocks.downloadFile,
  requestLogFileDownloadQueryKey: (name: string | null) => [
    "admin",
    "request-log-files",
    "download",
    name ?? "",
  ],
  useAdminRequestLogFileDownload: () => ({ data: "", isError: false }),
  useAdminRequestLogFiles: () => ({
    data: {
      data: [mocks.file],
      pagination: { page: 1, page_size: 200, total: 1, has_next: false },
    },
    isFetching: false,
  }),
  useDeleteAdminRequestLogFile: () => ({
    mutateAsync: mocks.deleteFile,
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

describe("RequestLogFilesPanel", () => {
  beforeEach(() => {
    storage.clear();
    mocks.deleteFile.mockReset();
    mocks.downloadFile.mockReset();
  });

  it("links each request dump row to related error and system logs", () => {
    render(<RequestLogFilesPanel />, { wrapper: wrap });

    expect(screen.getByText("req-dump")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "错误日志" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=req-dump",
    );
    expect(screen.getByRole("link", { name: "系统日志" })).toHaveAttribute(
      "href",
      "/admin/ops/system-logs?f_request_id=req-dump",
    );
  });
});
