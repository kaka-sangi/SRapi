import type { QuietStatus } from "@/components/ui/quiet-badge";

/**
 * Map any backend status string onto the design-system QuietBadge tone.
 * Keeps status rendering uniform across every admin table (§4.4 quiet badges).
 */
export function quietStatusFor(status: string | null | undefined): QuietStatus {
  const s = (status || "").toLowerCase();
  if (["active", "published", "paid", "fulfilled", "resolved", "ok", "healthy", "enabled", "enforce"].includes(s)) {
    return "active";
  }
  if (["limited", "pending", "draft", "suspended", "firing", "warning", "warn", "burning", "monitor", "refunding"].includes(s)) {
    return "limited";
  }
  if (["failed", "refund_failed", "error", "breached", "critical", "canceled", "cancelled", "refunded", "dead", "block"].includes(s)) {
    return "error";
  }
  return "disabled";
}

type Translate = (key: string, vars?: Record<string, string | number>) => string;

/**
 * Localized label for a backend status/mode enum. Avoids leaking internal tokens
 * (needs_reauth / suspended / monitor …) into the UI. Falls back to a humanized
 * form for any value not yet in the `status` message namespace — `t` returns the
 * raw dotted key on a miss, which we detect to switch to humanization.
 */
export function statusLabel(t: Translate, value: string | null | undefined): string {
  const v = (value || "").trim();
  if (!v) return "—";
  const key = `status.${v}`;
  const translated = t(key);
  return translated === key ? humanizeStatus(v) : translated;
}

function humanizeStatus(value: string): string {
  return value.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}
