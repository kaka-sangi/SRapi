"use client";

import {
  createAdminAffiliateRule,
  listAdminAffiliateInvites,
  listAdminAffiliateRebates,
  listAdminAffiliateRules,
  listAdminAffiliateTransfers,
  updateAdminAffiliateRule,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AffiliateInviteRecord,
  AffiliateLedgerEntry,
  AffiliateRule,
  CreateAffiliateRuleRequest,
  Id,
  ListAdminAffiliateInvitesData,
  ListAdminAffiliateRebatesData,
  ListAdminAffiliateRulesData,
  ListAdminAffiliateTransfersData,
  UpdateAffiliateRuleRequest,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const affiliateApi = {
  listAffiliateInvites(
    query?: ListAdminAffiliateInvitesData["query"],
  ): Promise<AdminListResult<AffiliateInviteRecord>> {
    return unwrapList(() => listAdminAffiliateInvites({ query, throwOnError: true }));
  },

  listAffiliateRebates(
    query?: ListAdminAffiliateRebatesData["query"],
  ): Promise<AdminListResult<AffiliateLedgerEntry>> {
    return unwrapList(() => listAdminAffiliateRebates({ query, throwOnError: true }));
  },

  listAffiliateTransfers(
    query?: ListAdminAffiliateTransfersData["query"],
  ): Promise<AdminListResult<AffiliateLedgerEntry>> {
    return unwrapList(() => listAdminAffiliateTransfers({ query, throwOnError: true }));
  },

  listAffiliateRules(
    query?: ListAdminAffiliateRulesData["query"],
  ): Promise<AdminListResult<AffiliateRule>> {
    return unwrapList(() => listAdminAffiliateRules({ query, throwOnError: true }));
  },

  createAffiliateRule(body: CreateAffiliateRuleRequest): Promise<AffiliateRule> {
    return unwrapData(() => createAdminAffiliateRule({ body, throwOnError: true }));
  },

  updateAffiliateRule(id: Id, body: UpdateAffiliateRuleRequest): Promise<AffiliateRule> {
    return unwrapData(() => updateAdminAffiliateRule({ path: { id }, body, throwOnError: true }));
  },
};
