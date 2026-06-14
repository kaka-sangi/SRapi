import type { Pagination } from "../../../../../packages/sdk/typescript/src/types.gen";

export interface AdminListResult<T> {
  data: T[];
  pagination?: Pagination;
}

export interface AdminTimeRange {
  start?: string;
  end?: string;
}

export interface CircuitBreakerEntry {
  account_id: number;
  state: "closed" | "open" | "half-open";
  requests: number;
  total_successes: number;
  total_failures: number;
  consecutive_successes: number;
  consecutive_failures: number;
  success_rate: number;
}

export interface CRSPreviewRequest {
  base_url: string;
  username: string;
  password: string;
}

export interface CRSPreviewAccount {
  crs_account_id: string;
  kind: string;
  name: string;
  platform: string;
  type: string;
}

export interface CRSPreviewResult {
  new_accounts: CRSPreviewAccount[];
  existing_accounts: CRSPreviewAccount[];
}

export interface CRSSyncRequest {
  base_url: string;
  username: string;
  password: string;
  sync_proxies?: boolean;
  selected_account_ids?: string[];
}

export interface CRSSyncResult {
  created: number;
  updated: number;
  skipped: number;
  failed: number;
  items: Array<{
    crs_account_id: string;
    kind: string;
    name: string;
    action: string;
    error?: string;
  }>;
}

export interface CacheStatsEntry {
  name: string;
  hits: number;
  misses: number;
  evictions: number;
  size: number;
  hit_rate: string;
}

export interface AdminUnsupportedSurface {
  title: string;
  contractPath?: string;
  reason: string;
}
