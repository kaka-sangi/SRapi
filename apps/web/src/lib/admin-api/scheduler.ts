"use client";

import {
  replaySchedulerStrategy as replaySchedulerStrategyFn,
  getAdminSchedulerOverview,
  listSchedulerStrategies as listSchedulerStrategiesFn,
  createSchedulerStrategy as createSchedulerStrategyFn,
  updateSchedulerStrategy as updateSchedulerStrategyFn,
  deprecateSchedulerStrategy as deprecateSchedulerStrategyFn,
  activateSchedulerStrategy as activateSchedulerStrategyFn,
  simulateSchedulerStrategy as simulateSchedulerStrategyFn,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  Id,
  ReplaySchedulerStrategyData,
  SimulateSchedulerStrategyData,
  CreateSchedulerStrategyData,
  UpdateSchedulerStrategyData,
  SchedulerOverview,
  SchedulerStrategy,
  SchedulerReplayResult,
  SchedulerSimulationResult,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const schedulerApi = {
  replaySchedulerStrategy(
    body: ReplaySchedulerStrategyData["body"],
  ): Promise<SchedulerReplayResult> {
    return unwrapData(() => replaySchedulerStrategyFn({ body, throwOnError: true }));
  },
  schedulerOverview(): Promise<SchedulerOverview> {
    return unwrapData(() => getAdminSchedulerOverview({ throwOnError: true }));
  },
  listSchedulerStrategies(): Promise<AdminListResult<SchedulerStrategy>> {
    return unwrapList(() => listSchedulerStrategiesFn({ throwOnError: true }));
  },
  createSchedulerStrategy(body: CreateSchedulerStrategyData["body"]): Promise<SchedulerStrategy> {
    return unwrapData(() => createSchedulerStrategyFn({ body, throwOnError: true }));
  },
  updateSchedulerStrategy(
    id: Id,
    body: UpdateSchedulerStrategyData["body"],
  ): Promise<SchedulerStrategy> {
    return unwrapData(() =>
      updateSchedulerStrategyFn({ path: { id }, body, throwOnError: true }),
    );
  },
  deprecateSchedulerStrategy(id: Id): Promise<SchedulerStrategy> {
    return unwrapData(() => deprecateSchedulerStrategyFn({ path: { id }, throwOnError: true }));
  },
  activateSchedulerStrategy(id: Id): Promise<SchedulerStrategy> {
    return unwrapData(() => activateSchedulerStrategyFn({ path: { id }, throwOnError: true }));
  },
  simulateSchedulerStrategy(
    body: SimulateSchedulerStrategyData["body"],
  ): Promise<SchedulerSimulationResult> {
    return unwrapData(() => simulateSchedulerStrategyFn({ body, throwOnError: true }));
  },
};
