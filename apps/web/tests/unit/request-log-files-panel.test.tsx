import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
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
    account_id: "12",
    source_protocol: "openai-compatible",
    source_endpoint: "/v1/chat/completions",
    success: false,
    status_code: 503,
    error_class: "server_bad",
    latency_ms: 891,
    attempt_count: 1,
    response_count: 1,
    has_summary: true,
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
  useAdminRequestLogFileDownload: () => ({
    data: `=== REQUEST INFO ===
Request-ID: req-dump
Account-ID: 12
Source-Protocol: openai-compatible
Source-Endpoint: /v1/chat/completions

=== REQUEST 1 ===
POST https://upstream.invalid/v1/chat/completions

=== RESPONSE 1 ===
Status: 503

=== SUMMARY ===
Success: false
Error-Class: server_bad
Status: 503
Latency-MS: 891
`,
    isError: false,
  }),
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
    expect(screen.getByText("server_bad")).toBeInTheDocument();
    expect(screen.getByText("891ms")).toBeInTheDocument();
    expect(screen.getByText("1 请求 / 1 响应")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "错误日志" })).toHaveAttribute(
      "href",
      "/admin/logs?tab=error&q=req-dump",
    );
    expect(screen.getByRole("link", { name: "系统日志" })).toHaveAttribute(
      "href",
      "/admin/ops/system-logs?f_request_id=req-dump",
    );
  });

  it("shows a compact diagnostic summary in the preview dialog", () => {
    render(<RequestLogFilesPanel />, { wrapper: wrap });

    fireEvent.click(screen.getByRole("button", { name: "预览" }));

    expect(screen.getAllByText("诊断摘要").length).toBeGreaterThan(1);
    expect(screen.getAllByText("失败").length).toBeGreaterThan(1);
    expect(screen.getAllByText("503").length).toBeGreaterThan(0);
    expect(screen.getAllByText("server_bad").length).toBeGreaterThan(1);
    expect(screen.getAllByText("891ms").length).toBeGreaterThan(1);
    expect(screen.getAllByText("1 请求 / 1 响应").length).toBeGreaterThan(1);
    expect(screen.getAllByText("/v1/chat/completions").length).toBeGreaterThan(1);
  });
});
