import type { RequestLogFileDescriptor } from "@/lib/admin-api/request-log-files";

export interface RequestDumpSummary {
  requestID?: string;
  userID?: string;
  apiKeyID?: string;
  accountID?: string;
  sourceProtocol?: string;
  sourceEndpoint?: string;
  startedAt?: string;
  success?: boolean;
  statusCode?: number;
  errorClass?: string;
  latencyMS?: number;
  attemptCount: number;
  responseCount: number;
  hasSummary: boolean;
}

export function requestLogDescriptorSummary(
  file: RequestLogFileDescriptor,
): RequestDumpSummary {
  const hasSummary =
    file.has_summary ??
    (file.success !== undefined ||
      file.status_code !== undefined ||
      file.error_class !== undefined ||
      file.latency_ms !== undefined);

  return {
    requestID: file.request_id,
    userID: file.user_id,
    apiKeyID: file.api_key_id,
    accountID: file.account_id,
    sourceProtocol: file.source_protocol,
    sourceEndpoint: file.source_endpoint,
    startedAt: file.started_at,
    success: file.success,
    statusCode: file.status_code,
    errorClass: file.error_class,
    latencyMS: file.latency_ms,
    attemptCount: file.attempt_count ?? 0,
    responseCount: file.response_count ?? 0,
    hasSummary,
  };
}

export function parseRequestDumpSummary(text: string): RequestDumpSummary {
  const requestInfo = parseNamedSection(text, "REQUEST INFO");
  const summary = parseNamedSection(text, "SUMMARY");
  const requestSections = countNumberedSections(text, "REQUEST");
  const responseSections = countNumberedSections(text, "RESPONSE");

  return {
    requestID: requestInfo["Request-ID"],
    userID: requestInfo["User-ID"],
    apiKeyID: requestInfo["API-Key-ID"],
    accountID: requestInfo["Account-ID"],
    sourceProtocol: requestInfo["Source-Protocol"],
    sourceEndpoint: requestInfo["Source-Endpoint"],
    startedAt: requestInfo["Started-At"],
    success: parseBoolean(summary.Success),
    statusCode: parseInteger(summary.Status),
    errorClass: summary["Error-Class"],
    latencyMS: parseInteger(summary["Latency-MS"]),
    attemptCount: Math.max(requestSections.max, responseSections.max),
    responseCount: responseSections.count,
    hasSummary: Object.keys(summary).length > 0,
  };
}

function parseNamedSection(text: string, name: string): Record<string, string> {
  const match = new RegExp(`^=== ${escapeRegExp(name)} ===\\s*$`, "m").exec(text);
  if (!match) return {};

  const start = match.index + match[0].length;
  const rest = text.slice(start);
  const nextSection = rest.search(/\n=== [^\n]+ ===\s*/);
  const block = nextSection >= 0 ? rest.slice(0, nextSection) : rest;
  const fields: Record<string, string> = {};

  for (const rawLine of block.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) continue;
    const sep = line.indexOf(":");
    if (sep <= 0) continue;
    const key = line.slice(0, sep).trim();
    const value = line.slice(sep + 1).trim();
    if (key && value) fields[key] = value;
  }

  return fields;
}

function countNumberedSections(text: string, name: string): { count: number; max: number } {
  const pattern = new RegExp(`^=== ${escapeRegExp(name)} (\\d+) ===\\s*$`, "gm");
  let count = 0;
  let max = 0;
  for (const match of text.matchAll(pattern)) {
    count++;
    const value = Number.parseInt(match[1] ?? "", 10);
    if (Number.isFinite(value)) max = Math.max(max, value);
  }
  return { count, max };
}

function parseBoolean(value: string | undefined): boolean | undefined {
  if (value === undefined) return undefined;
  const normalized = value.trim().toLowerCase();
  if (normalized === "true") return true;
  if (normalized === "false") return false;
  return undefined;
}

function parseInteger(value: string | undefined): number | undefined {
  if (value === undefined) return undefined;
  const parsed = Number.parseInt(value.trim(), 10);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
