import { describe, expect, it } from "vitest";
import {
  buildCreateProviderBody,
  buildUpdateProviderBody,
  emptyProviderForm,
  providerFormFromProvider,
  PROVIDER_ADAPTER_TYPES,
} from "@/lib/admin-provider-form";
import type { Provider } from "../../../../packages/sdk/typescript/src/types.gen";

const provider: Provider = {
  id: "provider-1",
  name: "openai_main",
  display_name: "OpenAI Main",
  adapter_type: "openai-compatible",
  protocol: "openai-compatible",
  status: "active",
  capabilities: { chat: true },
  config_schema: { required: ["api_key"] },
  created_at: "2026-05-24T00:00:00Z",
};

describe("admin-provider-form", () => {
  it("offers the generic reverse proxy adapter type", () => {
    expect(PROVIDER_ADAPTER_TYPES).toContain("generic-reverse-proxy");
  });

  it("builds create provider payloads from validated form state", () => {
    const form = {
      ...emptyProviderForm(),
      name: "openai_main",
      displayName: "OpenAI Main",
      status: "active" as const,
      capabilitiesJson: '{"chat":true,"images":false}',
      configSchemaJson: '{"required":["api_key"]}',
    };

    expect(buildCreateProviderBody(form)).toEqual({
      name: "openai_main",
      display_name: "OpenAI Main",
      adapter_type: "openai-compatible",
      protocol: "openai-compatible",
      status: "active",
      capabilities: { chat: true, images: false },
      config_schema: { required: ["api_key"] },
    });
  });

  it("builds update provider payloads without mutable provider name", () => {
    const form = providerFormFromProvider(provider);

    expect(buildUpdateProviderBody({ ...form, displayName: "OpenAI Prod" })).toMatchObject({
      display_name: "OpenAI Prod",
      adapter_type: "openai-compatible",
      protocol: "openai-compatible",
      status: "active",
    });
    expect(buildUpdateProviderBody(form)).not.toHaveProperty("name");
  });

  it("rejects invalid provider names", () => {
    const form = {
      ...emptyProviderForm(),
      name: "OpenAI",
      displayName: "OpenAI",
    };

    expect(() => buildCreateProviderBody(form)).toThrow(
      "Provider name must be 2-63 chars",
    );
  });

  it("rejects non-object JSON fields", () => {
    const form = {
      ...emptyProviderForm(),
      name: "openai_main",
      displayName: "OpenAI Main",
      capabilitiesJson: "[]",
    };

    expect(() => buildCreateProviderBody(form)).toThrow(
      "Capabilities must be a JSON object.",
    );
  });

  it("reports invalid capability JSON explicitly", () => {
    const form = {
      ...emptyProviderForm(),
      name: "openai_main",
      displayName: "OpenAI Main",
      capabilitiesJson: "{not-json",
    };

    expect(() => buildCreateProviderBody(form)).toThrow(
      "Capabilities must be valid JSON.",
    );
  });
});
