"use client";

import {
  createAdminErrorPassthroughRule,
  deleteAdminErrorPassthroughRule,
  listAdminErrorPassthroughRules,
  updateAdminErrorPassthroughRule,
  createAdminPayloadRule,
  deleteAdminPayloadRule,
  listAdminPayloadRules,
  updateAdminPayloadRule,
  createAdminScheduledTestPlan,
  deleteAdminScheduledTestPlan,
  listAdminScheduledTestPlans,
  listAdminScheduledTestPlanRuns,
  runAdminScheduledTestPlan,
  updateAdminScheduledTestPlan,
  listAdminChannelMonitors,
  createAdminChannelMonitor,
  updateAdminChannelMonitor,
  deleteAdminChannelMonitor,
  runAdminChannelMonitor,
  listAdminChannelMonitorRuns,
  listAdminChannelMonitorTemplates,
  createAdminChannelMonitorTemplate,
  updateAdminChannelMonitorTemplate,
  deleteAdminChannelMonitorTemplate,
  applyAdminChannelMonitorTemplate,
  createAdminTlsProfile,
  deleteAdminTlsProfile,
  listAdminTlsProfiles,
  updateAdminTlsProfile,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  CreateErrorPassthroughRuleRequest,
  ErrorPassthroughRule,
  UpdateErrorPassthroughRuleRequest,
  CreatePayloadRuleRequest,
  PayloadRule,
  UpdatePayloadRuleRequest,
  CreateScheduledTestPlanRequest,
  ScheduledTestPlan,
  ScheduledTestPlanRun,
  UpdateScheduledTestPlanRequest,
  ChannelMonitor,
  CreateChannelMonitorRequest,
  UpdateChannelMonitorRequest,
  ChannelMonitorRun,
  ChannelMonitorTemplate,
  CreateChannelMonitorTemplateRequest,
  UpdateChannelMonitorTemplateRequest,
  CreateTlsProfileRequest,
  TlsProfile,
  UpdateTlsProfileRequest,
  Id,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const monitoringApi = {
  listErrorPassthroughRules(): Promise<AdminListResult<ErrorPassthroughRule>> {
    return unwrapList(() => listAdminErrorPassthroughRules({ throwOnError: true }));
  },

  createErrorPassthroughRule(
    body: CreateErrorPassthroughRuleRequest,
  ): Promise<ErrorPassthroughRule> {
    return unwrapData(() => createAdminErrorPassthroughRule({ body, throwOnError: true }));
  },

  updateErrorPassthroughRule(
    id: Id,
    body: UpdateErrorPassthroughRuleRequest,
  ): Promise<ErrorPassthroughRule> {
    return unwrapData(() =>
      updateAdminErrorPassthroughRule({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteErrorPassthroughRule(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminErrorPassthroughRule({ path: { id }, throwOnError: true }));
  },

  listPayloadRules(): Promise<AdminListResult<PayloadRule>> {
    return unwrapList(() => listAdminPayloadRules({ throwOnError: true }));
  },

  createPayloadRule(body: CreatePayloadRuleRequest): Promise<PayloadRule> {
    return unwrapData(() => createAdminPayloadRule({ body, throwOnError: true }));
  },

  updatePayloadRule(id: Id, body: UpdatePayloadRuleRequest): Promise<PayloadRule> {
    return unwrapData(() => updateAdminPayloadRule({ path: { id }, body, throwOnError: true }));
  },

  deletePayloadRule(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminPayloadRule({ path: { id }, throwOnError: true }));
  },

  listScheduledTestPlans(): Promise<AdminListResult<ScheduledTestPlan>> {
    return unwrapList(() => listAdminScheduledTestPlans({ throwOnError: true }));
  },

  createScheduledTestPlan(
    body: CreateScheduledTestPlanRequest,
  ): Promise<ScheduledTestPlan> {
    return unwrapData(() => createAdminScheduledTestPlan({ body, throwOnError: true }));
  },

  updateScheduledTestPlan(
    id: Id,
    body: UpdateScheduledTestPlanRequest,
  ): Promise<ScheduledTestPlan> {
    return unwrapData(() =>
      updateAdminScheduledTestPlan({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteScheduledTestPlan(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminScheduledTestPlan({ path: { id }, throwOnError: true }));
  },

  listScheduledTestPlanRuns(
    id: Id,
    limit?: number,
  ): Promise<AdminListResult<ScheduledTestPlanRun>> {
    return unwrapList(() =>
      listAdminScheduledTestPlanRuns({
        path: { id },
        query: limit ? { limit } : {},
        throwOnError: true,
      }),
    );
  },

  listChannelMonitors(): Promise<AdminListResult<ChannelMonitor>> {
    return unwrapList(() => listAdminChannelMonitors({ throwOnError: true }));
  },

  createChannelMonitor(body: CreateChannelMonitorRequest): Promise<ChannelMonitor> {
    return unwrapData(() => createAdminChannelMonitor({ body, throwOnError: true }));
  },

  updateChannelMonitor(id: Id, body: UpdateChannelMonitorRequest): Promise<ChannelMonitor> {
    return unwrapData(() => updateAdminChannelMonitor({ path: { id }, body, throwOnError: true }));
  },

  deleteChannelMonitor(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminChannelMonitor({ path: { id }, throwOnError: true }));
  },

  runChannelMonitor(id: Id): Promise<ChannelMonitorRun> {
    return unwrapData(() => runAdminChannelMonitor({ path: { id }, throwOnError: true }));
  },

  listChannelMonitorRuns(id: Id, limit?: number): Promise<AdminListResult<ChannelMonitorRun>> {
    return unwrapList(() =>
      listAdminChannelMonitorRuns({ path: { id }, query: { limit }, throwOnError: true }),
    );
  },

  listChannelMonitorTemplates(): Promise<AdminListResult<ChannelMonitorTemplate>> {
    return unwrapList(() => listAdminChannelMonitorTemplates({ throwOnError: true }));
  },

  createChannelMonitorTemplate(
    body: CreateChannelMonitorTemplateRequest,
  ): Promise<ChannelMonitorTemplate> {
    return unwrapData(() => createAdminChannelMonitorTemplate({ body, throwOnError: true }));
  },

  updateChannelMonitorTemplate(
    id: Id,
    body: UpdateChannelMonitorTemplateRequest,
  ): Promise<ChannelMonitorTemplate> {
    return unwrapData(() =>
      updateAdminChannelMonitorTemplate({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteChannelMonitorTemplate(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminChannelMonitorTemplate({ path: { id }, throwOnError: true }));
  },

  applyChannelMonitorTemplate(
    id: Id,
    monitorIds: number[],
  ): Promise<AdminListResult<ChannelMonitor>> {
    return unwrapList(() =>
      applyAdminChannelMonitorTemplate({
        path: { id },
        body: { monitor_ids: monitorIds },
        throwOnError: true,
      }),
    );
  },

  runScheduledTestPlan(id: Id): Promise<ScheduledTestPlanRun> {
    return unwrapData(() => runAdminScheduledTestPlan({ path: { id }, throwOnError: true }));
  },

  listTlsProfiles(): Promise<AdminListResult<TlsProfile>> {
    return unwrapList(() => listAdminTlsProfiles({ throwOnError: true }));
  },

  createTlsProfile(body: CreateTlsProfileRequest): Promise<TlsProfile> {
    return unwrapData(() => createAdminTlsProfile({ body, throwOnError: true }));
  },

  updateTlsProfile(id: Id, body: UpdateTlsProfileRequest): Promise<TlsProfile> {
    return unwrapData(() => updateAdminTlsProfile({ path: { id }, body, throwOnError: true }));
  },

  deleteTlsProfile(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminTlsProfile({ path: { id }, throwOnError: true }));
  },
};
