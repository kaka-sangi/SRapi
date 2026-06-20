// Shared window-preset filter helpers for the /admin/logs cluster
// (audit-logs, billing-ledger, error-logs). Each preset is a minutes-back-
// from-now bound; an unset filter means "no bound" / "all time". The window
// is resolved at render time so the cutoff slides forward with the wall
// clock — picking "Last 24 hours" should always mean from this moment, not
// from when the filter was selected.
//
// The labelKey paths live under `adminLogs.window*` so multiple panels in
// the cluster share the same translated strings without crossing into
// panel-specific i18n namespaces (audit vs ledger vs error).

export interface LogWindowPreset {
  value: string;
  labelKey: string;
  minutes: number;
}

export const LOG_WINDOW_PRESETS: readonly LogWindowPreset[] = [
  { value: "1h", labelKey: "adminLogs.window1h", minutes: 60 },
  { value: "24h", labelKey: "adminLogs.window24h", minutes: 24 * 60 },
  { value: "7d", labelKey: "adminLogs.window7d", minutes: 7 * 24 * 60 },
  { value: "30d", labelKey: "adminLogs.window30d", minutes: 30 * 24 * 60 },
] as const;

export const LOG_WINDOW_ALL_LABEL_KEY = "adminLogs.allTime";

// Returns the cutoff Date for a preset key, or null when the filter is unset
// or the value isn't one of the known presets. Callers compare row timestamps
// against the cutoff and drop rows that are STRICTLY before it.
export function logWindowSince(value: string | undefined): Date | null {
  if (!value) return null;
  const preset = LOG_WINDOW_PRESETS.find((p) => p.value === value);
  if (!preset) return null;
  return new Date(Date.now() - preset.minutes * 60 * 1000);
}

// Resolve a preset relative to a fixed timestamp. This is for remote query
// keys: callers can keep "Last 1h" stable for the mounted view instead of
// generating a fresh ISO timestamp on every render and refetching forever.
export function logWindowSinceAt(value: string | undefined, nowMs: number): Date | null {
  if (!value) return null;
  const preset = LOG_WINDOW_PRESETS.find((p) => p.value === value);
  if (!preset) return null;
  return new Date(nowMs - preset.minutes * 60 * 1000);
}
