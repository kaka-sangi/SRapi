"use client";

import { useQuery } from "@tanstack/react-query";
import { listAdminOpsRealtimeSlots } from "../../../../../packages/sdk/typescript/src/index";
import type {
  RealtimeActiveSlot,
  RealtimeActiveSlotCounters,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { configureSdkClient } from "@/lib/sdk-client";
import { queryKeys } from "@/lib/query-keys";

// Re-exported so consumers (the admin dashboard) can type the counters payload
// without reaching into the deep generated-SDK relative path or editing the
// shared lib/sdk-types barrel.
export type { RealtimeActiveSlotCounters };

/**
 * Realtime active-slot snapshot for the admin dashboard.
 *
 * The shared `opsApi.listOpsRealtimeSlots()` wrapper (lib/admin-api/ops.ts)
 * only surfaces the slot `data[]` array — it drops the response-level
 * `counters` aggregate (active_slots + per-endpoint / per-kind breakdown).
 * Since the dashboard headline needs those counters, this hook reads the raw
 * generated SDK function directly and unwraps the full payload, mirroring the
 * client-configuration pattern used by lib/admin-api/_shared.ts.
 *
 * Admin-only endpoint (403 for non-admins); the AppShell role gate keeps
 * non-admins off the pages that consume it. Polls every 30s so the live slot
 * count tracks active realtime/websocket sessions without manual refresh.
 */
export interface RealtimeSlotsSnapshot {
  counters: RealtimeActiveSlotCounters;
  slots: RealtimeActiveSlot[];
}

const EMPTY_COUNTERS: RealtimeActiveSlotCounters = {
  active_slots: 0,
  acquired_total: 0,
  released_total: 0,
  rejected_total: 0,
  active_by_endpoint: {},
  active_by_kind: {},
  active_by_api_key_id: {},
};

export function useListOpsRealtimeSlots() {
  return useQuery<RealtimeSlotsSnapshot>({
    queryKey: queryKeys.admin.opsRealtimeSlots(),
    queryFn: async () => {
      configureSdkClient();
      const response = await listAdminOpsRealtimeSlots({ throwOnError: true });
      const body = response.data;
      if (!body || !Array.isArray(body.data)) {
        throw new Error("Admin API returned an empty realtime-slots response.");
      }
      return {
        counters: body.counters ?? EMPTY_COUNTERS,
        slots: body.data,
      };
    },
    refetchInterval: 30_000,
  });
}
