"use client";

import {
  cleanupAdminUsage,
  getAdminUsageAggregates,
  getAdminUsageDaily,
  listAdminAuditLogs,
  listAdminBillingLedger,
  listAdminOutboxEvents,
  listAdminUsageLogs,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AuditLog,
  BillingLedgerEntry,
  DomainEventOutbox,
  ListAdminAuditLogsData,
  ListAdminBillingLedgerData,
  ListAdminOutboxEventsData,
  ListAdminUsageLogsData,
  UsageCleanupRequest,
  UsageCleanupResult,
  UsageAggregate,
  UsageAggregateDimension,
  UsageLog,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapList, unwrapData } from "./_shared";
import type { AdminListResult, AdminTimeRange } from "./types";

export const usageApi = {
  listUsageLogs(query?: ListAdminUsageLogsData["query"]): Promise<AdminListResult<UsageLog>> {
    return unwrapList(() => listAdminUsageLogs({ query, throwOnError: true }));
  },

  listUsageDaily(query?: AdminTimeRange): Promise<AdminListResult<UsageAggregate>> {
    return unwrapList(() => getAdminUsageDaily({ query, throwOnError: true }));
  },

  listUsageAggregates(
    dimension: UsageAggregateDimension,
    query?: AdminTimeRange,
  ): Promise<AdminListResult<UsageAggregate>> {
    return unwrapList(
      () => getAdminUsageAggregates({
        query: { dimension, ...query },
        throwOnError: true,
      }),
    );
  },

  // Operator on-demand deletion of usage records (the counterpart to the
  // background retention worker). Requires ≥1 bounded filter (model/start/end);
  // dry_run previews the match count without deleting. Returns matched/deleted.
  cleanupUsage(body: UsageCleanupRequest): Promise<UsageCleanupResult> {
    return unwrapData(() => cleanupAdminUsage({ body, throwOnError: true }));
  },

  listAuditLogs(query?: ListAdminAuditLogsData["query"]): Promise<AdminListResult<AuditLog>> {
    return unwrapList(() => listAdminAuditLogs({ query, throwOnError: true }));
  },
  listOutboxEvents(
    query?: ListAdminOutboxEventsData["query"],
  ): Promise<AdminListResult<DomainEventOutbox>> {
    return unwrapList(() => listAdminOutboxEvents({ query, throwOnError: true }));
  },

  listBillingLedger(
    query?: ListAdminBillingLedgerData["query"],
  ): Promise<AdminListResult<BillingLedgerEntry>> {
    return unwrapList(() => listAdminBillingLedger({ query, throwOnError: true }));
  },
};
