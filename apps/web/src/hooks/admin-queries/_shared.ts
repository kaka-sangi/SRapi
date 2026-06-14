"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";

/**
 * Shared primitives for the admin-queries domain modules.
 *
 * Admin data hooks. Pages consume ONLY these (never useEffect+fetch).
 * Everything routes through `adminApi` (lib/admin-api.ts) → generated SDK.
 * All endpoints are admin-only and 403 for regular users — the AppShell role
 * gate keeps non-admins off these pages entirely.
 *
 * Param types are derived from each adminApi method so they always match the
 * generated SDK query shape (status enums etc.).
 */

/** First positional arg of a method, used for `list(params)` / `create(body)` shapes. */
export type P<F extends (...a: never[]) => unknown> = Parameters<F>[0];

/** Second positional arg of a method, used for `update(id, body)` shapes. */
export type B<F extends (...a: never[]) => unknown> = Parameters<F>[1];

// ============================================================
// Mutations (create / update / delete). Each invalidates the broad
// ["admin", <resource>] prefix so every param-scoped query variant
// refetches — the pattern established by useSetAccountStatus.
// ============================================================

export function useAdminMutation<TVars, TData>(
  mutationFn: (vars: TVars) => Promise<TData>,
  invalidate: readonly unknown[],
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => qc.invalidateQueries({ queryKey: invalidate }),
  });
}
