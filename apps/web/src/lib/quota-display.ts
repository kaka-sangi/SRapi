"use client";

import type { AccountQuotaSnapshot } from "@/lib/sdk-types";
import { formatDateTime } from "@/lib/admin-format";

export type QuotaWindowKind = "5h" | "7d" | "month" | "other";

export type QuotaDisplayWindow = {
  snapshot: AccountQuotaSnapshot;
  kind: QuotaWindowKind;
  label: string;
  remainingPercent: number;
  usedPercent: number;
  sortOrder: number;
};

type Translate = (key: string, vars?: Record<string, string | number>) => string;

export function latestQuotaWindows(snapshots: AccountQuotaSnapshot[]): QuotaDisplayWindow[] {
  const hasRealSnapshot = snapshots.some((snapshot) => !isSyntheticQuota(snapshot));
  const latestByType = new Map<string, AccountQuotaSnapshot>();
  for (const snapshot of snapshots) {
    if (hasRealSnapshot && isSyntheticQuota(snapshot)) {
      continue;
    }
    const existing = latestByType.get(snapshot.quota_type);
    if (!existing || Date.parse(snapshot.snapshot_at) > Date.parse(existing.snapshot_at)) {
      latestByType.set(snapshot.quota_type, snapshot);
    }
  }
  return [...latestByType.values()]
    .map(toQuotaDisplayWindow)
    .sort((a, b) => a.sortOrder - b.sortOrder || a.label.localeCompare(b.label));
}

export function quotaWindowTiming(window: QuotaDisplayWindow, t: Translate): string {
  if (window.snapshot.reset_at) {
    return t("adminAccounts.quotaResetsAt", { time: formatDateTime(window.snapshot.reset_at) });
  }
  return t("adminAccounts.quotaUpdatedAt", { time: formatDateTime(window.snapshot.snapshot_at) });
}

export function quotaWindowValue(window: QuotaDisplayWindow): string {
  const { snapshot } = window;
  return `${snapshot.used} / ${snapshot.quota_limit}`;
}

export function quotaWindowDisplayLabel(window: QuotaDisplayWindow, t: Translate): string {
  switch (window.kind) {
    case "5h":
      return t("adminAccounts.quotaWindow5h");
    case "7d":
      return t("adminAccounts.quotaWindow7d");
    case "month":
      return t("adminAccounts.quotaWindowMonth");
    default:
      return window.label;
  }
}

function toQuotaDisplayWindow(snapshot: AccountQuotaSnapshot): QuotaDisplayWindow {
  const kind = quotaWindowKind(snapshot.quota_type);
  const remainingPercent = clampPercent(snapshot.remaining_ratio * 100);
  return {
    snapshot,
    kind,
    label: quotaWindowLabel(snapshot.quota_type, kind),
    remainingPercent,
    usedPercent: clampPercent(100 - remainingPercent),
    sortOrder: quotaWindowSortOrder(kind),
  };
}

function quotaWindowKind(quotaType: string): QuotaWindowKind {
  const normalized = quotaType.toLowerCase();
  if (normalized.includes("5h") || normalized.includes("five_hour")) return "5h";
  if (normalized.includes("7d") || normalized.includes("seven_day")) return "7d";
  if (normalized === "provider_credits" || normalized.includes("monthly") || normalized.includes("month")) return "month";
  return "other";
}

function quotaWindowLabel(quotaType: string, kind: QuotaWindowKind): string {
  switch (kind) {
    case "5h":
      return "5h";
    case "7d":
      return "7d";
    case "month":
      return "Month";
    default:
      return quotaType.replaceAll("_", " ");
  }
}

function quotaWindowSortOrder(kind: QuotaWindowKind): number {
  switch (kind) {
    case "5h":
      return 10;
    case "7d":
      return 20;
    case "month":
      return 30;
    default:
      return 90;
  }
}

function isSyntheticQuota(snapshot: AccountQuotaSnapshot): boolean {
  return snapshot.quota_type.trim().toLowerCase().startsWith("synthetic_");
}

function clampPercent(value: number): number {
  if (!Number.isFinite(value)) return 0;
  if (value < 0) return 0;
  if (value > 100) return 100;
  return value;
}
