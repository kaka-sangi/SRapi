"use client";

import {
  getAdminContentSafetyConfig,
  getAdminRiskControlConfig,
  getAdminRiskControlStatus,
  listAdminRiskControlLogs,
  updateAdminContentSafetyConfig,
  updateAdminRiskControlConfig,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  ContentSafetyConfig,
  ListAdminRiskControlLogsData,
  RiskControlConfig,
  RiskControlLog,
  RiskControlStatus,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const riskApi = {
  getRiskConfig(): Promise<RiskControlConfig> {
    return unwrapData(() => getAdminRiskControlConfig({ throwOnError: true }));
  },

  updateRiskConfig(body: RiskControlConfig): Promise<RiskControlConfig> {
    return unwrapData(() => updateAdminRiskControlConfig({ body, throwOnError: true }));
  },

  getRiskStatus(): Promise<RiskControlStatus> {
    return unwrapData(() => getAdminRiskControlStatus({ throwOnError: true }));
  },

  listRiskLogs(query?: ListAdminRiskControlLogsData["query"]): Promise<AdminListResult<RiskControlLog>> {
    return unwrapList(() => listAdminRiskControlLogs({ query, throwOnError: true }));
  },

  getContentSafetyConfig(): Promise<ContentSafetyConfig> {
    return unwrapData(() => getAdminContentSafetyConfig({ throwOnError: true }));
  },

  updateContentSafetyConfig(body: ContentSafetyConfig): Promise<ContentSafetyConfig> {
    return unwrapData(() => updateAdminContentSafetyConfig({ body, throwOnError: true }));
  },
};
