"use client";

import { configureSdkClient } from "./sdk-client";
import {
  getCurrentUser,
  updateCurrentUserProfile,
  changeCurrentUserPassword,
  getCurrentUserTotpStatus,
  setupCurrentUserTotp,
  enableCurrentUserTotp,
  disableCurrentUserTotp,
  getCurrentUserBalance,
  getCurrentUserBillingHistory,
  listCurrentUserPlatformQuotas,
  getCurrentUserSubscriptions,
  redeemCurrentUserRedeemCode,
  listPaymentMethods,
  listPaymentOrders,
  listSubscriptionPlans,
  getPaymentOrder,
  createPaymentOrder,
  cancelPaymentOrder,
  getCurrentUserAffiliate,
  listCurrentUserAffiliateInviteCodes,
  createCurrentUserAffiliateInviteCode,
  listCurrentUserAffiliateLedger,
  requestCurrentUserAffiliateWithdrawal,
  transferCurrentUserAffiliateToBalance,
  listCurrentUserAnnouncements,
  markCurrentUserAnnouncementRead,
  listMePlaygroundModels,
  revokeAllCurrentUserSessions,
  getCurrentUserNotificationPreferences,
  updateCurrentUserNotificationPreferences,
  listCurrentUserNotificationContacts,
  requestCurrentUserNotificationContactVerification,
  confirmCurrentUserNotificationContactVerification,
  updateCurrentUserNotificationContact,
  deleteCurrentUserNotificationContact,
  listCurrentUserAuthIdentities,
  unbindCurrentUserAuthIdentity,
  uploadCurrentUserAvatar,
  deleteCurrentUserAvatar,
} from "../../../../packages/sdk/typescript/src/index";
import type {
  AffiliateTransferToBalanceRequest,
  AffiliateWithdrawalRequest,
  CreateAffiliateInviteCodeRequest,
  PlaygroundModel,
  ChangeCurrentUserPasswordRequest,
  CreatePaymentOrderRequest,
  ListCurrentUserAffiliateLedgerData,
  GetCurrentUserBillingHistoryData,
  ListPaymentOrdersData,
  Pagination,
  RedeemCodeRedemptionRequest,
  TotpVerifyRequest,
  UpdateCurrentUserProfileRequest,
  UpdateNotificationPreferencesRequest,
  NotificationContactVerificationRequest,
  NotificationContactConfirmRequest,
  UpdateNotificationContactRequest,
  UserAnnouncement,
  UserPlatformQuota,
} from "../../../../packages/sdk/typescript/src/types.gen";

export interface MeListResult<T> {
  data: T[];
  pagination?: Pagination;
}

export interface MeAnnouncementsResult {
  data: UserAnnouncement[];
  unread: number;
  pagination?: Pagination;
}

// SDK-client setup (base URL, cookie credentials, CSRF auth) is shared across
// the functional clients; see ./sdk-client. Kept under the original local name
// so the many call sites below stay untouched.
const configureClient = configureSdkClient;

async function unwrapData<T>(request: () => Promise<{ data?: { data?: T } }>): Promise<T> {
  configureClient();
  const response = await request();
  if (!response.data || !("data" in response.data)) {
    throw new Error("Request returned an empty response.");
  }
  return response.data.data as T;
}

async function unwrapList<T>(
  request: () => Promise<{ data?: { data?: T[]; pagination?: Pagination } }>,
): Promise<MeListResult<T>> {
  configureClient();
  const response = await request();
  if (!response.data || !Array.isArray(response.data.data)) {
    throw new Error("Request returned an empty list response.");
  }
  return { data: response.data.data, pagination: response.data.pagination };
}

export function meErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === "object" && error !== null) {
    const maybe = error as {
      error?: { message?: string };
      message?: string;
      response?: { data?: { error?: { message?: string } } };
    };
    return (
      maybe.response?.data?.error?.message ||
      maybe.error?.message ||
      maybe.message ||
      "Request failed."
    );
  }
  return "Request failed.";
}

