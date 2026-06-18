"use client";

import {
  deleteAdminRequestLogFile,
  downloadAdminRequestLogFile,
  getAdminRequestLogFile,
  listAdminRequestLogFiles,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  ListAdminRequestLogFilesData,
  RequestLogFileDescriptor,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export type { RequestLogFileDescriptor };
export type RequestLogFilesQuery = ListAdminRequestLogFilesData["query"];

export const requestLogFilesApi = {
  async listRequestLogFiles(
    query?: RequestLogFilesQuery,
  ): Promise<AdminListResult<RequestLogFileDescriptor>> {
    return unwrapList(() => listAdminRequestLogFiles({ query, throwOnError: true }));
  },

  async getRequestLogFile(name: string): Promise<RequestLogFileDescriptor> {
    return unwrapData(() =>
      getAdminRequestLogFile({ path: { name }, throwOnError: true }),
    );
  },

  async downloadRequestLogFile(name: string): Promise<string> {
    const response = await downloadAdminRequestLogFile({
      path: { name },
      throwOnError: true,
    });
    return response.data;
  },

  async deleteRequestLogFile(name: string): Promise<void> {
    await deleteAdminRequestLogFile({ path: { name }, throwOnError: true });
  },
};
