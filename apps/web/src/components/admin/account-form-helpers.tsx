import type { AdminAccountFormState } from "@/lib/admin-account-form";
import type { PlatformFamily } from "@/lib/sdk-types";

/** A provider entry enriched with the per-provider auth-method scoping data. */
export interface AccountProviderOption {
  value: string;
  label: string;
  platformFamily?: PlatformFamily | null;
  authMethods?: RuntimeClass[] | null;
  adapterType?: string;
  accountTemplate?: AccountTemplate | null;
}

export interface AccountTemplate {
  upstream_client?: string;
  default_metadata?: Record<string, unknown>;
  model_catalog?: string[];
  metadata_hints?: Record<string, string>;
}

/**
 * Stable display order for the grouped provider dropdown — first-party families
 * first, then reverse-proxy and rerank. Families not listed here (or providers
 * carrying no platform_family) fall through to an unlabeled trailing group.
 */
const PLATFORM_FAMILY_ORDER: PlatformFamily[] = [
  "anthropic_compatible",
  "gemini_compatible",
  "openai_compatible",
  "bedrock_anthropic",
  "reverse_proxy_antigravity",
  "rerank_compatible",
];

const CODEX_FALLBACK_TEMPLATE: AccountTemplate = {
  upstream_client: "codex_cli",
  default_metadata: { base_url: "https://chatgpt.com/backend-api/codex" },
  model_catalog: ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "codex-auto-review", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2", "codex-mini-latest"],
  metadata_hints: { base_url: "Codex upstream (adapter appends /responses)", chatgpt_account_id: "From session JWT (optional)" },
};

export function getProviderTemplate(
  providerOptions: AccountProviderOption[],
  providerId: string,
): AccountTemplate | null {
  const p = providerOptions.find((o) => o.value === providerId);
  if (p?.accountTemplate) return p.accountTemplate;
  if (p?.adapterType === "reverse-proxy-codex-cli") return CODEX_FALLBACK_TEMPLATE;
  return null;
}

export function providerLabelFor(providerOptions: AccountProviderOption[], providerId: string): string {
  return providerOptions.find((opt) => opt.value === providerId)?.label?.trim() || "account";
}

export function buildDefaultAccountName(providerLabel: string, credentialValue: string, index?: number): string {
  const prefix = slugName(providerLabel) || "account";
  const tail = credentialTail(credentialValue);
  if (tail) return `${prefix}-${tail}`;
  return `${prefix}-${index && index > 0 ? index : 1}`;
}

export type RuntimeClass = AdminAccountFormState["runtimeClass"];

/**
 * Group providers by platform family for the dropdown (sub2api-style), in a
 * stable family order. Providers without a family fall into a trailing group.
 */
export function groupProviders(
  providerOptions: AccountProviderOption[],
): { family: PlatformFamily | null; options: AccountProviderOption[] }[] {
  const byFamily = new Map<PlatformFamily, AccountProviderOption[]>();
  const ungrouped: AccountProviderOption[] = [];
  for (const opt of providerOptions) {
    if (opt.platformFamily) {
      const list = byFamily.get(opt.platformFamily) ?? [];
      list.push(opt);
      byFamily.set(opt.platformFamily, list);
    } else {
      ungrouped.push(opt);
    }
  }
  const groups: { family: PlatformFamily | null; options: AccountProviderOption[] }[] = [];
  for (const family of PLATFORM_FAMILY_ORDER) {
    const list = byFamily.get(family);
    if (list && list.length > 0) groups.push({ family, options: list });
  }
  for (const [family, list] of byFamily) {
    if (!PLATFORM_FAMILY_ORDER.includes(family) && list.length > 0) {
      groups.push({ family, options: list });
    }
  }
  if (ungrouped.length > 0) groups.push({ family: null, options: ungrouped });
  return groups;
}

/**
 * Per-runtime credential UX. The admin enters friendly values — an API key, a
 * token, a cookie, or a couple of labeled token fields — never raw JSON, and we
 * assemble the credential object with the exact keys the backend's `injectAuth`
 * switch reads. OAuth runtimes get one labeled input per token; the
 * service-account runtime keeps a JSON box but adds a file-upload button so the
 * admin can drop in the downloaded `.json` rather than hand-type it.
 */
