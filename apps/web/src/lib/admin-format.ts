"use client";

import type { BadgeProps } from "@/components/ui";

export function formatInteger(value: number | null | undefined): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return new Intl.NumberFormat().format(value);
}

export function formatCompactNumber(value: number | null | undefined): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return new Intl.NumberFormat(undefined, {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

export function formatMoney(
  value: string | number | null | undefined,
  currency = "USD",
): string {
  if (value === null || value === undefined || value === "") {
    return "-";
  }

  const numeric = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(numeric)) {
    return `${value} ${currency}`;
  }

  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
    maximumFractionDigits: 4,
  }).format(numeric);
}

export function formatPercent(value: number | null | undefined): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }

  return `${(value * 100).toFixed(value < 0.1 ? 2 : 1)}%`;
}

export function formatDateTime(value: string | null | undefined): string {
  if (!value) {
    return "-";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

export function formatDate(value: string | null | undefined): string {
  if (!value) {
    return "-";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(date);
}

export function statusBadgeVariant(status: string | null | undefined): BadgeProps["variant"] {
  const normalized = (status || "").toLowerCase();

  if (
    [
      "active",
      "published",
      "paid",
      "fulfilled",
      "resolved",
      "ok",
      "success",
    ].includes(normalized)
  ) {
    return "success";
  }

  if (
    [
      "pending",
      "draft",
      "needs_reauth",
      "suspended",
      "firing",
      "warning",
      "warn",
      "monitor",
      "refunding",
    ].includes(normalized)
  ) {
    return "warning";
  }

  if (
    [
      "disabled",
      "archived",
      "expired",
      "canceled",
      "cancelled",
      "failed",
      "refund_failed",
      "dead",
      "critical",
      "block",
      "error",
      "refunded",
    ].includes(normalized)
  ) {
    return "danger";
  }

  return "neutral";
}

export function safeJson(value: unknown): string {
  if (value === null || value === undefined) {
    return "{}";
  }

  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function clampPercent(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.max(0, Math.min(100, value));
}
