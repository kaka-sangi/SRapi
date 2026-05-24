import { describe, expect, it } from "vitest";
import {
  buildCreateAccountBody,
  buildBatchUpdateAccountsBody,
  buildImportAccountsBody,
  buildUpdateAccountBody,
  canConfirmAccountBatchStatus,
  canConfirmAccountModelDiscovery,
  canConfirmAccountProxyBinding,
  createAccountBatchStatusConfirmation,
  createAccountModelDiscoveryConfirmation,
  createAccountProxyBindingConfirmation,
  diffAccountGroupIds,
  emptyAccountForm,
} from "@/lib/admin-account-form";

describe("admin-account-form", () => {
  it("builds create payloads with write-only credential JSON", () => {
    const form = {
      ...emptyAccountForm("provider-1"),
      name: "primary-openai",
      credential: '{ "api_key": "sk-live" }',
      metadata: '{ "base_url": "https://api.openai.com/v1" }',
    };

    expect(buildCreateAccountBody(form)).toMatchObject({
      provider_id: "provider-1",
      name: "primary-openai",
      runtime_class: "api_key",
      credential: { api_key: "sk-live" },
      metadata: { base_url: "https://api.openai.com/v1" },
    });
  });

  it("does not send empty write-only fields while editing", () => {
    const form = {
      ...emptyAccountForm("provider-1"),
      name: "primary-openai",
      credential: "",
      proxyId: "",
    };

    const body = buildUpdateAccountBody(form);
    expect(body).not.toHaveProperty("credential");
    expect(body).not.toHaveProperty("proxy_id");
  });

  it("rejects non-object JSON fields", () => {
    const form = {
      ...emptyAccountForm("provider-1"),
      name: "primary-openai",
      credential: "[]",
    };

    expect(() => buildCreateAccountBody(form)).toThrow("Credential must be a JSON object.");
  });

  it("reports invalid credential JSON explicitly", () => {
    const form = {
      ...emptyAccountForm("provider-1"),
      name: "primary-openai",
      credential: "{not-json",
    };

    expect(() => buildCreateAccountBody(form)).toThrow("Credential must be valid JSON.");
  });

  it("computes only required group membership changes", () => {
    expect(diffAccountGroupIds(["1", "2"], ["2", "3"])).toEqual({
      add: ["3"],
      remove: ["1"],
    });
  });

  it("requires exact confirmation for account proxy binding changes", () => {
    const state = createAccountProxyBindingConfirmation({
      account: { id: "acct-1", name: "primary-openai" },
      currentProxyId: "",
      nextProxyId: "proxy-1",
    });

    expect(state).toMatchObject({
      accountId: "acct-1",
      nextProxyId: "proxy-1",
      action: "bind",
      phrase: "BIND PROXY primary-openai",
    });
    expect(canConfirmAccountProxyBinding({ ...state, confirmation: state.phrase })).toBe(true);
    expect(canConfirmAccountProxyBinding({ ...state, confirmation: "bind proxy primary-openai" })).toBe(false);
  });

  it("builds clear proxy confirmation when the next proxy is empty", () => {
    const state = createAccountProxyBindingConfirmation({
      account: { id: "acct-1", name: "primary-openai" },
      currentProxyId: "proxy-1",
      nextProxyId: "",
    });

    expect(state).toMatchObject({
      nextProxyId: null,
      action: "clear",
      phrase: "CLEAR PROXY primary-openai",
    });
  });

  it("requires exact confirmation before persisting discovered models", () => {
    const state = createAccountModelDiscoveryConfirmation({
      id: "acct-1",
      name: "primary-openai",
    });

    expect(state).toMatchObject({
      accountId: "acct-1",
      accountName: "primary-openai",
      phrase: "PERSIST MODELS primary-openai",
    });
    expect(canConfirmAccountModelDiscovery({ ...state, confirmation: state.phrase })).toBe(true);
    expect(canConfirmAccountModelDiscovery({ ...state, confirmation: "persist models primary-openai" })).toBe(false);
  });

  it("parses account imports as object payloads with a non-empty account list", () => {
    expect(buildImportAccountsBody('{"accounts":[{"provider_id":"p1","name":"a1","runtime_class":"api_key","credential":{"api_key":"sk"}}]}')).toMatchObject({
      accounts: [
        {
          provider_id: "p1",
          name: "a1",
          runtime_class: "api_key",
          credential: { api_key: "sk" },
        },
      ],
    });

    expect(() => buildImportAccountsBody('{"accounts":[]}')).toThrow(
      "Import JSON must include a non-empty accounts array.",
    );
  });

  it("reports invalid account import JSON explicitly", () => {
    expect(() => buildImportAccountsBody("{not-json")).toThrow(
      "Import JSON must be valid JSON.",
    );
  });

  it("deduplicates batch status account ids and requires selection", () => {
    expect(buildBatchUpdateAccountsBody({
      accountIds: ["acct-1", "acct-1", "acct-2"],
      status: "disabled",
    })).toEqual({
      account_ids: ["acct-1", "acct-2"],
      status: "disabled",
    });

    expect(() => buildBatchUpdateAccountsBody({ accountIds: [], status: "disabled" })).toThrow(
      "Select at least one account.",
    );
  });

  it("requires exact confirmation before batch status changes", () => {
    const state = createAccountBatchStatusConfirmation({
      accountIds: ["acct-1", "acct-2"],
      status: "suspended",
    });

    expect(state.phrase).toBe("SET 2 ACCOUNTS suspended");
    expect(canConfirmAccountBatchStatus({ ...state, confirmation: state.phrase })).toBe(true);
    expect(canConfirmAccountBatchStatus({ ...state, confirmation: "SET 2 ACCOUNTS SUSPENDED" })).toBe(false);
  });
});