type CredKind = "password" | "textarea" | "json" | "fields";
interface CredFieldSpec {
  key: string; // credential object key the backend reads
  cred: string; // i18n suffix under adminAccounts.cred.* (…Label / …Hint)
  secret?: boolean; // render as a password input
}
export interface CredSpec {
  kind: CredKind;
  credKey?: string;
  cred: string; // i18n suffix under adminAccounts.cred.*
  template?: string;
  /** kind "fields": one labeled input per credential key */
  fields?: CredFieldSpec[];
}
const OAUTH_FIELDS: CredFieldSpec[] = [
  { key: "access_token", cred: "accessToken", secret: true },
  { key: "refresh_token", cred: "refreshToken", secret: true },
  { key: "id_token", cred: "idToken", secret: true },
];
const SERVICE_ACCOUNT_JSON_TEMPLATE = `{
  "type": "service_account",
  "project_id": "",
  "private_key_id": "",
  "private_key": "-----BEGIN PRIVATE KEY-----\\n...\\n-----END PRIVATE KEY-----\\n",
  "client_email": "",
  "token_uri": "https://oauth2.googleapis.com/token"
}`;

const CREDENTIAL_SPECS: Record<RuntimeClass, CredSpec> = {
  api_key: { kind: "password", credKey: "api_key", cred: "apiKey" },
  cli_client_token: { kind: "password", credKey: "access_token", cred: "accessToken" },
  custom_reverse_proxy: { kind: "password", credKey: "access_token", cred: "accessToken" },
  web_session_cookie: { kind: "textarea", credKey: "cookie", cred: "cookie" },
  oauth_refresh: { kind: "fields", cred: "oauth", fields: OAUTH_FIELDS },
  oauth_device_code: { kind: "fields", cred: "oauth", fields: OAUTH_FIELDS },
  // Vertex AI: the admin pastes the GCP service-account JSON wholesale; the
  // backend normalizes the private_key (PKCS#1/PKCS#8 conversion, CRLF/ANSI
  // recovery), encrypts it at rest, and signs JWT bearer tokens at dispatch
  // time. Stored under the `service_account_json` credential key.
  service_account_json: {
    kind: "json",
    credKey: "service_account_json",
    cred: "serviceAccountJson",
    template: SERVICE_ACCOUNT_JSON_TEMPLATE,
  },
};

export function specFor(rc: RuntimeClass): CredSpec {
  return CREDENTIAL_SPECS[rc] ?? CREDENTIAL_SPECS.api_key;
}

