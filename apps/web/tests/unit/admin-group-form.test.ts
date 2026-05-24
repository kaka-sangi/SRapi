import { describe, expect, it } from "vitest";
import {
  accountGroupFormFromGroup,
  applyModelScopeSelection,
  applyProviderScopeSelection,
  buildCreateAccountGroupBody,
  emptyAccountGroupForm,
  modelScopeLabel,
  providerScopeLabel,
} from "@/lib/admin-group-form";
import type { AccountGroup, Model, Provider } from "../../../../packages/sdk/typescript/src/types.gen";

const group: AccountGroup = {
  id: "group-1",
  name: "OpenAI Fast",
  description: "Fast OpenAI accounts",
  provider_scope: { provider_id: "provider-1" },
  model_scope: { canonical_name: "gpt-4o-mini" },
  strategy_hint: "latency_first",
  status: "active",
  created_at: "2026-05-24T00:00:00Z",
};

describe("admin-group-form", () => {
  it("round-trips account group JSON scopes", () => {
    const form = accountGroupFormFromGroup(group);
    const body = buildCreateAccountGroupBody(form);

    expect(body).toMatchObject({
      name: "OpenAI Fast",
      provider_scope: { provider_id: "provider-1" },
      model_scope: { canonical_name: "gpt-4o-mini" },
      strategy_hint: "latency_first",
      status: "active",
    });
  });

  it("builds scope JSON from picker selections", () => {
    const withProvider = applyProviderScopeSelection(
      { ...emptyAccountGroupForm(), name: "Scoped Group" },
      "provider-1",
    );
    const withModel = applyModelScopeSelection(withProvider, "gpt-4o-mini");

    expect(buildCreateAccountGroupBody(withModel)).toMatchObject({
      provider_scope: { provider_id: "provider-1" },
      model_scope: { canonical_name: "gpt-4o-mini" },
    });
  });

  it("rejects invalid scope JSON", () => {
    const form = {
      ...emptyAccountGroupForm(),
      name: "Bad Group",
      providerScopeJson: "[]",
    };

    expect(() => buildCreateAccountGroupBody(form)).toThrow(
      "Provider scope must be a JSON object.",
    );
  });

  it("reports invalid scope JSON explicitly", () => {
    const form = {
      ...emptyAccountGroupForm(),
      name: "Bad Group",
      modelScopeJson: "{not-json",
    };

    expect(() => buildCreateAccountGroupBody(form)).toThrow(
      "Model scope must be valid JSON.",
    );
  });

  it("labels scopes using catalog records when available", () => {
    const providers = [{ id: "provider-1", display_name: "OpenAI Compatible" }] as Provider[];
    const models = [{ canonical_name: "gpt-4o-mini", display_name: "GPT 4o Mini" }] as Model[];

    expect(providerScopeLabel({ provider_id: "provider-1" }, providers)).toBe("OpenAI Compatible");
    expect(modelScopeLabel({ canonical_name: "gpt-4o-mini" }, models)).toBe("GPT 4o Mini");
    expect(modelScopeLabel({ family: "claude" }, models)).toBe("family:claude");
  });
});
