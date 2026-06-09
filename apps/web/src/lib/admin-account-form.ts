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

export const ACCOUNT_RUNTIME_CLASSES: RuntimeClass[] = [
  "api_key",
  "oauth_refresh",
  "oauth_device_code",
  "web_session_cookie",
  "cli_client_token",
  "custom_reverse_proxy",
];

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
    priority: String(account.priority),
    weight: String(account.weight),
    metadata: (account.metadata ?? {}) as Record<string, unknown>,
    groupIds: account.group_ids,
  };
}

export function parseJsonObject(value: string, fieldName: string): Record<string, unknown> {
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

export function diffAccountGroupIds(currentGroupIds: Id[], nextGroupIds: Id[]) {
  const current = new Set(currentGroupIds);
  const next = new Set(nextGroupIds);
  return {
    add: [...next].filter((id) => !current.has(id)),
    remove: [...current].filter((id) => !next.has(id)),
  };
}

export interface AccountProxyBindingConfirmationState {
  accountId: Id;
  accountName: string;
  currentProxyId: string;
  nextProxyId: string | null;
  action: "bind" | "clear";
  phrase: string;
  confirmation: string;
}

export interface AccountModelDiscoveryConfirmationState {
  accountId: Id;
  accountName: string;
  phrase: string;
  confirmation: string;
}

export interface AccountBatchStatusConfirmationState {
  accountIds: Id[];
  status: ProviderAccountStatus;
  phrase: string;
  confirmation: string;
}

export function createAccountProxyBindingConfirmation({
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

export function canConfirmAccountProxyBinding(
  state: AccountProxyBindingConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

export function createAccountModelDiscoveryConfirmation(
  account: Pick<ProviderAccount, "id" | "name">,
): AccountModelDiscoveryConfirmationState {
  return {
    accountId: account.id,
    accountName: account.name,
    phrase: `PERSIST MODELS ${account.name}`,
    confirmation: "",
  };
}

export function canConfirmAccountModelDiscovery(
  state: AccountModelDiscoveryConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

export function buildImportAccountsBody(importJson: string): ImportAdminAccountsData["body"] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(importJson || "{}") as unknown;
  } catch {
    throw new Error("Import JSON must be valid JSON.");
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("Import JSON must be an object.");
  }
  const body = parsed as ImportAdminAccountsData["body"];
  if (!Array.isArray(body.accounts) || body.accounts.length === 0) {
    throw new Error("Import JSON must include a non-empty accounts array.");
  }
  return body;
}

export function buildBatchUpdateAccountsBody({
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

export function createAccountBatchStatusConfirmation({
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

export function canConfirmAccountBatchStatus(
  state: AccountBatchStatusConfirmationState | null,
): boolean {
  return Boolean(state && state.confirmation.trim() === state.phrase);
}

function nullableTrim(value: string): string | null {
  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}
