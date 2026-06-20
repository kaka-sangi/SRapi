import type {
  CreateAdminProxyData,
  ProxyDefinition,
  ProxyFallbackMode,
  ProxyDefinitionStatus,
  ProxyDefinitionType,
  UpdateAdminProxyData,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const PROXY_TYPES: ProxyDefinitionType[] = ["http", "https", "socks5", "socks5h"];

export const PROXY_STATUSES: ProxyDefinitionStatus[] = ["active", "disabled"];

export const PROXY_FALLBACK_MODES: ProxyFallbackMode[] = ["none", "direct", "proxy"];

export const PROXY_BACKUP_NONE = "__none__";

export interface ProxyFormState {
  name: string;
  type: ProxyDefinitionType;
  url: string;
  status: ProxyDefinitionStatus;
  metadata: Record<string, unknown>;
  // ISO-3166-1 alpha-2 country code (operator-supplied). Empty string clears
  // it; the dialog renders a dropdown sourced from countryOptions().
  countryCode: string;
  // Localized display label snapshotted at save time so the list view does not
  // depend on the viewer's locale to render a stable country column.
  countryName: string;
  expiresAtLocal: string;
  fallbackMode: ProxyFallbackMode;
  backupProxyId: string;
}

// COUNTRY_NONE mirrors the sentinel SelectItem value used by the country
// picker in the proxies admin form; lives here so the form helper can seed it
// without importing from the page component.
export const COUNTRY_NONE = "__none__";

export function emptyProxyForm(): ProxyFormState {
  return {
    name: "",
    type: "http",
    url: "",
    status: "active",
    metadata: {},
    countryCode: COUNTRY_NONE,
    countryName: "",
    expiresAtLocal: "",
    fallbackMode: "none",
    backupProxyId: PROXY_BACKUP_NONE,
  };
}

export function proxyFormFromProxy(proxy: ProxyDefinition): ProxyFormState {
  return {
    name: proxy.name,
    type: proxy.type,
    url: "",
    status: proxy.status,
    metadata: (proxy.metadata ?? {}) as Record<string, unknown>,
    countryCode: (proxy.country_code ?? "").trim() || COUNTRY_NONE,
    countryName: proxy.country_name ?? "",
    expiresAtLocal: isoToLocalDateTime(proxy.expires_at),
    fallbackMode: proxy.fallback_mode ?? "none",
    backupProxyId: proxy.backup_proxy_id ?? PROXY_BACKUP_NONE,
  };
}

export function buildCreateProxyBody(form: ProxyFormState): CreateAdminProxyData["body"] {
  const body: CreateAdminProxyData["body"] = {
    name: requiredText(form.name, "Name"),
    type: form.type,
    url: requiredText(form.url, "Proxy URL"),
    status: form.status,
    metadata: form.metadata,
    country_code: normalizeCountryCodeField(form.countryCode),
    country_name: form.countryName || null,
  };
  const expiresAt = localDateTimeToIso(form.expiresAtLocal, "Expires at");
  if (expiresAt) body.expires_at = expiresAt;
  body.fallback_mode = form.fallbackMode;
  const backupProxyId = normalizeBackupProxyID(form.backupProxyId);
  if (backupProxyId) body.backup_proxy_id = backupProxyId;
  if (form.fallbackMode === "proxy" && !backupProxyId) {
    throw new Error("Backup proxy is required when fallback mode is proxy.");
  }
  if (form.fallbackMode !== "proxy") {
    delete body.backup_proxy_id;
  }
  return body;
}

export function buildUpdateProxyBody(form: ProxyFormState): UpdateAdminProxyData["body"] {
  const body: UpdateAdminProxyData["body"] = {
    name: requiredText(form.name, "Name"),
    type: form.type,
    status: form.status,
    metadata: form.metadata,
    country_code: normalizeCountryCodeField(form.countryCode),
    country_name: form.countryName || null,
  };
  const url = form.url.trim();
  if (url) {
    body.url = url;
  }
  const expiresAt = localDateTimeToIso(form.expiresAtLocal, "Expires at");
  if (expiresAt) {
    body.expires_at = expiresAt;
  } else {
    body.clear_expires_at = true;
  }
  body.fallback_mode = form.fallbackMode;
  const backupProxyId = normalizeBackupProxyID(form.backupProxyId);
  if (backupProxyId) {
    body.backup_proxy_id = backupProxyId;
  } else {
    body.clear_backup_proxy_id = true;
  }
  if (form.fallbackMode === "proxy" && !backupProxyId) {
    throw new Error("Backup proxy is required when fallback mode is proxy.");
  }
  if (form.fallbackMode !== "proxy") {
    delete body.backup_proxy_id;
    body.clear_backup_proxy_id = true;
  }
  return body;
}

function normalizeCountryCodeField(raw: string): string | null {
  const trimmed = raw.trim();
  if (!trimmed || trimmed === COUNTRY_NONE) return null;
  return trimmed.toUpperCase();
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}

function normalizeBackupProxyID(raw: string): string | undefined {
  const trimmed = raw.trim();
  if (!trimmed || trimmed === PROXY_BACKUP_NONE) return undefined;
  return trimmed;
}

function localDateTimeToIso(value: string, fieldName: string): string | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const date = new Date(trimmed);
  if (Number.isNaN(date.getTime())) {
    throw new Error(`${fieldName} must be a valid date and time.`);
  }
  return date.toISOString();
}

function isoToLocalDateTime(value?: string | null): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const offsetMs = date.getTimezoneOffset() * 60 * 1000;
  return new Date(date.getTime() - offsetMs).toISOString().slice(0, 16);
}
