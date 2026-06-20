import { describe, expect, it } from "vitest";
import {
  buildCreateProviderBody,
  buildUpdateProviderBody,
  providerFormFromProvider,
} from "@/lib/admin-provider-form";
import type { Provider } from "@/lib/sdk-types";

const baseProvider: Provider = {
  id: "42",
  name: "openai",
  display_name: "OpenAI",
  adapter_type: "openai-compatible",
  protocol: "openai-compatible",
  status: "active",
  capabilities: {},
  config_schema: {},
  created_at: "2026-06-21T00:00:00Z",
};

describe("admin provider form endpoint capability switches", () => {
  it("loads protocol endpoint capabilities into explicit tri-state controls", () => {
    const form = providerFormFromProvider({
      ...baseProvider,
      capabilities: {
        chat_completions: true,
        responses: false,
        messages: true,
        custom_flag: "kept",
      },
    });

    expect(form.chatCompletionsCapability).toBe("on");
    expect(form.responsesCapability).toBe("off");
    expect(form.responsesCompactCapability).toBe("auto");
    expect(form.responsesInputItemsCapability).toBe("auto");
    expect(form.messagesCapability).toBe("on");
    expect(form.capabilities.custom_flag).toBe("kept");
  });

  it("writes endpoint switches into provider capabilities without losing custom flags", () => {
    const form = providerFormFromProvider({
      ...baseProvider,
      capabilities: {
        chat_completions: true,
        responses: true,
        responses_compact: false,
        messages: true,
        custom_flag: "kept",
      },
    });

    const body = buildUpdateProviderBody({
      ...form,
      responsesCapability: "off",
      responsesCompactCapability: "auto",
      responsesInputItemsCapability: "on",
    });

    expect(body.capabilities).toEqual({
      chat_completions: true,
      responses: false,
      responses_input_items: true,
      messages: true,
      custom_flag: "kept",
    });
  });

  it("omits endpoint capability keys left on auto when creating providers", () => {
    const body = buildCreateProviderBody({
      name: "custom-openai",
      displayName: "Custom OpenAI",
      adapterType: "openai-compatible",
      protocol: "openai-compatible",
      status: "active",
      chatCompletionsCapability: "auto",
      responsesCapability: "auto",
      responsesCompactCapability: "auto",
      responsesInputItemsCapability: "auto",
      messagesCapability: "off",
      capabilities: {
        responses: true,
        custom_flag: true,
      },
      configSchema: {},
    });

    expect(body.capabilities).toEqual({
      messages: false,
      custom_flag: true,
    });
  });
});