export const meApi = {
  // ---- Profile ----
  getProfile() {
    return unwrapData(() => getCurrentUser({ throwOnError: true }));
  },
  updateProfile(body: UpdateCurrentUserProfileRequest) {
    return unwrapData(() => updateCurrentUserProfile({ body, throwOnError: true }));
  },

  // ---- Security ----
  async changePassword(body: ChangeCurrentUserPasswordRequest): Promise<void> {
    configureClient();
    await changeCurrentUserPassword({ body, throwOnError: true });
  },
  getTotpStatus() {
    return unwrapData(() => getCurrentUserTotpStatus({ throwOnError: true }));
  },
  async revokeAllSessions(): Promise<void> {
    configureClient();
    await revokeAllCurrentUserSessions({ throwOnError: true });
  },
  setupTotp() {
    return unwrapData(() => setupCurrentUserTotp({ throwOnError: true }));
  },
  enableTotp(body: TotpVerifyRequest) {
    return unwrapData(() => enableCurrentUserTotp({ body, throwOnError: true }));
  },
  disableTotp(body: TotpVerifyRequest) {
    return unwrapData(() => disableCurrentUserTotp({ body, throwOnError: true }));
  },

  // ---- Billing ----
  getBalance() {
    return unwrapData(() => getCurrentUserBalance({ throwOnError: true }));
  },
  listPlatformQuotas(): Promise<MeListResult<UserPlatformQuota>> {
    return unwrapList(() => listCurrentUserPlatformQuotas({ throwOnError: true }));
  },
  getPlaygroundModels(): Promise<PlaygroundModel[]> {
    return unwrapData(() => listMePlaygroundModels({ throwOnError: true }));
  },
  listPaymentMethods() {
    return unwrapList(() => listPaymentMethods({ throwOnError: true }));
  },
  // Public storefront catalog: only for_sale + active plans. No auth required —
  // safe to call from the marketing /pricing page before sign-in.
  listSubscriptionPlans() {
    return unwrapList(() => listSubscriptionPlans({ throwOnError: true }));
  },
  listOrders(query?: ListPaymentOrdersData["query"]) {
    return unwrapList(() => listPaymentOrders({ query, throwOnError: true }));
  },
  // Authenticated billing ledger for the session user. Scoped at the DB layer
  // by user_id so we never leak other users' rows.
  listBillingHistory(query?: GetCurrentUserBillingHistoryData["query"]) {
    return unwrapList(() => getCurrentUserBillingHistory({ query, throwOnError: true }));
  },
  getOrder(id: string) {
    return unwrapData(() => getPaymentOrder({ path: { id }, throwOnError: true }));
  },
  createOrder(body: CreatePaymentOrderRequest) {
    return unwrapData(() => createPaymentOrder({ body, throwOnError: true }));
  },
  cancelOrder(id: string) {
    return unwrapData(() => cancelPaymentOrder({ path: { id }, throwOnError: true }));
  },
  getSubscriptions() {
    return unwrapList(() => getCurrentUserSubscriptions({ throwOnError: true }));
  },

  // ---- Redeem ----
  redeemCode(body: RedeemCodeRedemptionRequest) {
    return unwrapData(() => redeemCurrentUserRedeemCode({ body, throwOnError: true }));
  },

  // ---- Affiliate ----
  getAffiliate() {
    return unwrapData(() => getCurrentUserAffiliate({ throwOnError: true }));
  },
  listAffiliateInviteCodes() {
    return unwrapList(() => listCurrentUserAffiliateInviteCodes({ throwOnError: true }));
  },
  createAffiliateInviteCode(body?: CreateAffiliateInviteCodeRequest) {
    return unwrapData(() => createCurrentUserAffiliateInviteCode({ body, throwOnError: true }));
  },
  listAffiliateLedger(query?: ListCurrentUserAffiliateLedgerData["query"]) {
    return unwrapList(() => listCurrentUserAffiliateLedger({ query, throwOnError: true }));
  },
  transferToBalance(body: AffiliateTransferToBalanceRequest) {
    return unwrapData(() =>
      transferCurrentUserAffiliateToBalance({
        body,
        headers: { "Idempotency-Key": crypto.randomUUID() },
        throwOnError: true,
      }),
    );
  },
  requestAffiliateWithdrawal(body: AffiliateWithdrawalRequest) {
    return unwrapData(() =>
      requestCurrentUserAffiliateWithdrawal({
        body,
        headers: { "Idempotency-Key": crypto.randomUUID() },
        throwOnError: true,
      }),
    );
  },

  // ---- Notification preferences (per-event opt in/out) ----
  listNotificationPreferences() {
    return unwrapList(() => getCurrentUserNotificationPreferences({ throwOnError: true }));
  },
  updateNotificationPreferences(body: UpdateNotificationPreferencesRequest) {
    return unwrapList(() =>
      updateCurrentUserNotificationPreferences({ body, throwOnError: true }),
    );
  },

  // ---- Notification contacts (secondary verified emails) ----
  listNotificationContacts() {
    return unwrapList(() => listCurrentUserNotificationContacts({ throwOnError: true }));
  },
  requestNotificationContactVerification(body: NotificationContactVerificationRequest) {
    return unwrapData(() =>
      requestCurrentUserNotificationContactVerification({ body, throwOnError: true }),
    );
  },
  confirmNotificationContact(body: NotificationContactConfirmRequest) {
    return unwrapData(() =>
      confirmCurrentUserNotificationContactVerification({ body, throwOnError: true }),
    );
  },
  updateNotificationContact(id: string, body: UpdateNotificationContactRequest) {
    return unwrapData(() =>
      updateCurrentUserNotificationContact({ path: { id }, body, throwOnError: true }),
    );
  },
  async deleteNotificationContact(id: string): Promise<void> {
    configureClient();
    await deleteCurrentUserNotificationContact({ path: { id }, throwOnError: true });
  },

  // ---- Linked sign-in identities (OAuth/OIDC) ----
  listAuthIdentities() {
    return unwrapList(() => listCurrentUserAuthIdentities({ throwOnError: true }));
  },
  async unbindAuthIdentity(id: string): Promise<void> {
    configureClient();
    await unbindCurrentUserAuthIdentity({ path: { id }, throwOnError: true });
  },

  // ---- Avatar ----
  uploadAvatar(file: File) {
    return unwrapData(() =>
      uploadCurrentUserAvatar({ body: { avatar: file }, throwOnError: true }),
    );
  },
  async deleteAvatar(): Promise<void> {
    configureClient();
    await deleteCurrentUserAvatar({ throwOnError: true });
  },

  // ---- Announcements ----
  // The list endpoint returns the unread count alongside data, so we surface
  // the full envelope rather than going through the generic `unwrapList`.
  async listAnnouncements(): Promise<MeAnnouncementsResult> {
    configureClient();
    const response = await listCurrentUserAnnouncements({ throwOnError: true });
    const body = response.data;
    if (!body || !Array.isArray(body.data)) {
      throw new Error("Request returned an empty list response.");
    }
    return { data: body.data, unread: body.unread ?? 0, pagination: body.pagination };
  },
  async markAnnouncementRead(id: string): Promise<void> {
    configureClient();
    await markCurrentUserAnnouncementRead({ path: { id }, throwOnError: true });
  },
};
