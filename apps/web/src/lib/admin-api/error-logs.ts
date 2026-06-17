"use client";

import {
  getAdminErrorLog,
  listAdminErrorLogs,
  resolveAdminErrorLog,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  ErrorLog,
  ListAdminErrorLogsData,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapList, unwrapData } from "./_shared";
import type { AdminListResult } from "./types";

export const errorLogsApi = {
  // Server-side paginated/filtered list of failed-request records. The query
  // shape mirrors the generated `ListAdminErrorLogsData["query"]`
  // (page/page_size + user/account/model/error_class/source_endpoint/time-range/q).
  listErrorLogs(
    query?: ListAdminErrorLogsData["query"],
  ): Promise<AdminListResult<ErrorLog>> {
    return unwrapList(() => listAdminErrorLogs({ query, throwOnError: true }));
  },

  // Full metadata for a single failed request (opened from a list row click).
  getErrorLog(id: string): Promise<ErrorLog> {
    return unwrapData(() => getAdminErrorLog({ path: { id }, throwOnError: true }));
  },

  // Toggle the resolved flag on an error log. Returns the updated ErrorLog.
  resolveErrorLog(id: string, resolved: boolean): Promise<ErrorLog> {
    return unwrapData(() =>
      resolveAdminErrorLog({
        path: { id },
        body: { resolved },
        throwOnError: true,
      }),
    );
  },
};
