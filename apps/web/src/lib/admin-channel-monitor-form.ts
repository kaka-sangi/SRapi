import type {
  ChannelMonitor,
  ChannelMonitorScope,
  CreateChannelMonitorRequest,
  ChannelMonitorTemplate,
  CreateChannelMonitorTemplateRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const CHANNEL_MONITOR_SCOPES: ChannelMonitorScope[] = [
  "account",
  "group",
  "provider",
  "model",
];

export const CHANNEL_MONITOR_METHODS = ["", "GET", "HEAD", "POST"] as const;

export interface ChannelMonitorFormState {
  name: string;
  scope: ChannelMonitorScope;
  scopeRef: string;
  enabled: boolean;
  intervalSeconds: string;
  model: string;
  method: string;
  url: string;
  headers: Record<string, string>;
  body: string;
  expectedStatusCodes: string[];
  responseJsonPath: string;
  responseContains: string;
}

export function emptyChannelMonitorForm(): ChannelMonitorFormState {
  return {
    name: "",
    scope: "account",
    scopeRef: "",
    enabled: true,
    intervalSeconds: "300",
    model: "",
    method: "",
    url: "",
    headers: {},
    body: "",
    expectedStatusCodes: [],
    responseJsonPath: "",
    responseContains: "",
  };
}

export function channelMonitorFormFromDefinition(def: ChannelMonitor): ChannelMonitorFormState {
  const request = def.request ?? {};
  return {
    name: def.name,
    scope: def.scope,
    scopeRef: def.scope_ref ?? "",
    enabled: def.enabled,
    intervalSeconds: String(def.interval_seconds ?? 300),
    model: def.model ?? "",
    method: request.method ?? "",
    url: request.url ?? "",
    headers: request.headers ?? {},
    body: request.body ?? "",
    expectedStatusCodes: (request.expected_status_codes ?? []).map(String),
    responseJsonPath: request.response_json_path ?? "",
    responseContains: request.response_contains ?? "",
  };
}

function buildRequest(form: {
  method: string;
  url: string;
  headers: Record<string, string>;
  body: string;
  expectedStatusCodes: string[];
  responseJsonPath: string;
  responseContains: string;
}): CreateChannelMonitorRequest["request"] {
  const headers: Record<string, string> = {};
  for (const [key, value] of Object.entries(form.headers ?? {})) {
    const k = key.trim();
    const v = (value ?? "").trim();
    if (k && v) headers[k] = v;
  }
  const expected = form.expectedStatusCodes
    .map((code) => Number(code.trim()))
    .filter((code) => Number.isInteger(code) && code >= 100 && code <= 599);
  const body = form.body.trim();
  if (body) {
    try {
      JSON.parse(body);
    } catch {
      throw new Error("Probe body must be valid JSON.");
    }
  }
  return {
    method: form.method.trim().toUpperCase(),
    url: form.url.trim(),
    headers,
    body,
    expected_status_codes: expected,
    response_json_path: form.responseJsonPath.trim(),
    response_contains: form.responseContains.trim(),
  };
}

export function buildChannelMonitorBody(
  form: ChannelMonitorFormState,
): CreateChannelMonitorRequest {
  const name = form.name.trim();
  if (!name) {
    throw new Error("Name is required.");
  }
  const interval = form.intervalSeconds.trim();
  return {
    name,
    scope: form.scope,
    scope_ref: form.scopeRef.trim(),
    enabled: form.enabled,
    interval_seconds: interval === "" ? 300 : Number(interval),
    model: form.model.trim(),
    request: buildRequest(form),
  };
}

export interface ChannelMonitorTemplateFormState {
  name: string;
  description: string;
  method: string;
  url: string;
  headers: Record<string, string>;
  body: string;
  expectedStatusCodes: string[];
  responseJsonPath: string;
  responseContains: string;
}

export function emptyChannelMonitorTemplateForm(): ChannelMonitorTemplateFormState {
  return {
    name: "",
    description: "",
    method: "",
    url: "",
    headers: {},
    body: "",
    expectedStatusCodes: [],
    responseJsonPath: "",
    responseContains: "",
  };
}

export function channelMonitorTemplateFormFromTemplate(
  tpl: ChannelMonitorTemplate,
): ChannelMonitorTemplateFormState {
  const request = tpl.request ?? {};
  return {
    name: tpl.name,
    description: tpl.description ?? "",
    method: request.method ?? "",
    url: request.url ?? "",
    headers: request.headers ?? {},
    body: request.body ?? "",
    expectedStatusCodes: (request.expected_status_codes ?? []).map(String),
    responseJsonPath: request.response_json_path ?? "",
    responseContains: request.response_contains ?? "",
  };
}

export function buildChannelMonitorTemplateBody(
  form: ChannelMonitorTemplateFormState,
): CreateChannelMonitorTemplateRequest {
  const name = form.name.trim();
  if (!name) {
    throw new Error("Name is required.");
  }
  return {
    name,
    description: form.description.trim(),
    request: buildRequest(form),
  };
}
