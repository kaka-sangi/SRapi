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
  metadata: string;
}

export function emptyProxyForm(): ProxyFormState {
  return {
    name: "",
    type: "http",
    url: "",
    status: "active",
    metadata: "{}",
  };
}

export function proxyFormFromProxy(proxy: ProxyDefinition): ProxyFormState {
  return {
    name: proxy.name,
    type: proxy.type,
    url: "",
    status: proxy.status,
    metadata: JSON.stringify(proxy.metadata ?? {}, null, 2),
  };
}

export function buildCreateProxyBody(form: ProxyFormState): CreateAdminProxyData["body"] {
  return {
    name: requiredText(form.name, "Name"),
    type: form.type,
    url: requiredText(form.url, "Proxy URL"),
    status: form.status,
    metadata: parseJsonObject(form.metadata, "Metadata"),
  };
}

export function buildUpdateProxyBody(form: ProxyFormState): UpdateAdminProxyData["body"] {
  const body: UpdateAdminProxyData["body"] = {
    name: requiredText(form.name, "Name"),
    type: form.type,
    status: form.status,
    metadata: parseJsonObject(form.metadata, "Metadata"),
  };
  const url = form.url.trim();
  if (url) {
    body.url = url;
  }
  return body;
}

function parseJsonObject(value: string, fieldName: string): Record<string, unknown> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value || "{}") as unknown;
  } catch {
    throw new Error(`${fieldName} must be valid JSON.`);
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error(`${fieldName} must be a JSON object.`);
  }
  return parsed as Record<string, unknown>;
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
