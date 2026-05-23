import { onCLS, onFCP, onINP, onLCP, onTTFB, type Metric } from "web-vitals";

/**
 * SRapi v0.1.0 telemetry primitives.
 *
 * - `reportWebVitals` wires Core Web Vitals to a console reporter by
 *   default and a remote endpoint if `NEXT_PUBLIC_SRAPI_TELEMETRY_URL` is
 *   set. Self-hosted deploys can point this at any HTTP collector that
 *   accepts a JSON `POST`.
 * - `captureException` is a Sentry-compatible stub. Production deploys can
 *   replace this with `import * as Sentry from "@sentry/nextjs"` later
 *   without touching call sites.
 */
type WebVitalReporter = (metric: Metric) => void;

const TELEMETRY_URL =
  typeof process !== "undefined" ? process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL ?? "" : "";

const consoleReporter: WebVitalReporter = (metric) => {
  if (process.env.NODE_ENV === "production") return;
  console.info(`[srapi:webvitals] ${metric.name}`, {
    value: Math.round(metric.value * 100) / 100,
    rating: metric.rating,
    id: metric.id,
  });
};

const beaconReporter: WebVitalReporter = (metric) => {
  if (!TELEMETRY_URL) return;
  const body = JSON.stringify({
    name: metric.name,
    value: metric.value,
    rating: metric.rating,
    id: metric.id,
    page: typeof window === "undefined" ? "" : window.location.pathname,
    ts: Date.now(),
  });

  if (typeof navigator !== "undefined" && "sendBeacon" in navigator) {
    navigator.sendBeacon(TELEMETRY_URL, body);
  } else if (typeof fetch !== "undefined") {
    void fetch(TELEMETRY_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
      keepalive: true,
    }).catch(() => {
      /* swallow network errors so telemetry is never user-visible */
    });
  }
};

export function reportWebVitals(): void {
  const report: WebVitalReporter = (metric) => {
    consoleReporter(metric);
    beaconReporter(metric);
  };
  onCLS(report);
  onFCP(report);
  onINP(report);
  onLCP(report);
  onTTFB(report);
}

export function captureException(error: unknown, context?: Record<string, unknown>): void {
  if (process.env.NODE_ENV !== "production") {
    console.error("[srapi:capture]", error, context);
  }
  // No-op in production until a Sentry/OTel integration is wired here.
}
