import type { OpsSettings } from "@/lib/sdk-types";

export interface OpsSettingsFormState {
  autoRefreshEnabled: boolean;
  refreshIntervalSeconds: string;
  errorRateThreshold: string;
  latencyP95ThresholdMs: string;
  alertRetentionDays: string;
}

// There is no GET for ops settings, so the form opens on documented defaults
// rather than the live values; submitting replaces the whole object (PUT).
export function defaultOpsSettingsForm(): OpsSettingsFormState {
  return {
    autoRefreshEnabled: true,
    refreshIntervalSeconds: "30",
    errorRateThreshold: "0.05",
    latencyP95ThresholdMs: "1000",
    alertRetentionDays: "30",
  };
}

export function buildOpsSettingsBody(form: OpsSettingsFormState): OpsSettings {
  return {
    auto_refresh_enabled: form.autoRefreshEnabled,
    refresh_interval_seconds: requireInt(form.refreshIntervalSeconds, "Refresh interval", 1),
    error_rate_threshold: requireRate(form.errorRateThreshold, "Error rate threshold"),
    latency_p95_threshold_ms: requireInt(form.latencyP95ThresholdMs, "Latency p95 threshold", 1),
    alert_retention_days: requireInt(form.alertRetentionDays, "Alert retention", 1),
  };
}

function requireInt(value: string, name: string, min: number): number {
  const n = Number(value.trim());
  if (!Number.isInteger(n) || n < min) {
    throw new Error(`${name} must be a whole number ≥ ${min}.`);
  }
  return n;
}

function requireRate(value: string, name: string): number {
  const n = Number(value.trim());
  if (!Number.isFinite(n) || n < 0 || n > 1) {
    throw new Error(`${name} must be between 0 and 1.`);
  }
  return n;
}
