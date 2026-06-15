"use client";

/**
 * Barrel for the admin data hooks. Pages consume ONLY these (never useEffect+fetch).
 *
 * The hooks were split into domain-cohesive modules under `admin-queries/` for
 * maintainability; this file re-exports them so every existing import path
 * (`@/hooks/admin-queries`) keeps working unchanged.
 *
 * Everything routes through `adminApi` (lib/admin-api.ts) → generated SDK.
 * All endpoints are admin-only and 403 for regular users — the AppShell role
 * gate keeps non-admins off these pages entirely.
 */

export * from "./admin-queries/users";
export * from "./admin-queries/accounts";
export * from "./admin-queries/account-usage";
export * from "./admin-queries/providers";
export * from "./admin-queries/models";
export * from "./admin-queries/subscriptions";
export * from "./admin-queries/payments";
export * from "./admin-queries/affiliate";
export * from "./admin-queries/ops";
export * from "./admin-queries/error-logs";
export * from "./admin-queries/system";
export * from "./admin-queries/settings";
