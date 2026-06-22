import type {
  BatchAccountActionRequest,
  BatchActionAdminAccountsData,
  BatchUpdateAdminAccountsData,
  CreateAdminAccountData,
  Id,
  ImportAdminAccountsData,
  ProviderAccount,
  ProviderAccountStatus,
  RuntimeClass,
  UpdateAdminAccountData,
} from "../../../../packages/sdk/typescript/src/types.gen";

export type AccountRiskLevel = NonNullable<CreateAdminAccountData["body"]["risk_level"]>;

export const ACCOUNT_RISK_LEVELS: AccountRiskLevel[] = ["normal", "medium", "high"];

export const ACCOUNT_RUNTIME_CLASSES: RuntimeClass[] = [
  "api_key",
  "oauth_refresh",
  "oauth_device_code",
  "web_session_cookie",
  "cli_client_token",
  "custom_reverse_proxy",
];

// Human label for a runtime class (auth method). Use everywhere instead of
// showing the raw identifier (e.g. "oauth_refresh") to the operator. Falls back
// to the raw value for any unknown class.
export function runtimeClassLabel(t: (key: string) => string, runtimeClass: string): string {
  if (!runtimeClass) return "";
  const label = t(`adminAccounts.runtime.${runtimeClass}`);
  // t() returns the key itself when missing — fall back to the raw value then.
  return label === `adminAccounts.runtime.${runtimeClass}` ? runtimeClass : label;
}

export const ACCOUNT_STATUSES: ProviderAccountStatus[] = [
  "active",
  "disabled",
  "needs_reauth",
  "suspended",
  "dead",
];

export interface AdminAccountFormState {
  providerId: Id;
  name: string;
  runtimeClass: RuntimeClass;
  upstreamClient: string;
  credential: string;
  proxyId: string;
  status: ProviderAccountStatus;
  riskLevel: AccountRiskLevel;
  priority: string;
  weight: string;
  metadata: Record<string, unknown>;
  groupIds: Id[];
}

export function emptyAccountForm(defaultProviderId = ""): AdminAccountFormState {
  return {
    providerId: defaultProviderId,
    name: "",
    runtimeClass: "api_key",
    upstreamClient: "",
    credential: '{\n  "api_key": ""\n}',
    proxyId: "",
    status: "active",
    riskLevel: "normal",
    priority: "0",
    weight: "1",
    metadata: {},
    groupIds: [],
  };
}

