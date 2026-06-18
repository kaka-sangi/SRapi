import type { OpsSystemLog } from "@/lib/sdk-types";

const SYSTEM_LOG_METADATA_EVIDENCE_KEYS = [
  "api_key_prefix",
  "attempted_key_prefix",
  "deleted_key_id",
  "deleted_key_name",
  "provider_id",
  "provider",
  "account_id",
  "account",
  "model",
  "upstream_model",
  "source_endpoint",
  "target_protocol",
  "status_code",
  "error_class",
  "error_phase",
  "error_owner",
  "reason",
] as const;

export function opsSystemLogEvidenceItems(
  log: OpsSystemLog,
): Array<{ key: string; value: string }> {
  const items: Array<{ key: string; value: string }> = [];
  addEvidenceItem(items, "req", log.request_id);
  addEvidenceItem(items, "trace", log.trace_id);
  const metadata = log.metadata;
  if (!metadata || typeof metadata !== "object") {
    return items.slice(0, 8);
  }
  for (const key of SYSTEM_LOG_METADATA_EVIDENCE_KEYS) {
    addEvidenceItem(items, key, metadata[key]);
  }
  return items.slice(0, 8);
}

function addEvidenceItem(
  items: Array<{ key: string; value: string }>,
  key: string,
  value: unknown,
) {
  const formatted = formatSystemLogEvidenceValue(value);
  if (!formatted) return;
  if (items.some((item) => item.key === key && item.value === formatted)) return;
  items.push({ key, value: formatted });
}

export function formatSystemLogEvidenceValue(value: unknown): string {
  if (typeof value === "string") {
    return value.trim();
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}
