"use client";

import {
  listAdminModels,
  createAdminModel,
  createAdminModelAlias,
  quickMapAdminModels,
  listAdminModelAliases,
  deleteAdminModelAlias,
  createAdminModelMapping,
  listAdminModelMappings,
  deleteAdminModelMapping,
  updateAdminModel,
  deleteAdminModel,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AdminQuickMapModelsResult,
  Id,
  ListAdminModelsData,
  Model,
  ModelAlias,
  ModelProviderMapping,
  QuickMapAdminModelsData,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const modelsApi = {
  listModels(query?: ListAdminModelsData["query"]): Promise<AdminListResult<Model>> {
    return unwrapList(() => listAdminModels({ query, throwOnError: true }));
  },

  quickMapModels(body: QuickMapAdminModelsData["body"]): Promise<AdminQuickMapModelsResult> {
    return unwrapData(() => quickMapAdminModels({ body, throwOnError: true }));
  },

  createModel(body: Parameters<typeof createAdminModel>[0]["body"]): Promise<Model> {
    return unwrapData(() => createAdminModel({ body, throwOnError: true }));
  },

  updateModel(id: Id, body: Parameters<typeof updateAdminModel>[0]["body"]): Promise<Model> {
    return unwrapData(() => updateAdminModel({ path: { id }, body, throwOnError: true }));
  },

  deleteModel(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminModel({ path: { id }, throwOnError: true }));
  },
  createModelAlias(
    id: Id,
    body: Parameters<typeof createAdminModelAlias>[0]["body"],
  ): Promise<ModelAlias> {
    return unwrapData(() => createAdminModelAlias({ path: { id }, body, throwOnError: true }));
  },
  createModelMapping(
    id: Id,
    body: Parameters<typeof createAdminModelMapping>[0]["body"],
  ): Promise<ModelProviderMapping> {
    return unwrapData(() => createAdminModelMapping({ path: { id }, body, throwOnError: true }));
  },
  listModelAliases(id: Id): Promise<AdminListResult<ModelAlias>> {
    return unwrapList(() => listAdminModelAliases({ path: { id }, throwOnError: true }));
  },
  deleteModelAlias(id: Id, aliasId: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminModelAlias({ path: { id, aliasId }, throwOnError: true }));
  },
  listModelMappings(id: Id): Promise<AdminListResult<ModelProviderMapping>> {
    return unwrapList(() => listAdminModelMappings({ path: { id }, throwOnError: true }));
  },
  deleteModelMapping(id: Id, mappingId: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminModelMapping({ path: { id, mappingId }, throwOnError: true }));
  },
};
