"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";

export function useAdminGatewayResources() {
  return useQuery({
    queryKey: queryKeys.admin.gatewayResources(),
    queryFn: () => adminApi.getGatewayResources(),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}
