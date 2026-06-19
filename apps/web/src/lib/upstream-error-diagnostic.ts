const GENERIC_UPSTREAM_MESSAGES = new Set([
  "upstream request failed",
  "upstream request failed after retries",
  "upstream gateway error",
  "upstream service temporarily unavailable",
]);

const DELIMITED_KEYS = new Set(["class", "status", "type", "code", "message"]);

export interface UpstreamErrorDiagnostic {
  className?: string;
  status?: number;
  type?: string;
  code?: string;
  message?: string;
  isGenericGatewayWrapper: boolean;
  source?: "body" | "attempt";
}

export interface CompactUpstreamErrorDiagnostic {
  parts: string[];
  message?: string;
}

export interface UpstreamErrorDiagnosticSource {
  errorBodyExcerpt?: string | null;
  upstreamErrors?: Array<{
    body_excerpt?: string | null;
    message?: string | null;
  }> | null;
}

/** Parse redacted upstream error excerpts into low-sensitive triage fields. */
export function parseUpstreamErrorDiagnostic(value?: string | null): UpstreamErrorDiagnostic | null {
  const text = String(value ?? "").trim();
  if (!text) return null;

  const jsonDiagnostic = parseJSONDiagnostic(text);
  if (jsonDiagnostic) return jsonDiagnostic;

  return parseDelimitedDiagnostic(text);
}

/** Prefer real upstream attempt evidence when the top-level body is only a gateway wrapper. */
export function resolveUpstreamErrorDiagnostic(
  source: UpstreamErrorDiagnosticSource,
): UpstreamErrorDiagnostic | null {
  const primary = parseUpstreamErrorDiagnostic(source.errorBodyExcerpt);
  const attempt = firstAttemptDiagnostic(source.upstreamErrors);

  if (primary && !primary.isGenericGatewayWrapper) {
    return { ...primary, source: "body" };
  }
  if (attempt && !attempt.isGenericGatewayWrapper) {
    return { ...attempt, source: "attempt" };
  }
  if (primary) return { ...primary, source: "body" };
  if (attempt) return { ...attempt, source: "attempt" };
  return null;
}

/** Return the dense chips useful in log tables. Generic wrappers are skipped. */
export function compactUpstreamErrorDiagnostic(
  value?: string | null,
): CompactUpstreamErrorDiagnostic | null {
  const diagnostic = parseUpstreamErrorDiagnostic(value);
  if (!diagnostic || diagnostic.isGenericGatewayWrapper) return null;
  const parts = diagnosticParts(diagnostic);
  if (parts.length === 0) return null;
  return { parts, message: diagnostic.message };
}

export function diagnosticParts(diagnostic: UpstreamErrorDiagnostic): string[] {
  return uniqueNonEmpty([
    diagnostic.className,
    diagnostic.status != null ? String(diagnostic.status) : undefined,
    diagnostic.type,
    diagnostic.code,
  ]);
}

function firstAttemptDiagnostic(
  events: UpstreamErrorDiagnosticSource["upstreamErrors"],
): UpstreamErrorDiagnostic | null {
  let fallback: UpstreamErrorDiagnostic | null = null;
  for (const event of events ?? []) {
    const diagnostic =
      parseUpstreamErrorDiagnostic(event.body_excerpt) ??
      parseUpstreamErrorDiagnostic(event.message);
    if (!diagnostic) continue;
    if (!fallback) fallback = diagnostic;
    if (!diagnostic.isGenericGatewayWrapper) return diagnostic;
  }
  return fallback;
}

function parseDelimitedDiagnostic(text: string): UpstreamErrorDiagnostic | null {
  if (!text.includes("=")) return null;
  const fields: Record<string, string> = {};
  let lastKey = "";

  for (const segment of text.split(/\s+\|\s+/)) {
    const separator = segment.indexOf("=");
    if (separator > 0) {
      const key = segment.slice(0, separator).trim().toLowerCase();
      const value = segment.slice(separator + 1).trim();
      if (DELIMITED_KEYS.has(key)) {
        fields[key] = value;
        lastKey = key;
        continue;
      }
    }
    if (lastKey === "message" && segment.trim()) {
      fields.message = `${fields.message ?? ""} | ${segment.trim()}`.trim();
    }
  }

  return normalizeDiagnostic({
    className: fields.class,
    status: numericField(fields.status),
    type: fields.type,
    code: fields.code,
    message: fields.message,
  });
}

function parseJSONDiagnostic(text: string): UpstreamErrorDiagnostic | null {
  if (!text.startsWith("{") && !text.startsWith("[")) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return null;
  }
  if (!isRecord(parsed) || hasSchedulerEvidence(parsed)) return null;

  const nestedError = isRecord(parsed.error) ? parsed.error : undefined;
  const source = nestedError ?? parsed;
  const rootType = stringField(parsed.type);
  const sourceStatusText = stringField(source.status) ?? stringField(parsed.status);
  const type =
    stringField(source.type) ??
    (rootType && rootType !== "error" ? rootType : undefined) ??
    (sourceStatusText && numericField(sourceStatusText) == null ? sourceStatusText : undefined);

  const status =
    numericField(source.status_code) ??
    numericField(parsed.status_code) ??
    numericField(source.status) ??
    numericField(parsed.status) ??
    numericField(source.code) ??
    numericField(parsed.code);

  return normalizeDiagnostic({
    className:
      stringField(source.class) ??
      stringField(source.error_class) ??
      stringField(parsed.class) ??
      stringField(parsed.error_class),
    status,
    type,
    code: valueString(source.code) ?? valueString(parsed.code),
    message:
      stringField(source.message) ??
      stringField(source.detail) ??
      stringField(parsed.message) ??
      (typeof parsed.error === "string" ? stringField(parsed.error) : undefined),
  });
}

function normalizeDiagnostic(input: {
  className?: string;
  status?: number;
  type?: string;
  code?: string;
  message?: string;
}): UpstreamErrorDiagnostic | null {
  const diagnostic = {
    className: clean(input.className),
    status: input.status,
    type: clean(input.type),
    code: clean(input.code),
    message: clean(input.message),
  };
  if (
    !diagnostic.className &&
    diagnostic.status == null &&
    !diagnostic.type &&
    !diagnostic.code &&
    !diagnostic.message
  ) {
    return null;
  }
  return {
    ...diagnostic,
    isGenericGatewayWrapper: isGenericGatewayWrapper(diagnostic.type, diagnostic.className, diagnostic.message),
  };
}

function isGenericGatewayWrapper(
  type?: string,
  className?: string,
  message?: string,
): boolean {
  const normalizedType = clean(type)?.toLowerCase();
  const normalizedClass = clean(className)?.toLowerCase();
  const normalizedMessage = clean(message)?.toLowerCase();
  if (!normalizedMessage || !GENERIC_UPSTREAM_MESSAGES.has(normalizedMessage)) return false;
  return normalizedType === "upstream_error" || normalizedClass === "upstream_error";
}

function hasSchedulerEvidence(value: Record<string, unknown>): boolean {
  return Object.keys(value).some((key) => key.startsWith("scheduler_"));
}

function numericField(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return undefined;
}

function valueString(value: unknown): string | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  return stringField(value);
}

function stringField(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value.trim() : undefined;
}

function clean(value?: string): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

function uniqueNonEmpty(values: Array<string | undefined>): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const cleaned = clean(value);
    if (!cleaned || seen.has(cleaned)) continue;
    seen.add(cleaned);
    out.push(cleaned);
  }
  return out;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}
