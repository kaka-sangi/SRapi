"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";
import {
  requestLogFilesApi,
  type RequestLogFileDescriptor,
} from "@/lib/admin-api/request-log-files";

// RequestLogFilesPanel renders the per-request HTTP envelope dumps written
// by the gateway when SRAPI_REQUEST_LOG_ENABLED=true. The dataset is small
// (capped by retention + count) and changes infrequently, so we use a
// lightweight built-in fetch + 30s auto-refresh rather than the
// AdminListView machinery used by the other tabs.
export function RequestLogFilesPanel() {
  const { t } = useLanguage();
  const [items, setItems] = useState<RequestLogFileDescriptor[]>([]);
  const [loading, setLoading] = useState(false);
  const [errorOnly, setErrorOnly] = useState(false);
  const [prefix, setPrefix] = useState("");
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [selected, setSelected] = useState<RequestLogFileDescriptor | null>(null);
  const [previewBody, setPreviewBody] = useState<string>("");
  const [previewError, setPreviewError] = useState<string>("");

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const data = await requestLogFilesApi.list({
        request_id: prefix.trim() || undefined,
        error_only: errorOnly,
        limit: 200,
      });
      setItems(data);
    } catch {
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, [prefix, errorOnly]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (!autoRefresh) return undefined;
    const id = window.setInterval(() => {
      void reload();
    }, 30000);
    return () => window.clearInterval(id);
  }, [autoRefresh, reload]);

  const openPreview = useCallback(async (file: RequestLogFileDescriptor) => {
    setSelected(file);
    setPreviewBody("");
    setPreviewError("");
    try {
      const text = await requestLogFilesApi.download(file.name);
      setPreviewBody(text);
    } catch {
      setPreviewError(t("adminRequestLogFiles.detailLoadFailed"));
    }
  }, [t]);

  const downloadFile = useCallback((file: RequestLogFileDescriptor) => {
    // Trigger a browser download by creating a blob URL from the captured
    // text. We do not rely on Content-Disposition because the admin fetch
    // already wraps the response — going through a blob preserves the
    // expected filename and avoids re-fetching.
    void (async () => {
      try {
        const text = await requestLogFilesApi.download(file.name);
        const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = file.name;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
      } catch {
        /* ignore — operator can click again */
      }
    })();
  }, []);

  const removeFile = useCallback(async (file: RequestLogFileDescriptor) => {
    if (typeof window !== "undefined") {
      if (!window.confirm(t("adminRequestLogFiles.confirmDelete"))) return;
    }
    try {
      await requestLogFilesApi.remove(file.name);
      await reload();
    } catch {
      /* swallow — retain row so the operator can retry */
    }
  }, [reload, t]);

  const formattedRows = useMemo(
    () =>
      items.map((item) => ({
        ...item,
        createdAtLabel: formatDateTime(item.created_at),
        sizeLabel: formatSize(item.size),
      })),
    [items],
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("adminRequestLogFiles.title")}
        description={t("adminRequestLogFiles.subtitle")}
      />

      <div className="flex flex-wrap items-center gap-3 rounded-lg border border-srapi-border-subtle bg-srapi-bg-card p-3">
        <input
          value={prefix}
          onChange={(e) => setPrefix(e.target.value)}
          placeholder={t("adminRequestLogFiles.searchPlaceholder")}
          className="h-8 flex-1 min-w-[180px] rounded border border-srapi-border-subtle bg-srapi-bg-input px-2 text-sm"
        />
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={errorOnly}
            onChange={(e) => setErrorOnly(e.target.checked)}
          />
          {t("adminRequestLogFiles.errorOnly")}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={autoRefresh}
            onChange={(e) => setAutoRefresh(e.target.checked)}
          />
          {t("adminRequestLogFiles.autoRefresh")}
        </label>
      </div>

      {formattedRows.length === 0 ? (
        <div className="rounded-lg border border-srapi-border-subtle bg-srapi-bg-card p-6 text-center text-sm text-srapi-text-tertiary">
          <p className="font-medium text-srapi-text-secondary">
            {t("adminRequestLogFiles.emptyTitle")}
          </p>
          <p>{t("adminRequestLogFiles.emptyBody")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-srapi-border-subtle bg-srapi-bg-card">
          <table className="w-full table-fixed text-left text-sm">
            <thead className="border-b border-srapi-border-subtle bg-srapi-bg-card-elevated">
              <tr>
                <th className="px-3 py-2 font-medium">{t("adminRequestLogFiles.name")}</th>
                <th className="px-3 py-2 font-medium">{t("adminRequestLogFiles.requestId")}</th>
                <th className="px-3 py-2 font-medium">{t("adminRequestLogFiles.createdAt")}</th>
                <th className="px-3 py-2 font-medium">{t("adminRequestLogFiles.size")}</th>
                <th className="w-48 px-3 py-2 font-medium">&nbsp;</th>
              </tr>
            </thead>
            <tbody>
              {formattedRows.map((row) => (
                <tr key={row.name} className="border-t border-srapi-border-subtle">
                  <td className="px-3 py-2 font-mono text-xs">
                    <button
                      type="button"
                      onClick={() => void openPreview(row)}
                      className="underline-offset-2 hover:underline"
                    >
                      {row.name}
                    </button>
                    {row.is_error_only ? (
                      <span className="ml-2 rounded bg-red-500/15 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-red-300">
                        error
                      </span>
                    ) : null}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-srapi-text-secondary">
                    {row.request_id}
                  </td>
                  <td className="px-3 py-2 text-xs text-srapi-text-tertiary">
                    {row.createdAtLabel}
                  </td>
                  <td className="px-3 py-2 text-xs text-srapi-text-tertiary">
                    {row.sizeLabel}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => void openPreview(row)}
                        className="rounded border border-srapi-border-subtle px-2 py-1 text-xs hover:bg-srapi-bg-card-elevated"
                      >
                        {t("adminRequestLogFiles.preview")}
                      </button>
                      <button
                        type="button"
                        onClick={() => downloadFile(row)}
                        className="rounded border border-srapi-border-subtle px-2 py-1 text-xs hover:bg-srapi-bg-card-elevated"
                      >
                        {t("adminRequestLogFiles.download")}
                      </button>
                      <button
                        type="button"
                        onClick={() => void removeFile(row)}
                        className="rounded border border-red-500/30 px-2 py-1 text-xs text-red-300 hover:bg-red-500/10"
                      >
                        {t("adminRequestLogFiles.delete")}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {loading ? (
        <p className="text-xs text-srapi-text-tertiary">…</p>
      ) : null}

      <Dialog open={selected !== null} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t("adminRequestLogFiles.detailTitle")}</DialogTitle>
            <DialogDescription>
              {selected ? (
                <span className="font-mono text-xs">{selected.name}</span>
              ) : null}
            </DialogDescription>
          </DialogHeader>
          {previewError ? (
            <p className="text-sm text-red-300">{previewError}</p>
          ) : (
            <pre className="max-h-[60vh] overflow-auto rounded bg-srapi-bg-input p-3 text-xs">
              {previewBody}
            </pre>
          )}
          {selected ? (
            <div className="flex justify-end">
              <button
                type="button"
                onClick={() => selected && downloadFile(selected)}
                className="rounded border border-srapi-border-subtle px-3 py-1 text-sm hover:bg-srapi-bg-card-elevated"
              >
                {t("adminRequestLogFiles.download")}
              </button>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>
    </div>
  );
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}
