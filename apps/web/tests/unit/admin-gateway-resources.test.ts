import { describe, expect, it } from "vitest";
import { buildGatewayResourceSummary } from "@/lib/admin-gateway-resources";
import type { AccountHealthSnapshot, ApiKey, Model, Provider, ProviderAccount } from "@/lib/sdk-types";

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
