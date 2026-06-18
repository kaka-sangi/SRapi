"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type P } from "./_shared";

export const requestLogFileDownloadQueryKey = (name: string | null) =>
  ["admin", "request-log-files", "download", name ?? ""] as const;

export function downloadAdminRequestLogFileText(name: string): Promise<string> {
  return adminApi.downloadRequestLogFile(name);
}

export function useAdminRequestLogFiles(
  params?: P<typeof adminApi.listRequestLogFiles>,
  enabled = true,
  refetchInterval?: number | false,
) {
  return useQuery({
    queryKey: queryKeys.admin.requestLogFiles(params),
    queryFn: () => adminApi.listRequestLogFiles(params),
    enabled,
    refetchInterval,
  });
}

export function useAdminRequestLogFileDownload(name: string | null, enabled = true) {
  return useQuery({
    queryKey: requestLogFileDownloadQueryKey(name),
    queryFn: () => downloadAdminRequestLogFileText(name as string),
    enabled: enabled && Boolean(name),
  });
}

export function useDeleteAdminRequestLogFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => adminApi.deleteRequestLogFile(name),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin", "request-log-files"] });
    },
  });
}
