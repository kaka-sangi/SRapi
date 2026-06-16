"use client";

import {
  createAdminUser,
  createAdminUserAttributeDefinition,
  deleteAdminUser,
  deleteAdminUserAttributeDefinition,
  listAdminUserAttributeDefinitions,
  listAdminUserAttributeValues,
  batchListAdminUserAttributeValues,
  setAdminUserAttributeValue,
  updateAdminUserAttributeDefinition,
  listAdminUserPlatformQuotas,
  upsertAdminUserPlatformQuota,
  deleteAdminUserPlatformQuota,
  disableAdminUser,
  enableAdminUser,
  listAdminUsers,
  updateAdminUser,
  updateAdminUserBalance,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  CreateUserAttributeDefinitionRequest,
  UserAttributeDefinition,
  UserAttributeValue,
  UserAttributeValueWithUserId,
  UpdateUserAttributeDefinitionRequest,
  UserPlatformQuota,
  UpsertUserPlatformQuotaRequest,
  CreateAdminUserData,
  Id,
  ListAdminUsersData,
  SetUserAttributeValueRequest,
  UpdateAdminUserData,
  UpdateUserBalanceRequest,
  User,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const usersApi = {
  listUsers(query?: ListAdminUsersData["query"]): Promise<AdminListResult<User>> {
    return unwrapList(() => listAdminUsers({ query, throwOnError: true }));
  },

  createUser(body: CreateAdminUserData["body"]): Promise<User> {
    return unwrapData(() => createAdminUser({ body, throwOnError: true }));
  },

  updateUser(id: Id, body: UpdateAdminUserData["body"]): Promise<User> {
    return unwrapData(() => updateAdminUser({ path: { id }, body, throwOnError: true }));
  },

  deleteUser(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminUser({ path: { id }, throwOnError: true }));
  },

  updateUserBalance(id: Id, body: UpdateUserBalanceRequest): Promise<User> {
    return unwrapData(() => updateAdminUserBalance({ path: { id }, body, throwOnError: true }));
  },

  setUserEnabled(user: User): Promise<User> {
    if (user.status === "disabled") {
      return unwrapData(() => enableAdminUser({ path: { id: user.id }, throwOnError: true }));
    }
    return unwrapData(() => disableAdminUser({ path: { id: user.id }, throwOnError: true }));
  },

  setUserEnabledById(id: Id, enabled: boolean): Promise<User> {
    return enabled
      ? unwrapData(() => enableAdminUser({ path: { id }, throwOnError: true }))
      : unwrapData(() => disableAdminUser({ path: { id }, throwOnError: true }));
  },

  listUserAttributeDefinitions(): Promise<AdminListResult<UserAttributeDefinition>> {
    return unwrapList(() => listAdminUserAttributeDefinitions({ throwOnError: true }));
  },

  createUserAttributeDefinition(
    body: CreateUserAttributeDefinitionRequest,
  ): Promise<UserAttributeDefinition> {
    return unwrapData(() => createAdminUserAttributeDefinition({ body, throwOnError: true }));
  },

  updateUserAttributeDefinition(
    id: Id,
    body: UpdateUserAttributeDefinitionRequest,
  ): Promise<UserAttributeDefinition> {
    return unwrapData(() =>
      updateAdminUserAttributeDefinition({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteUserAttributeDefinition(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() =>
      deleteAdminUserAttributeDefinition({ path: { id }, throwOnError: true }),
    );
  },

  listUserAttributeValues(userId: Id): Promise<AdminListResult<UserAttributeValue>> {
    return unwrapList(() =>
      listAdminUserAttributeValues({ path: { id: userId }, throwOnError: true }),
    );
  },

  batchListUserAttributeValues(userIds: Id[]): Promise<UserAttributeValueWithUserId[]> {
    if (userIds.length === 0) return Promise.resolve([]);
    return unwrapData(() =>
      batchListAdminUserAttributeValues({
        query: { user_ids: userIds.join(",") },
        throwOnError: true,
      }),
    );
  },

  setUserAttributeValue(
    userId: Id,
    definitionId: Id,
    body: SetUserAttributeValueRequest,
  ): Promise<{ definition_id: number; value: string; updated_at?: string }> {
    return unwrapData(() =>
      setAdminUserAttributeValue({
        path: { id: userId, definitionId },
        body,
        throwOnError: true,
      }),
    );
  },

  listUserPlatformQuotas(userId: Id): Promise<AdminListResult<UserPlatformQuota>> {
    return unwrapList(() => listAdminUserPlatformQuotas({ path: { id: userId }, throwOnError: true }));
  },

  upsertUserPlatformQuota(
    userId: Id,
    body: UpsertUserPlatformQuotaRequest,
  ): Promise<UserPlatformQuota> {
    return unwrapData(() =>
      upsertAdminUserPlatformQuota({ path: { id: userId }, body, throwOnError: true }),
    );
  },

  deleteUserPlatformQuota(userId: Id, platform: string): Promise<{ deleted: boolean }> {
    return unwrapData(() =>
      deleteAdminUserPlatformQuota({ path: { id: userId, platform }, throwOnError: true }),
    );
  },
};