export function accountFormFromAccount(account: ProviderAccount): AdminAccountFormState {
  return {
    providerId: account.provider_id,
    name: account.name,
    runtimeClass: account.runtime_class,
    upstreamClient: account.upstream_client ?? "",
    credential: "",
    proxyId: "",
    status: account.status,
    riskLevel: normalizeRiskLevel(account.risk_level),
    priority: String(account.priority),
    weight: String(account.weight),
    metadata: (account.metadata ?? {}) as Record<string, unknown>,
    groupIds: account.group_ids,
  };
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

export function buildCreateAccountBody(form: AdminAccountFormState): CreateAdminAccountData["body"] {
  return {
    provider_id: form.providerId,
    name: form.name.trim(),
    runtime_class: form.runtimeClass,
    upstream_client: nullableTrim(form.upstreamClient),
    credential: parseJsonObject(form.credential, "Credential"),
    proxy_id: nullableTrim(form.proxyId),
    status: form.status,
    risk_level: form.riskLevel,
    priority: Number(form.priority || 0),
    weight: Number(form.weight || 1),
    metadata: form.metadata,
  };
}

export function buildUpdateAccountBody(form: AdminAccountFormState): UpdateAdminAccountData["body"] {
  const body: UpdateAdminAccountData["body"] = {
    name: form.name.trim(),
    runtime_class: form.runtimeClass,
    upstream_client: nullableTrim(form.upstreamClient),
    status: form.status,
    risk_level: form.riskLevel,
    priority: Number(form.priority || 0),
    weight: Number(form.weight || 1),
    metadata: form.metadata,
  };

  if (form.proxyId.trim()) {
    body.proxy_id = form.proxyId.trim();
  }

  if (form.credential.trim()) {
    body.credential = parseJsonObject(form.credential, "Credential");
  }

  return body;
}

function normalizeRiskLevel(value: string | null | undefined): AccountRiskLevel {
  return ACCOUNT_RISK_LEVELS.includes(value as AccountRiskLevel)
    ? (value as AccountRiskLevel)
    : "normal";
}

function diffAccountGroupIds(currentGroupIds: Id[], nextGroupIds: Id[]) {
  const current = new Set(currentGroupIds);
  const next = new Set(nextGroupIds);
  return {
    add: [...next].filter((id) => !current.has(id)),
    remove: [...current].filter((id) => !next.has(id)),
  };
}

interface AccountProxyBindingConfirmationState {
  accountId: Id;
  accountName: string;
  currentProxyId: string;
  nextProxyId: string | null;
  action: "bind" | "clear";
  phrase: string;
  confirmation: string;
}

interface AccountModelDiscoveryConfirmationState {
  accountId: Id;
  accountName: string;
  phrase: string;
  confirmation: string;
}

interface AccountBatchStatusConfirmationState {
  accountIds: Id[];
  status: ProviderAccountStatus;
  phrase: string;
  confirmation: string;
}

function createAccountProxyBindingConfirmation({
  account,
  currentProxyId,
  nextProxyId,
}: {
  account: Pick<ProviderAccount, "id" | "name">;
  currentProxyId?: string | null;
  nextProxyId?: string | null;
}): AccountProxyBindingConfirmationState {
  const normalizedNext = nextProxyId?.trim() || null;
  const action = normalizedNext ? "bind" : "clear";
  return {
    accountId: account.id,
    accountName: account.name,
    currentProxyId: currentProxyId?.trim() ?? "",
    nextProxyId: normalizedNext,
    action,
    phrase: `${action === "bind" ? "BIND" : "CLEAR"} PROXY ${account.name}`,
    confirmation: "",
  };
}

function canConfirmAccountProxyBinding(
  state: AccountProxyBindingConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

function createAccountModelDiscoveryConfirmation(
  account: Pick<ProviderAccount, "id" | "name">,
): AccountModelDiscoveryConfirmationState {
  return {
    accountId: account.id,
    accountName: account.name,
    phrase: `PERSIST MODELS ${account.name}`,
    confirmation: "",
  };
}

function canConfirmAccountModelDiscovery(
  state: AccountModelDiscoveryConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

export function buildImportAccountsBody(
  importJson: string,
  options: { defaultProviderId?: Id; defaultRuntimeClass?: RuntimeClass; defaultUpstreamClient?: string } = {},
): ImportAdminAccountsData["body"] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(importJson || "{}") as unknown;
  } catch {
    throw new Error("Import JSON must be valid JSON.");
  }
  return buildImportAccountsBodyFromValue(parsed, options);
}

export function buildImportAccountsBodyFromValue(
  parsed: unknown,
  options: { defaultProviderId?: Id; defaultRuntimeClass?: RuntimeClass; defaultUpstreamClient?: string } = {},
): ImportAdminAccountsData["body"] {
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("Import JSON must be an object.");
  }
  const body = parsed as { accounts?: unknown };
  if (!Array.isArray(body.accounts) || body.accounts.length === 0) {
    throw new Error("Import JSON must include a non-empty accounts array.");
  }
  return {
    accounts: body.accounts.map((account, index) =>
      normalizeImportAccount(account, index, options),
    ),
  };
}

function normalizeImportAccount(
  value: unknown,
  index: number,
  options: { defaultProviderId?: Id; defaultRuntimeClass?: RuntimeClass; defaultUpstreamClient?: string },
): ImportAdminAccountsData["body"]["accounts"][number] {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`accounts[${index}] must be an object.`);
  }
  const raw = value as Record<string, unknown>;
  if (isSrapiImportAccount(raw)) {
    return raw as ImportAdminAccountsData["body"]["accounts"][number];
  }
  if (looksLikeSub2apiAccount(raw)) {
    return normalizeSub2apiAccount(raw, index, options);
  }
  throw new Error(`accounts[${index}] must include provider_id/runtime_class or sub2api platform/type.`);
}

function isSrapiImportAccount(raw: Record<string, unknown>): boolean {
  return typeof raw.provider_id === "string" && typeof raw.runtime_class === "string";
}

function looksLikeSub2apiAccount(raw: Record<string, unknown>): boolean {
  return typeof raw.platform === "string" || typeof raw.type === "string" || isRecord(raw.credentials);
}