// metadataStringList reads a metadata field as a list of trimmed, non-empty
// strings. Accepts an array (the canonical shape) or a comma-separated string
// (a legacy/typo-friendly shape the gateway also tolerates).
export function metadataStringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value === "string") {
    return value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

export function defaultCredInput(rc: RuntimeClass): string {
  const spec = specFor(rc);
  return spec.kind === "json" ? (spec.template ?? "{}") : "";
}

/** True when the admin has supplied a credential for the selected runtime. */
export function hasCredential(rc: RuntimeClass, value: string, fields: Record<string, string>): boolean {
  const spec = specFor(rc);
  if (spec.kind === "fields") return (spec.fields ?? []).some((f) => fields[f.key]?.trim());
  return value.trim() !== "";
}

/** Assemble the credential JSON string consumed by buildCreate/UpdateAccountBody. */
export function buildCredentialJson(
  rc: RuntimeClass,
  value: string,
  fields: Record<string, string>,
): string {
  const spec = specFor(rc);
  if (spec.kind === "json") return value; // raw JSON, validated downstream
  if (spec.kind === "fields") {
    const object: Record<string, string> = {};
    for (const f of spec.fields ?? []) {
      const v = fields[f.key]?.trim();
      if (v) object[f.key] = v;
    }
    return Object.keys(object).length ? JSON.stringify(object) : "";
  }
  const v = value.trim();
  return v ? JSON.stringify({ [spec.credKey as string]: v }) : "";
}

export function credentialFieldsFromPaste(value: string, preferredPlainTextField = "access_token"): {
  fields: Record<string, string>;
  metadata: Record<string, unknown>;
  name?: string;
} {
  const trimmed = value.trim();
  if (!trimmed) return { fields: {}, metadata: {} };
  const parsed = parsePastedJson(trimmed);
  if (!parsed) return { fields: { [preferredPlainTextField]: trimmed }, metadata: {} };
  const object = accountCredentialObject(parsed);
  if (!object) return { fields: {}, metadata: {} };
  const fields: Record<string, string> = {};
  for (const key of ["access_token", "refresh_token", "id_token", "session_token", "api_key", "cookie"]) {
    const value = credentialStringField(object, key);
    if (value) fields[key] = value;
  }
  const metadata: Record<string, unknown> = {};
  const extra = isPlainObject(object.extra) ? object.extra : isPlainObject(parsed.extra) ? parsed.extra : null;
  if (extra) {
    Object.assign(metadata, extra);
  }
  for (const key of ["email", "chatgpt_account_id", "chatgpt_user_id", "organization_id", "plan_type"]) {
    const value = credentialStringField(object, key);
    if (value && metadata[key] == null) metadata[key] = value;
  }
  const name = credentialStringField(parsed, "name") || credentialStringField(object, "email");
  return { fields, metadata, name: name || undefined };
}

export function credentialNameSeed(
  rc: RuntimeClass,
  value: string,
  fields: Record<string, string>,
): string {
  const spec = specFor(rc);
  if (spec.kind === "fields") {
    for (const f of spec.fields ?? []) {
      const v = fields[f.key]?.trim();
      if (v) return v;
    }
    return "";
  }
  return value.trim();
}

function slugName(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 36);
}

function credentialTail(value: string): string {
  const normalized = value.trim().replace(/[^a-zA-Z0-9]/g, "");
  if (normalized.length < 6) return "";
  return normalized.slice(-8).toLowerCase();
}

function parsePastedJson(value: string): Record<string, unknown> | null {
  if (!value.startsWith("{")) return null;
  try {
    const parsed = JSON.parse(value) as unknown;
    return isPlainObject(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function accountCredentialObject(value: Record<string, unknown>): Record<string, unknown> | null {
  if (isPlainObject(value.credentials)) return value.credentials;
  if (isPlainObject(value.credential)) return value.credential;
  return value;
}

function credentialStringField(value: Record<string, unknown>, key: string): string {
  return (
    stringField(value, key) ||
    credentialAliasStringField(value, key) ||
    nestedCredentialStringField(value, "tokens", key) ||
    nestedCredentialAliasStringField(value, "tokens", key)
  );
}

function nestedCredentialStringField(value: Record<string, unknown>, objectKey: string, fieldKey: string): string {
  const nested = value[objectKey];
  return isPlainObject(nested) ? stringField(nested, fieldKey) : "";
}

function nestedCredentialAliasStringField(value: Record<string, unknown>, objectKey: string, fieldKey: string): string {
  const nested = value[objectKey];
  return isPlainObject(nested) ? credentialAliasStringField(nested, fieldKey) : "";
}

function credentialAliasStringField(value: Record<string, unknown>, key: string): string {
  for (const alias of credentialFieldAliases(key)) {
    const raw = stringField(value, alias);
    if (raw) return raw;
  }
  return "";
}

function credentialFieldAliases(key: string): string[] {
  switch (key) {
    case "access_token":
      return ["accessToken", "token"];
    case "refresh_token":
      return ["refreshToken", "rt"];
    case "id_token":
      return ["idToken"];
    case "session_token":
      return ["sessionToken"];
    case "api_key":
      return ["apiKey"];
    default:
      return [];
  }
}

function stringField(value: Record<string, unknown>, key: string): string {
  const raw = value[key];
  return typeof raw === "string" ? raw.trim() : "";
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
