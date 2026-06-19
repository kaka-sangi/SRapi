"use client";

import {
  getAdminOpsErrorLog,
  listAdminOpsErrorLogFingerprints,
  listAdminOpsErrorLogs,
  updateAdminOpsErrorLogResolution,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  ListAdminOpsErrorLogFingerprintsData,
  ListAdminOpsErrorLogsData,
  OpsErrorFingerprint,
  OpsErrorFingerprintListMeta,
  OpsErrorLog,
  OpsErrorLogResolutionUpdate,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapList, unwrapData } from "./_shared";
import type { AdminListResult } from "./types";

export const errorLogsApi = {
  // Server-side paginated/filtered AdminOps upstream-failure evidence feed.
  // The legacy /admin/error-logs usage-derived endpoint is intentionally not
  // used here; operators need durable ops_error_logs rows, not inferred rows.
  listErrorLogs(query?: ListAdminOpsErrorLogsData["query"]): Promise<AdminListResult<OpsErrorLog>> {
    return unwrapList(() => listAdminOpsErrorLogs({ query, throwOnError: true }));
  },

  async listErrorLogFingerprints(
    query?: ListAdminOpsErrorLogFingerprintsData["query"],
  ): Promise<{ data: OpsErrorFingerprint[]; meta: OpsErrorFingerprintListMeta }> {
    const response = await listAdminOpsErrorLogFingerprints({ query, throwOnError: true });
    if (!response.data || !Array.isArray(response.data.data)) {
      throw new Error("Admin API returned an empty fingerprint response.");
    }
    return { data: response.data.data, meta: response.data.meta };
  },

  getErrorLog(id: string): Promise<OpsErrorLog> {
    return unwrapData(() => getAdminOpsErrorLog({ path: { id }, throwOnError: true }));
  },

  updateErrorLogResolution(id: string, body: OpsErrorLogResolutionUpdate): Promise<OpsErrorLog> {
    return unwrapData(() =>
      updateAdminOpsErrorLogResolution({
        path: { id },
        body,
        throwOnError: true,
      }),
    );
  },
};
