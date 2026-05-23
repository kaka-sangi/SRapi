/**
 * SRapi v0.1.0 query keys.
 * Centralised to avoid typos and to make targeted invalidation simple.
 */
export const queryKeys = {
  runtimeStatus: () => ["runtime-status"] as const,
  smokeStatus: (model?: string) => ["smoke-status", model ?? "default"] as const,
  apiKeys: () => ["api-keys"] as const,
  usageLogs: () => ["usage-logs"] as const,
  providerAccounts: () => ["provider-accounts"] as const,
  schedulerDecisions: () => ["scheduler-decisions"] as const,
  overviewStats: () => ["overview-stats"] as const,
  slos: () => ["slos"] as const,
};
