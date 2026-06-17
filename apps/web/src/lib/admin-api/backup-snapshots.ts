"use client";

import {
  deleteAdminBackupSnapshot,
  getAdminBackupSnapshot,
  listAdminBackupSnapshots,
  triggerAdminBackupSnapshot,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  BackupSnapshot,
  BackupSnapshotPagination,
  Id,
  ListAdminBackupSnapshotsData,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { configuredApiBaseUrl, getStoredCSRFToken, unwrapData } from "./_shared";

export interface BackupSnapshotListPage {
  data: BackupSnapshot[];
  pagination: BackupSnapshotPagination;
}

export const backupSnapshotsApi = {
  // Offset-paginated history (newest first). Status filter narrows to
  // running/success/failed/superseded — empty returns all.
  async listBackupSnapshots(
    query?: ListAdminBackupSnapshotsData["query"],
  ): Promise<BackupSnapshotListPage> {
    const response = await listAdminBackupSnapshots({ query, throwOnError: true });
    if (!response.data) {
      throw new Error("Admin backups list returned an empty response.");
    }
    return {
      data: response.data.data,
      pagination: response.data.pagination,
    };
  },

  getBackupSnapshot(id: Id): Promise<BackupSnapshot> {
    return unwrapData(() => getAdminBackupSnapshot({ path: { id }, throwOnError: true }));
  },

  // Synchronously kicks off a real pg_dump run tagged as kind=manual. The
  // server returns the created snapshot row once the file is on disk.
  triggerBackupSnapshot(): Promise<BackupSnapshot> {
    return unwrapData(() => triggerAdminBackupSnapshot({ throwOnError: true }));
  },

  async deleteBackupSnapshot(id: Id): Promise<{ deleted: boolean }> {
    const data = await unwrapData(() =>
      deleteAdminBackupSnapshot({ path: { id }, throwOnError: true }),
    );
    return { deleted: data.deleted };
  },

  // Stream the dump file via the browser's native download flow. The
  // generated SDK only supports JSON envelopes, so we hit the endpoint with
  // a raw fetch and pipe the blob into a one-shot anchor click. Cookies +
  // CSRF header line up with every other admin call.
  async downloadBackupSnapshot(id: Id, fileName?: string): Promise<void> {
    const base = configuredApiBaseUrl();
    const url = `${base}/api/v1/admin/backups/${encodeURIComponent(String(id))}/download`;
    const headers: Record<string, string> = {};
    const csrf = getStoredCSRFToken();
    if (csrf) {
      headers["X-CSRF-Token"] = csrf;
    }
    const response = await fetch(url, {
      method: "GET",
      credentials: "include",
      headers,
    });
    if (!response.ok) {
      throw new Error(`Backup download failed (HTTP ${response.status})`);
    }
    const blob = await response.blob();
    const objectUrl = URL.createObjectURL(blob);
    try {
      const anchor = document.createElement("a");
      anchor.href = objectUrl;
      const disposition = response.headers.get("Content-Disposition") || "";
      const match = disposition.match(/filename\*?=(?:UTF-8'')?"?([^";]+)"?/i);
      anchor.download = fileName || (match && match[1]) || `srapi-backup-${id}.dump`;
      document.body.appendChild(anchor);
      anchor.click();
      document.body.removeChild(anchor);
    } finally {
      URL.revokeObjectURL(objectUrl);
    }
  },
};
