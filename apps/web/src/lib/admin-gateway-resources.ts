import type { AccountHealthSnapshot, ApiKey, Model, Provider, ProviderAccount } from "@/lib/sdk-types";

export interface GatewayProviderResourceRow {
  provider: Provider;
  totalAccounts: number;
  routableAccounts: number;
  attentionAccounts: number;
  apiKeyCount: number;
  scopedKeyCount: number;
  status: "ready" | "limited" | "blocked";
  reasons: string[];
}

type GatewayProviderResourceStatus = GatewayProviderResourceRow["status"];

export interface GatewayResourceSummary {
  providers: number;
  activeProviders: number;
  activeModels: number;
  activeApiKeys: number;
  activeAccounts: number;
  routableAccounts: number;
  scopedApiKeys: number;
  rows: GatewayProviderResourceRow[];
}

export function buildGatewayResourceSummary({
  providers,
  accounts,
  health,
  apiKeys,
  models,
}: {
  providers: Provider[];
  accounts: ProviderAccount[];
  health: AccountHealthSnapshot[];
  apiKeys: ApiKey[];
  models: Model[];
}): GatewayResourceSummary {
  const healthByAccount = new Map(health.map((item) => [String(item.account_id), item]));
  const activeKeys = apiKeys.filter((key) => key.status === "active");
  const activeModels = models.filter((model) => model.status === "active");
  const scopedKeyCount = activeKeys.filter(
    (key) => key.allowed_models.length > 0 || key.group_ids.length > 0,
  ).length;

  const rows = providers.map((provider) => {
    const providerAccounts = accounts.filter((account) => account.provider_id === provider.id);
    const activeAccounts = providerAccounts.filter((account) => account.status === "active");
    const routableAccounts = activeAccounts.filter((account) =>
      accountIsRoutable(account, healthByAccount.get(String(account.id))),
    );
    const attentionAccounts = activeAccounts.length - routableAccounts.length;
    const providerGroupIds = new Set(providerAccounts.flatMap((account) => account.group_ids.map(String)));
    const providerKeys = activeKeys.filter(
      (key) =>
        key.group_ids.length === 0 ||
        key.group_ids.some((groupId) => providerGroupIds.has(String(groupId))),
    );
    const scopedProviderKeys = providerKeys.filter(
      (key) => key.allowed_models.length > 0 || key.group_ids.length > 0,
    );
    const reasons: string[] = [];
    if (provider.status !== "active") reasons.push("provider_disabled");
    if (activeModels.length === 0) reasons.push("no_active_models");
    if (activeAccounts.length === 0) reasons.push("no_active_accounts");
    else if (routableAccounts.length === 0) reasons.push("no_routable_accounts");
    if (providerKeys.length === 0) reasons.push("no_api_keys");

    const status: GatewayProviderResourceStatus =
      reasons.length === 0 ? "ready" : routableAccounts.length > 0 ? "limited" : "blocked";

    return {
      provider,
      totalAccounts: providerAccounts.length,
      routableAccounts: routableAccounts.length,
      attentionAccounts,
      apiKeyCount: providerKeys.length,
      scopedKeyCount: scopedProviderKeys.length,
      status,
      reasons,
    };
  });

  return {
    providers: providers.length,
    activeProviders: providers.filter((provider) => provider.status === "active").length,
    activeModels: activeModels.length,
    activeApiKeys: activeKeys.length,
    activeAccounts: accounts.filter((account) => account.status === "active").length,
    routableAccounts: rows.reduce((sum, row) => sum + row.routableAccounts, 0),
    scopedApiKeys: scopedKeyCount,
    rows,
  };
}

function accountIsRoutable(account: ProviderAccount, health?: AccountHealthSnapshot): boolean {
  if (account.status !== "active") return false;
  if (!health) return true;
  if (health.quota_exhausted) return false;
  if (health.circuit_state === "open") return false;
  if (health.status === "dead" || health.status === "suspended") return false;
  return true;
}
