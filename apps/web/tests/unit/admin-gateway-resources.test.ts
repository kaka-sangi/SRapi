import { describe, expect, it } from "vitest";
import { buildGatewayResourceSummary } from "@/lib/admin-gateway-resources";
import type {
  AccountHealthSnapshot,
  ApiKey,
  Model,
  Provider,
  ProviderAccount,
  ProxyDefinition,
} from "@/lib/sdk-types";

describe("buildGatewayResourceSummary", () => {
  it("marks providers ready only when active accounts, models, and keys can route", () => {
    const summary = buildGatewayResourceSummary({
      providers: [
        provider({ id: "p1", name: "openai" }),
        provider({ id: "p2", name: "codex" }),
      ],
      accounts: [
        account({ id: "a1", provider_id: "p1", group_ids: ["g1"] }),
        account({ id: "a2", provider_id: "p2", group_ids: ["g2"] }),
      ],
      health: [
        health({ account_id: "a1", provider_id: "p1" }),
        health({ account_id: "a2", provider_id: "p2", quota_exhausted: true }),
      ],
      apiKeys: [
        apiKey({ id: "k1", group_ids: ["g1"] }),
      ],
      models: [model({ id: "m1" })],
    });

    expect(summary.activeProviders).toBe(2);
    expect(summary.routableAccounts).toBe(1);
    expect(summary.rows).toEqual([
      expect.objectContaining({
        provider: expect.objectContaining({ id: "p1" }),
        status: "ready",
        routableAccounts: 1,
        reasons: [],
      }),
      expect.objectContaining({
        provider: expect.objectContaining({ id: "p2" }),
        status: "blocked",
        routableAccounts: 0,
        reasons: ["no_routable_accounts", "no_api_keys"],
      }),
    ]);
  });

  it("treats unscoped active keys as available to every provider", () => {
    const summary = buildGatewayResourceSummary({
      providers: [provider({ id: "p1" })],
      accounts: [account({ id: "a1", provider_id: "p1" })],
      health: [],
      apiKeys: [apiKey({ id: "k1", group_ids: [], allowed_models: [] })],
      models: [model({ id: "m1" })],
    });

    expect(summary.rows[0]).toEqual(
      expect.objectContaining({
        apiKeyCount: 1,
        scopedKeyCount: 0,
        status: "ready",
      }),
    );
  });

  it("counts proxy availability and flags accounts bound to unavailable proxies", () => {
    const summary = buildGatewayResourceSummary({
      providers: [provider({ id: "p1" })],
      accounts: [
        account({ id: "a1", provider_id: "p1", proxy_id: "1" }),
        account({ id: "a2", provider_id: "p1", proxy_id: "2" }),
        account({ id: "a3", provider_id: "p1", proxy_id: "4" }),
      ],
      health: [],
      apiKeys: [apiKey({ id: "k1" })],
      models: [model({ id: "m1" })],
      proxies: [
        proxy({ id: "1", name: "primary" }),
        proxy({
          id: "2",
          name: "expired-with-backup",
          expires_at: "2026-06-19T00:00:00Z",
          fallback_mode: "proxy",
          backup_proxy_id: "3",
        }),
        proxy({ id: "3", name: "backup" }),
        proxy({ id: "4", name: "disabled", status: "disabled" }),
      ],
      nowMs: Date.parse("2026-06-20T00:00:00Z"),
    });

    expect(summary.activeProxies).toBe(3);
    expect(summary.availableProxies).toBe(3);
    expect(summary.expiredProxies).toBe(1);
    expect(summary.proxiedAccounts).toBe(3);
    expect(summary.proxyAttentionAccounts).toBe(2);
    expect(summary.routableAccounts).toBe(2);
    expect(summary.rows[0]).toEqual(
      expect.objectContaining({
        status: "limited",
        proxiedAccounts: 3,
        proxyAttentionAccounts: 2,
        routableAccounts: 2,
        reasons: ["proxy_attention"],
      }),
    );
  });
});

function provider(overrides: Partial<Provider> = {}): Provider {
  return {
    id: "p1",
    name: "provider",
    display_name: "Provider",
    adapter_type: "openai-compatible",
    protocol: "openai-compatible",
    status: "active",
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function account(overrides: Partial<ProviderAccount> = {}): ProviderAccount {
  return {
    id: "a1",
    provider_id: "p1",
    name: "account",
    runtime_class: "api_key",
    status: "active",
    priority: 0,
    weight: 1,
    group_ids: [],
    proxy_id: null,
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function health(overrides: Partial<AccountHealthSnapshot> = {}): AccountHealthSnapshot {
  return {
    account_id: "a1",
    provider_id: "p1",
    runtime_class: "api_key",
    status: "healthy",
    success_rate: 1,
    error_rate: 0,
    latency_p50_ms: 10,
    latency_p95_ms: 20,
    quota_remaining_ratio: 1,
    quota_exhausted: false,
    rate_limit_count: 0,
    timeout_count: 0,
    circuit_state: "closed",
    snapshot_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function apiKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: "k1",
    name: "key",
    prefix: "sk-test",
    status: "active",
    scopes: ["gateway:invoke"],
    allowed_models: [],
    group_ids: [],
    allowed_ips: [],
    denied_ips: [],
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function model(overrides: Partial<Model> = {}): Model {
  return {
    id: "m1",
    canonical_name: "gpt-test",
    display_name: "GPT Test",
    status: "active",
    capabilities: [],
    created_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}

function proxy(overrides: Partial<ProxyDefinition> = {}): ProxyDefinition {
  return {
    id: "1",
    name: "proxy",
    type: "http",
    status: "active",
    url_configured: true,
    country_code: null,
    country_name: null,
    expires_at: null,
    fallback_mode: "none",
    backup_proxy_id: null,
    last_probed_at: null,
    probe_success_count: 0,
    probe_failure_count: 0,
    last_probe_latency_ms: 0,
    probe_success_pct_7d: null,
    created_at: "2026-06-20T00:00:00Z",
    updated_at: "2026-06-20T00:00:00Z",
    ...overrides,
  };
}
