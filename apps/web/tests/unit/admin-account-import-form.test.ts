import { describe, expect, it } from "vitest";
import {
  buildImportAccountsBodyFromValue,
  buildImportAccountsBody,
} from "@/lib/admin-account-form";

describe("buildImportAccountsBody", () => {
  it("normalizes sub2api account exports into SRapi import rows", () => {
    const body = buildImportAccountsBodyFromValue(
      {
        exported_at: "2026-06-20T12:51:24Z",
        proxies: [],
        accounts: [
          sub2apiAccount("alice@example.test", "user-a", "refresh-a"),
          sub2apiAccount("bob@example.test", "user-b", "refresh-b"),
          sub2apiAccount("carol@example.test", "user-c", "refresh-c"),
        ],
      },
      {
        defaultProviderId: "codex-provider",
        defaultRuntimeClass: "oauth_refresh",
        defaultUpstreamClient: "codex_cli",
      },
    );

    expect(body.accounts).toHaveLength(3);
    expect(body.accounts.map((account) => account.name)).toEqual([
      "alice@example.test",
      "bob@example.test",
      "carol@example.test",
    ]);
    expect(body.accounts.map((account) => account.provider_id)).toEqual([
      "codex-provider",
      "codex-provider",
      "codex-provider",
    ]);
    expect(body.accounts.map((account) => account.runtime_class)).toEqual([
      "oauth_refresh",
      "oauth_refresh",
      "oauth_refresh",
    ]);
    expect(body.accounts.map((account) => account.upstream_client)).toEqual([
      "codex_cli",
      "codex_cli",
      "codex_cli",
    ]);
    expect(body.accounts.map((account) => account.credential?.refresh_token)).toEqual([
      "refresh-a",
      "refresh-b",
      "refresh-c",
    ]);
    expect(body.accounts.map((account) => account.metadata?.chatgpt_user_id)).toEqual([
      "user-a",
      "user-b",
      "user-c",
    ]);
    expect(body.accounts.map((account) => account.metadata?.chatgpt_account_id)).toEqual([
      "workspace-1",
      "workspace-1",
      "workspace-1",
    ]);
    expect(body.accounts[0]).toEqual(
      expect.objectContaining({
        priority: 1,
        weight: 1,
        metadata: expect.objectContaining({
          auto_pause_on_expired: true,
          max_concurrency: 10,
          source: "sub_bundle_input",
        }),
      }),
    );
  });

  it("keeps native SRapi import JSON unchanged", () => {
    const json = JSON.stringify({
      accounts: [
        {
          provider_id: "1",
          name: "native",
          runtime_class: "api_key",
          credential: { api_key: "sk-native" },
          metadata: { base_url: "https://api.example.test/v1" },
        },
      ],
    });

    expect(buildImportAccountsBody(json)).toEqual({
      accounts: [
        {
          provider_id: "1",
          name: "native",
          runtime_class: "api_key",
          credential: { api_key: "sk-native" },
          metadata: { base_url: "https://api.example.test/v1" },
        },
      ],
    });
  });
});

function sub2apiAccount(email: string, userID: string, refreshToken: string) {
  return {
    name: email,
    platform: "openai",
    type: "oauth",
    credentials: {
      access_token: `access-${userID}`,
      refresh_token: refreshToken,
      chatgpt_account_id: "workspace-1",
      chatgpt_user_id: userID,
      email,
      organization_id: `org-${userID}`,
      plan_type: "k12",
    },
    extra: {
      email,
      source: "sub_bundle_input",
      codex_5h_used_percent: 1,
    },
    concurrency: 10,
    priority: 1,
    rate_multiplier: 1,
    auto_pause_on_expired: true,
  };
}
