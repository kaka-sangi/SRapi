import { onCLS, onFCP, onINP, onLCP, onTTFB, type Metric } from "web-vitals";

/**
 * SRapi v0.1.0 telemetry primitives.
 *
 * - `reportWebVitals` wires Core Web Vitals to a console reporter by
 *   default and a remote endpoint if `NEXT_PUBLIC_SRAPI_TELEMETRY_URL` is
 *   set. Self-hosted deploys can point this at any HTTP collector that
 *   accepts a JSON `POST`.
 * - `captureException` sends a low-cardinality, redacted exception summary
 *   to the same endpoint. It never uploads raw request bodies, prompts,
 *   cookies, authorization headers, API keys, or provider credentials.
 */
type WebVitalReporter = (metric: Metric) => void;
type SafeContextValue = string | number | boolean | null;

interface TelemetryPayload {
  kind: "web_vital" | "exception";
  page: string;
  ts: number;
  [key: string]: unknown;
}

const TELEMETRY_URL =
  typeof process !== "undefined" ? process.env.NEXT_PUBLIC_SRAPI_TELEMETRY_URL ?? "" : "";
const MAX_CONTEXT_KEYS = 20;
const MAX_CONTEXT_VALUE_LENGTH = 240;
const MAX_ERROR_MESSAGE_LENGTH = 500;
const MAX_STACK_LENGTH = 2_000;

const SENSITIVE_KEY_RE =
  /(api[-_ ]?key|authorization|cookie|credential|csrf|fingerprint|jwt|message|password|prompt|refresh[-_ ]?token|request[-_ ]?body|secret|session|token)/i;
const REDACTION_PATTERNS: Array<[RegExp, string]> = [
  [/\bBearer\s+[A-Za-z0-9._~+/=-]{8,}\b/gi, "Bearer [redacted]"],
  [/\bsk-[A-Za-z0-9_-]{8,}\b/g, "sk-[redacted]"],
  [/\b(?:csrf|sess)_[A-Za-z0-9_-]{12,}\b/gi, "[redacted]"],
  [/\b[A-Za-z0-9_-]{32,}\b/g, "[redacted]"],
];

const consoleReporter: WebVitalReporter = (metric) => {
  if (process.env.NODE_ENV === "production") return;
  console.info(`[srapi:webvitals] ${metric.name}`, {
    value: Math.round(metric.value * 100) / 100,
    rating: metric.rating,
    id: metric.id,
  });
};

const beaconReporter: WebVitalReporter = (metric) => {
  sendTelemetry({
    kind: "web_vital",
    name: metric.name,
    value: metric.value,
    rating: metric.rating,
    id: metric.id,
    page: currentPage(),
    ts: Date.now(),
  });
};

function currentPage(): string {
  return typeof window === "undefined" ? "" : window.location.pathname;
}

function redactText(value: string, maxLength = MAX_CONTEXT_VALUE_LENGTH): string {
  let redacted = value;
  for (const [pattern, replacement] of REDACTION_PATTERNS) {
    redacted = redacted.replace(pattern, replacement);
  }
  return redacted.length > maxLength ? `${redacted.slice(0, maxLength)}...` : redacted;
}

function safeContext(context?: Record<string, unknown>): Record<string, SafeContextValue> | undefined {
  if (!context) return undefined;
  const entries: Array<[string, SafeContextValue]> = [];
  for (const [key, value] of Object.entries(context)) {
    if (entries.length >= MAX_CONTEXT_KEYS) break;
    if (SENSITIVE_KEY_RE.test(key)) {
      entries.push([key, "[redacted]"]);
      continue;
    }
    if (value === null || typeof value === "number" || typeof value === "boolean") {
      entries.push([key, value]);
      continue;
    }
    if (typeof value === "string") {
      entries.push([key, redactText(value)]);
      continue;
    }
    entries.push([key, `[${Array.isArray(value) ? "array" : typeof value}]`]);
  }
  return Object.fromEntries(entries);
}

function errorSummary(error: unknown): { name: string; message: string; stack?: string } {
  if (error instanceof Error) {
    return {
      name: redactText(error.name || "Error", 120),
      message: redactText(error.message || "Unknown error", MAX_ERROR_MESSAGE_LENGTH),
      stack: error.stack ? redactText(error.stack, MAX_STACK_LENGTH) : undefined,
    };
  }
  return {
    name: typeof error,
    message: redactText(String(error), MAX_ERROR_MESSAGE_LENGTH),
  };
}

function sendTelemetry(payload: TelemetryPayload): void {
  if (!TELEMETRY_URL) return;
  const body = JSON.stringify(payload);

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
  sendTelemetry({
    kind: "exception",
    page: currentPage(),
    ts: Date.now(),
    error: errorSummary(error),
    context: safeContext(context),
  });
}
