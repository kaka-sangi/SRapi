import type {
  CreateAdminProxyData,
  ProxyDefinition,
  ProxyDefinitionStatus,
  ProxyDefinitionType,
  UpdateAdminProxyData,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const PROXY_TYPES: ProxyDefinitionType[] = ["http", "https", "socks5"];

export const PROXY_STATUSES: ProxyDefinitionStatus[] = ["active", "disabled"];

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
  };
}

export function buildCreateProxyBody(form: ProxyFormState): CreateAdminProxyData["body"] {
  return {
    name: requiredText(form.name, "Name"),
    type: form.type,
    url: requiredText(form.url, "Proxy URL"),
    status: form.status,
    metadata: form.metadata,
    country_code: normalizeCountryCodeField(form.countryCode),
    country_name: form.countryName || null,
  };
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
