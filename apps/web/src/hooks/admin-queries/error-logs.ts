"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type P } from "./_shared";

// ---- Error logs ----
//
// Failed-request records. The list is server-side paginated/filtered (it can
// grow unbounded), so page + filters drive the query. The detail query is
// lazy: it only fires once a row is clicked (id present + dialog open), mirroring
// `useUserBalanceHistory`'s `enabled` gating.

export function useAdminErrorLogs(params?: P<typeof adminApi.listErrorLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.errorLogs(params),
    queryFn: () => adminApi.listErrorLogs(params),
  });
}

export function useAdminErrorLog(id: string | null, enabled = true) {
  return useQuery({
    queryKey: queryKeys.admin.errorLog(id ?? ""),
    queryFn: () => adminApi.getErrorLog(id as string),
    enabled: enabled && Boolean(id),
  });
}

/**
 * Toggle the resolved flag on an error log. Invalidates the list + detail
 * queries so the next read picks up the updated resolved_at/by + flag.
 */
export function useResolveErrorLog() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, resolved }: { id: string; resolved: boolean }) =>
      adminApi.resolveErrorLog(id, resolved),
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: queryKeys.admin.errorLog(vars.id) });
      // The list cache key embeds the active filters; invalidate the whole
      // errorLogs surface so any paginated/filtered list is refetched.
      void qc.invalidateQueries({ queryKey: ["admin", "errorLogs"] });
    },
  });
}
