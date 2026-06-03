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
}

export function emptyProxyForm(): ProxyFormState {
  return {
    name: "",
    type: "http",
    url: "",
    status: "active",
    metadata: {},
  };
}

export function proxyFormFromProxy(proxy: ProxyDefinition): ProxyFormState {
  return {
    name: proxy.name,
    type: proxy.type,
    url: "",
    status: proxy.status,
    metadata: (proxy.metadata ?? {}) as Record<string, unknown>,
  };
}

export function buildCreateProxyBody(form: ProxyFormState): CreateAdminProxyData["body"] {
  return {
    name: requiredText(form.name, "Name"),
    type: form.type,
    url: requiredText(form.url, "Proxy URL"),
    status: form.status,
    metadata: form.metadata,
  };
}

export function buildUpdateProxyBody(form: ProxyFormState): UpdateAdminProxyData["body"] {
  const body: UpdateAdminProxyData["body"] = {
    name: requiredText(form.name, "Name"),
    type: form.type,
    status: form.status,
    metadata: form.metadata,
  };
  const url = form.url.trim();
  if (url) {
    body.url = url;
  }
  return body;
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
