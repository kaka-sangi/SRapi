"use client";

import { getAdminGatewayResources } from "../../../../../packages/sdk/typescript/src/index";
import type { GatewayResourceSummary } from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData } from "./_shared";

export const gatewayResourcesApi = {
  getGatewayResources(): Promise<GatewayResourceSummary> {
    return unwrapData(() => getAdminGatewayResources({ throwOnError: true }));
  },
};