function normalizeSub2apiAccount(
  raw: Record<string, unknown>,
  index: number,
  options: { defaultProviderId?: Id; defaultRuntimeClass?: RuntimeClass; defaultUpstreamClient?: string },
): ImportAdminAccountsData["body"]["accounts"][number] {
  const providerId = stringValue(raw.provider_id) || options.defaultProviderId || "";
  if (!providerId) {
    throw new Error(`accounts[${index}].provider_id required. Select a target provider before importing sub2api data.`);
  }
  const credential = isRecord(raw.credentials) ? { ...raw.credentials } : {};
  if (Object.keys(credential).length === 0) {
    throw new Error(`accounts[${index}].credentials required.`);
  }
  const extra = isRecord(raw.extra) ? raw.extra : {};
  const metadata = { ...extra };
  if (typeof raw.auto_pause_on_expired === "boolean" && metadata.auto_pause_on_expired == null) {
    metadata.auto_pause_on_expired = raw.auto_pause_on_expired;
  }
  const concurrency = numberValue(raw.concurrency);
  if (concurrency !== undefined && metadata.max_concurrency == null) {
    metadata.max_concurrency = concurrency;
  }
  // Lift identity fields from the sub2api credential bag (upstream-protocol
  // names like chatgpt_account_id) into canonical account metadata keys —
  // canonical names match the backend service-layer canonicalizer at
  // apps/api/internal/modules/accounts/service/metadata_canonical.go. The
  // backend canonicalizes on write so an alias-keyed payload would still land
  // canonical, but writing canonical from the start avoids one round-trip of
  // confusion in the operator-facing import preview.
  const credentialToCanonicalMetadata: Record<string, string> = {
    email: "email",
    chatgpt_account_id: "upstream_account_id",
    chatgpt_user_id: "upstream_user_id",
    organization_id: "organization_id",
    plan_type: "plan_type",
  };
  for (const [credKey, metaKey] of Object.entries(credentialToCanonicalMetadata)) {
    const value = stringValue(credential[credKey]);
    if (value && metadata[metaKey] == null) metadata[metaKey] = value;
  }
  const runtimeClass =
    runtimeClassFromSub2apiType(stringValue(raw.type)) ??
    options.defaultRuntimeClass ??
    "oauth_refresh";
  const upstreamClient =
    stringValue(raw.upstream_client) ||
    upstreamClientFromSub2apiPlatform(stringValue(raw.platform)) ||
    options.defaultUpstreamClient ||
    undefined;
  return {
    provider_id: providerId,
    name: stringValue(raw.name) || stringValue(credential.email) || `sub2api-account-${index + 1}`,
    runtime_class: runtimeClass,
    ...(upstreamClient ? { upstream_client: upstreamClient } : {}),
    credential,
    status: providerAccountStatus(raw.status),
    risk_level: "normal",
    priority: numberValue(raw.priority),
    weight: numberValue(raw.weight) ?? numberValue(raw.rate_multiplier) ?? 1,
    metadata,
  };
}

function runtimeClassFromSub2apiType(value: string): RuntimeClass | null {
  switch (value.trim().toLowerCase()) {
    case "api_key":
    case "apikey":
    case "api-key":
      return "api_key";
    case "oauth":
    case "oauth_refresh":
      return "oauth_refresh";
    case "oauth_device_code":
    case "device_code":
      return "oauth_device_code";
    case "cookie":
    case "session_cookie":
    case "web_session_cookie":
      return "web_session_cookie";
    case "cli":
    case "cli_client_token":
      return "cli_client_token";
    case "custom_reverse_proxy":
      return "custom_reverse_proxy";
    default:
      return null;
  }
}

function upstreamClientFromSub2apiPlatform(value: string): string | null {
  switch (value.trim().toLowerCase()) {
    case "openai":
    case "codex":
    case "codex-cli":
      return "codex_cli";
    case "chatgpt":
    case "chatgpt_web":
      return "chatgpt_web";
    case "claude":
    case "claude_code":
      return "claude_code_cli";
    case "antigravity":
      return "antigravity_desktop";
    default:
      return null;
  }
}

function providerAccountStatus(value: unknown): ProviderAccountStatus {
  return ACCOUNT_STATUSES.includes(value as ProviderAccountStatus)
    ? (value as ProviderAccountStatus)
    : "active";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function numberValue(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return undefined;
}

function buildBatchUpdateAccountsBody({
  accountIds,
  status,
}: {
  accountIds: Id[];
  status: ProviderAccountStatus;
}): BatchUpdateAdminAccountsData["body"] {
  const ids = [...new Set(accountIds.map((id) => id.trim()).filter(Boolean))];
  if (ids.length === 0) {
    throw new Error("Select at least one account.");
  }
  return { account_ids: ids, status };
}

export type AccountBatchAction = BatchAccountActionRequest["action"];

export function buildBatchAccountActionBody({
  accountIds,
  action,
}: {
  accountIds: Id[];
  action: AccountBatchAction;
}): BatchActionAdminAccountsData["body"] {
  const ids = [...new Set(accountIds.map((id) => id.trim()).filter(Boolean))];
  if (ids.length === 0) {
    throw new Error("Select at least one account.");
  }
  return { account_ids: ids, action };
}

function createAccountBatchStatusConfirmation({
  accountIds,
  status,
}: {
  accountIds: Id[];
  status: ProviderAccountStatus;
}): AccountBatchStatusConfirmationState {
  const uniqueIds = [...new Set(accountIds)];
  return {
    accountIds: uniqueIds,
    status,
    phrase: `SET ${uniqueIds.length} ACCOUNTS ${status}`,
    confirmation: "",
  };
}

function canConfirmAccountBatchStatus(
  state: AccountBatchStatusConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

function nullableTrim(value: string): string | null {
  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}
