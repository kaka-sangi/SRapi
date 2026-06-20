import type {
  AccountHealthSnapshot,
  ApiKey,
  Model,
  Provider,
  ProviderAccount,
  ProxyDefinition,
} from "@/lib/sdk-types";

export interface GatewayProviderResourceRow {
  provider: Provider;
  totalAccounts: number;
  routableAccounts: number;
  attentionAccounts: number;
  proxiedAccounts: number;
  proxyAttentionAccounts: number;
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
  activeProxies: number;
  availableProxies: number;
  expiredProxies: number;
  proxiedAccounts: number;
  proxyAttentionAccounts: number;
  scopedApiKeys: number;
  rows: GatewayProviderResourceRow[];
}

export function buildGatewayResourceSummary({
  providers,
  accounts,
  health,
  apiKeys,
  models,
  proxies = [],
  nowMs = Date.now(),
}: {
  providers: Provider[];
  accounts: ProviderAccount[];
  health: AccountHealthSnapshot[];
  apiKeys: ApiKey[];
  models: Model[];
  proxies?: ProxyDefinition[];
  nowMs?: number;
}): GatewayResourceSummary {
  const healthByAccount = new Map(health.map((item) => [String(item.account_id), item]));
  const proxyStates = buildProxyRuntimeStates(proxies, nowMs);
  const activeKeys = apiKeys.filter((key) => key.status === "active");
  const activeModels = models.filter((model) => model.status === "active");
  const scopedKeyCount = activeKeys.filter(
    (key) => key.allowed_models.length > 0 || key.group_ids.length > 0,
  ).length;
  const activeProxies = proxies.filter((proxy) => proxy.status === "active");
  const availableProxies = activeProxies.filter(
    (proxy) => proxyStates.get(String(proxy.id))?.available === true,
  ).length;
  const expiredProxies = activeProxies.filter(
    (proxy) => proxyStates.get(String(proxy.id))?.expired === true,
  ).length;

  const rows = providers.map((provider) => {
    const providerAccounts = accounts.filter((account) => account.provider_id === provider.id);
    const activeAccounts = providerAccounts.filter((account) => account.status === "active");
    const routableAccounts = activeAccounts.filter((account) =>
      accountIsRoutable(account, healthByAccount.get(String(account.id))) &&
      accountProxyCanRoute(account, proxyStates),
    );
    const attentionAccounts = activeAccounts.length - routableAccounts.length;
    const proxiedAccounts = activeAccounts.filter((account) => Boolean(accountProxyID(account))).length;
    const proxyAttentionAccounts = activeAccounts.filter((account) =>
      accountNeedsProxyAttention(account, proxyStates),
    ).length;
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
    if (proxyAttentionAccounts > 0) reasons.push("proxy_attention");
    if (providerKeys.length === 0) reasons.push("no_api_keys");

    const status: GatewayProviderResourceStatus =
      reasons.length === 0 ? "ready" : routableAccounts.length > 0 ? "limited" : "blocked";

    return {
      provider,
      totalAccounts: providerAccounts.length,
      routableAccounts: routableAccounts.length,
      attentionAccounts,
      proxiedAccounts,
      proxyAttentionAccounts,
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
    activeProxies: activeProxies.length,
    availableProxies,
    expiredProxies,
    proxiedAccounts: rows.reduce((sum, row) => sum + row.proxiedAccounts, 0),
    proxyAttentionAccounts: rows.reduce((sum, row) => sum + row.proxyAttentionAccounts, 0),
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

type ProxyRuntimeState = {
  proxy: ProxyDefinition;
  available: boolean;
  attention: boolean;
  expired: boolean;
};

type ProxyRuntimeStates = Map<string, ProxyRuntimeState>;

function buildProxyRuntimeStates(proxies: ProxyDefinition[], nowMs: number): ProxyRuntimeStates {
  const byID = new Map(proxies.map((proxy) => [String(proxy.id), proxy]));
  const states: ProxyRuntimeStates = new Map();
  const resolving = new Set<string>();

  const resolve = (id: string): ProxyRuntimeState | undefined => {
    const existing = states.get(id);
    if (existing) return existing;

    const proxy = byID.get(id);
    if (!proxy) return undefined;

    const expired = proxyIsExpired(proxy, nowMs);
    if (resolving.has(id)) {
      return { proxy, available: false, attention: true, expired };
    }

    resolving.add(id);

    const usablePrimary = proxy.status === "active" && proxy.url_configured;
    let available = usablePrimary && !expired;
    if (!available && usablePrimary && expired) {
      const fallbackMode = proxy.fallback_mode ?? "none";
      if (fallbackMode === "direct") {
        available = true;
      } else if (fallbackMode === "proxy") {
        const backupProxyID = trimProxyID(proxy.backup_proxy_id);
        available = Boolean(backupProxyID && resolve(backupProxyID)?.available);
      }
    }

    const state = {
      proxy,
      available,
      attention: proxy.status !== "active" || !proxy.url_configured || expired,
      expired,
    };
    states.set(id, state);
    resolving.delete(id);
    return state;
  };

  for (const proxy of proxies) {
    resolve(String(proxy.id));
  }
  return states;
}

function accountProxyCanRoute(account: ProviderAccount, proxies: ProxyRuntimeStates): boolean {
  const proxyID = accountProxyID(account);
  if (!proxyID) return true;
  return proxies.get(proxyID)?.available === true;
}

function accountNeedsProxyAttention(account: ProviderAccount, proxies: ProxyRuntimeStates): boolean {
  const proxyID = accountProxyID(account);
  if (!proxyID) return false;
  return proxies.get(proxyID)?.attention !== false;
}

function accountProxyID(account: ProviderAccount): string {
  return trimProxyID(account.proxy_id);
}

function proxyIsExpired(proxy: ProxyDefinition, nowMs: number): boolean {
  if (!proxy.expires_at) return false;
  const expiresAt = new Date(proxy.expires_at).getTime();
  if (!Number.isFinite(expiresAt)) return false;
  return expiresAt <= nowMs;
}

function trimProxyID(proxyID: string | null | undefined): string {
  return typeof proxyID === "string" ? proxyID.trim() : "";
}
