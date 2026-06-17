"use client";

import {
  addAdminAccountGroupMember,
  batchActionAdminAccounts,
  batchCreateAdminAccounts,
  batchDeleteAdminAccounts,
  batchUpdateAdminAccountConcurrency,
  batchUpdateAdminAccounts,
  bindAdminAccountProxy,
  clearAdminAccountError,
  createAdminAccount,
  startAdminAccountOAuthAuthorizeUrl,
  exchangeAdminAccountOAuthCode,
  startAdminAccountOAuthDeviceCode,
  pollAdminAccountOAuthDeviceCode,
  createAdminAccountGroup,
  deleteAdminAccountGroup,
  listAdminAccountsAvailability,
  deleteAdminAccount,
  discoverAdminAccountModels,
  disableAdminAccount,
  enableAdminAccount,
  exportAdminAccounts,
  getAdminAccountHealth,
  getAdminAccountProxyQuality,
  getAdminAccountsHealthSummary,
  getAdminAccountUsageWindows,
  getAdminAccountUsageToday,
  batchGetAdminAccountsUsageToday,
  getAdminAccountUsageDaily,
  fetchAdminAccountQuota,
  getAdminAccountQuota,
  getAdminAccountRpmStatus,
  importAdminAccounts,
  importAdminCodexSession,
  listAdminAccountGroups,
  listAdminAccountGroupMembers,
  listAdminAccounts,
  recoverAdminAccount,
  refreshAdminAccount,
  removeAdminAccountGroupMember,
  resetAdminAccountQuota,
  testAdminAccount,
  updateAdminAccount,
  updateAdminAccountGroup,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AccountGroup,
  AccountGroupMember,
  AccountHealthSnapshot,
  AccountModelDiscovery,
  AccountProxyQuality,
  AccountQuotaReport,
  AccountQuotaSnapshot,
  AccountRpmStatus,
  AccountUsageWindowsResult,
  AccountUsageToday,
  AccountUsageTodayWithId,
  AccountUsageDailyPoint,
  GetAdminAccountUsageDailyData,
  AdminAccountTestRequest,
  AdminTestResult,
  AccountAvailabilitySummary,
  CreateAccountGroupRequest,
  CreateAdminAccountData,
  AccountOAuthProviderConfig,
  AccountOAuthAuthorizeUrl,
  AccountOAuthCredential,
  AccountOAuthDeviceCode,
  AccountOAuthPending,
  DiscoverAdminAccountModelsData,
  BatchCreateAdminAccountsData,
  BatchCreateProviderAccountsResult,
  BatchDeleteProviderAccountsResult,
  BatchUpdateAccountConcurrencyItem,
  BatchUpdateAccountConcurrencyResult,
  BatchUpdateAccountsResult,
  Id,
  CodexSessionImportResult,
  ImportAdminAccountsData,
  ImportAdminCodexSessionData,
  ListAdminAccountsData,
  ProviderAccount,
  ProviderAccountExportItem,
  ProviderAccountImportResult,
  ProviderAccountStatus,
  UpdateAccountGroupRequest,
  UpdateAdminAccountData,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { configureAdminClient, unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const accountsApi = {
  listAccounts(query?: ListAdminAccountsData["query"]): Promise<AdminListResult<ProviderAccount>> {
    return unwrapList(() => listAdminAccounts({ query, throwOnError: true }));
  },

  exportAccounts(): Promise<ProviderAccountExportItem[]> {
    return unwrapData(() => exportAdminAccounts({ throwOnError: true }));
  },

  importAccounts(body: ImportAdminAccountsData["body"]): Promise<ProviderAccountImportResult> {
    return unwrapData(() => importAdminAccounts({ body, throwOnError: true }));
  },

  importCodexSession(body: ImportAdminCodexSessionData["body"]): Promise<CodexSessionImportResult> {
    return unwrapData(() => importAdminCodexSession({ body, throwOnError: true }));
  },

  batchUpdateAccounts(body: Parameters<typeof batchUpdateAdminAccounts>[0]["body"]): Promise<BatchUpdateAccountsResult> {
    return unwrapData(() => batchUpdateAdminAccounts({ body, throwOnError: true }));
  },

  batchActionAccounts(body: Parameters<typeof batchActionAdminAccounts>[0]["body"]): Promise<BatchUpdateAccountsResult> {
    return unwrapData(() => batchActionAdminAccounts({ body, throwOnError: true }));
  },

  createAccount(body: CreateAdminAccountData["body"]): Promise<ProviderAccount> {
    return unwrapData(() => createAdminAccount({ body, throwOnError: true }));
  },

  // Bulk-create — one server call inserts up to 1000 accounts under a shared
  // defaults set. Per-row credential failures surface in result.results[i].error
  // without aborting the rest. Replaces the old "loop client-side over N
  // single-create calls" pattern from the batch tab of the import dialog.
  batchCreateAccounts(body: BatchCreateAdminAccountsData["body"]): Promise<BatchCreateProviderAccountsResult> {
    return unwrapData(() => batchCreateAdminAccounts({ body, throwOnError: true }));
  },

  // Bulk soft-delete up to 1000 accounts in one call. Idempotent on missing
  // ids (NotFound is treated as success — the caller's "this id should not
  // exist" intent is already true). Per-id failures surface in
  // result.errors[] without aborting the call.
  batchDeleteAccounts(accountIds: Id[]): Promise<BatchDeleteProviderAccountsResult> {
    return unwrapData(() =>
      batchDeleteAdminAccounts({ body: { account_ids: accountIds }, throwOnError: true }),
    );
  },

  // Bulk-set per-account max_concurrency (verbatim port of sub2api's
  // BatchUpdateConcurrency). NotFound is idempotent on the server; per-id
  // failures surface in result.errors[].
  batchUpdateAccountConcurrency(
    items: BatchUpdateAccountConcurrencyItem[],
  ): Promise<BatchUpdateAccountConcurrencyResult> {
    return unwrapData(() =>
      batchUpdateAdminAccountConcurrency({ body: { items }, throwOnError: true }),
    );
  },

  // Interactive upstream-account OAuth provisioning (replaces hand-pasting
  // access_token/refresh_token). The minted credential is returned write-only
  // and immediately fed into createAccount — it is never persisted server-side.
  startAccountOAuthAuthorizeUrl(config: AccountOAuthProviderConfig): Promise<AccountOAuthAuthorizeUrl> {
    return unwrapData(() =>
      startAdminAccountOAuthAuthorizeUrl({ body: { config }, throwOnError: true }),
    );
  },

  exchangeAccountOAuthCode(input: {
    sessionId: string;
    code: string;
    state: string;
  }): Promise<AccountOAuthCredential> {
    return unwrapData(() =>
      exchangeAdminAccountOAuthCode({
        body: { session_id: input.sessionId, code: input.code, state: input.state },
        throwOnError: true,
      }),
    );
  },

  startAccountOAuthDeviceCode(config: AccountOAuthProviderConfig): Promise<AccountOAuthDeviceCode> {
    return unwrapData(() =>
      startAdminAccountOAuthDeviceCode({ body: { config }, throwOnError: true }),
    );
  },

  // Polls once. Returns the minted credential on success, or a pending status
  // (status === "pending") that the caller keeps polling on a fixed interval.
  // The endpoint returns a union (200 credential / 202 pending), so we unwrap
  // the envelope directly rather than through the single-type unwrapData.
  async pollAccountOAuthDeviceCode(
    sessionId: string,
  ): Promise<AccountOAuthCredential | AccountOAuthPending> {
    configureAdminClient();
    const response = await pollAdminAccountOAuthDeviceCode({
      body: { session_id: sessionId },
      throwOnError: true,
    });
    const data = response.data?.data;
    if (!data) {
      throw new Error("Device-code poll returned an empty response.");
    }
    return data;
  },

  updateAccount(id: Id, body: UpdateAdminAccountData["body"]): Promise<ProviderAccount> {
    return unwrapData(() => updateAdminAccount({ path: { id }, body, throwOnError: true }));
  },

  deleteAccount(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminAccount({ path: { id }, throwOnError: true }));
  },

  setAccountStatus(id: Id, status: ProviderAccountStatus): Promise<ProviderAccount> {
    // `status` is the desired TARGET state set by the caller (e.g. row toggle /
    // bulk action). Disable the account when the target is "disabled", enable it
    // otherwise. (Previously inverted: enable/disable were swapped, so the row
    // toggle was a no-op and the bulk buttons did the opposite of their label.)
    if (status === "disabled") {
      return unwrapData(() => disableAdminAccount({ path: { id }, throwOnError: true }));
    }
    return unwrapData(() => enableAdminAccount({ path: { id }, throwOnError: true }));
  },

  testAccount(id: Id, body?: AdminAccountTestRequest): Promise<AdminTestResult> {
    return unwrapData(() => testAdminAccount({ path: { id }, body, throwOnError: true }));
  },

  discoverAccountModels(
    id: Id,
    body?: DiscoverAdminAccountModelsData["body"],
  ): Promise<AccountModelDiscovery> {
    return unwrapData(() =>
      discoverAdminAccountModels({ path: { id }, body, throwOnError: true }),
    );
  },

  clearAccountError(id: Id): Promise<ProviderAccount> {
    return unwrapData(() => clearAdminAccountError({ path: { id }, throwOnError: true }));
  },

  recoverAccount(id: Id): Promise<ProviderAccount> {
    return unwrapData(() => recoverAdminAccount({ path: { id }, throwOnError: true }));
  },

  // Trigger an on-demand OAuth access-token refresh against the upstream
  // (same code path the accounts_token_refresh worker uses). Returns the
  // updated account so the UI re-renders in place.
  refreshAccount(id: Id): Promise<ProviderAccount> {
    return unwrapData(() => refreshAdminAccount({ path: { id }, throwOnError: true }));
  },

  resetAccountQuota(id: Id): Promise<ProviderAccount> {
    return unwrapData(() => resetAdminAccountQuota({ path: { id }, throwOnError: true }));
  },

  bindAccountProxy(id: Id, proxyId: string | null): Promise<ProviderAccount> {
    return unwrapData(() =>
      bindAdminAccountProxy({
        path: { id },
        body: { proxy_id: proxyId },
        throwOnError: true,
      }),
    );
  },

  getAccountProxyQuality(id: Id): Promise<AccountProxyQuality> {
    return unwrapData(() => getAdminAccountProxyQuality({ path: { id }, throwOnError: true }));
  },

  getAccountsHealthSummary(): Promise<AccountHealthSnapshot[]> {
    return unwrapData(() => getAdminAccountsHealthSummary({ throwOnError: true }));
  },

  getAccountHealth(id: Id): Promise<AccountHealthSnapshot> {
    return unwrapData(() => getAdminAccountHealth({ path: { id }, throwOnError: true }));
  },

  getAccountQuota(id: Id): Promise<AdminListResult<AccountQuotaSnapshot>> {
    return unwrapList(() => getAdminAccountQuota({ path: { id }, throwOnError: true }));
  },
  // Trigger a live quota pull from the upstream provider (vs the stored
  // snapshots getAccountQuota returns). Returns the fresh provider report.
  fetchAccountQuota(id: Id): Promise<AccountQuotaReport> {
    return unwrapData(() => fetchAdminAccountQuota({ path: { id }, throwOnError: true }));
  },

  getAccountRpmStatus(id: Id): Promise<AccountRpmStatus> {
    return unwrapData(() => getAdminAccountRpmStatus({ path: { id }, throwOnError: true }));
  },

  // Per-account usage roll-ups (read models). Windows = 5h/7d aggregate cards;
  // today = the live "since midnight" stat row; daily = a 30-day point series.
  getAccountUsageWindows(id: Id): Promise<AccountUsageWindowsResult> {
    return unwrapData(() => getAdminAccountUsageWindows({ path: { id }, throwOnError: true }));
  },

  getAccountUsageToday(id: Id): Promise<AccountUsageToday> {
    return unwrapData(() => getAdminAccountUsageToday({ path: { id }, throwOnError: true }));
  },

  batchGetAccountsUsageToday(accountIds: Id[]): Promise<AccountUsageTodayWithId[]> {
    if (accountIds.length === 0) return Promise.resolve([]);
    return unwrapData(() =>
      batchGetAdminAccountsUsageToday({
        query: { account_ids: accountIds.join(",") },
        throwOnError: true,
      }),
    );
  },

  getAccountUsageDaily(
    id: Id,
    query?: GetAdminAccountUsageDailyData["query"],
  ): Promise<AccountUsageDailyPoint[]> {
    return unwrapData(() => getAdminAccountUsageDaily({ path: { id }, query, throwOnError: true }));
  },

  listAccountGroups(): Promise<AdminListResult<AccountGroup>> {
    return unwrapList(() => listAdminAccountGroups({ throwOnError: true }));
  },

  createAccountGroup(body: CreateAccountGroupRequest): Promise<AccountGroup> {
    return unwrapData(() => createAdminAccountGroup({ body, throwOnError: true }));
  },

  updateAccountGroup(id: Id, body: UpdateAccountGroupRequest): Promise<AccountGroup> {
    return unwrapData(() => updateAdminAccountGroup({ path: { id }, body, throwOnError: true }));
  },

  deleteAccountGroup(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminAccountGroup({ path: { id }, throwOnError: true }));
  },

  addAccountToGroup(accountId: Id, groupId: Id): Promise<AccountGroupMember> {
    return unwrapData(() =>
      addAdminAccountGroupMember({
        path: { id: groupId, account_id: accountId },
        throwOnError: true,
      }),
    );
  },

  async removeAccountFromGroup(accountId: Id, groupId: Id): Promise<void> {
    configureAdminClient();
    await removeAdminAccountGroupMember({
      path: { id: groupId, account_id: accountId },
      throwOnError: true,
    });
  },

  listAccountGroupMembers(groupId: Id): Promise<AdminListResult<AccountGroupMember>> {
    return unwrapList(() =>
      listAdminAccountGroupMembers({ path: { id: groupId }, throwOnError: true }),
    );
  },

  listAccountsAvailability(days?: number): Promise<AdminListResult<AccountAvailabilitySummary>> {
    return unwrapList(() => listAdminAccountsAvailability({ query: { days }, throwOnError: true }));
  },
};
