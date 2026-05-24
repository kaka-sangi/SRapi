import { describe, expect, it } from "vitest";
import {
  buildCreateProxyBody,
  buildUpdateProxyBody,
  emptyProxyForm,
  proxyFormFromProxy,
} from "@/lib/admin-proxy-form";
import type { ProxyDefinition } from "../../../../packages/sdk/typescript/src/types.gen";

const proxy: ProxyDefinition = {
  id: "proxy-1",
  name: "egress_tokyo",
  type: "socks5",
  status: "active",
  url_configured: true,
  metadata: { region: "ap-northeast-1" },
  created_at: "2026-05-24T00:00:00Z",
  updated_at: "2026-05-24T00:00:00Z",
};

describe("admin-proxy-form", () => {
  it("builds create proxy payloads with write-only url and metadata", () => {
    const form = {
      ...emptyProxyForm(),
      name: "egress_tokyo",
      type: "socks5" as const,
      url: "socks5://user:pass@proxy.example:1080",
      metadata: '{"region":"ap-northeast-1"}',
    };

    expect(buildCreateProxyBody(form)).toEqual({
      name: "egress_tokyo",
      type: "socks5",
      url: "socks5://user:pass@proxy.example:1080",
      status: "active",
      metadata: { region: "ap-northeast-1" },
    });
  });

  it("builds update proxy payloads without returning url when left blank", () => {
    const form = proxyFormFromProxy(proxy);

    expect(form.url).toBe("");
    expect(buildUpdateProxyBody(form)).toEqual({
      name: "egress_tokyo",
      type: "socks5",
      status: "active",
      metadata: { region: "ap-northeast-1" },
    });
  });

  it("requires url for new proxies", () => {
    const form = {
      ...emptyProxyForm(),
      name: "egress_tokyo",
      url: "",
    };

    expect(() => buildCreateProxyBody(form)).toThrow("Proxy URL is required.");
  });

  it("reports invalid metadata JSON explicitly", () => {
    const form = {
      ...emptyProxyForm(),
      name: "egress_tokyo",
      url: "https://proxy.example",
      metadata: "{not-json",
    };

    expect(() => buildCreateProxyBody(form)).toThrow("Metadata must be valid JSON.");
  });
});
