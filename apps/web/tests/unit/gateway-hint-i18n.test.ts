import { describe, expect, it } from "vitest";
import { GATEWAY_HINT_KEYS } from "@/lib/gateway-error-hint";
import { en } from "@/i18n/messages/en";
import { zh } from "@/i18n/messages/zh";

// Guards that the diagnostic helper and the translation catalogs never drift:
// every reject-reason hint the gateway-error-hint helper can emit must resolve
// to real en + zh copy, otherwise a failed request would show a blank hint.
describe("gateway hint i18n coverage", () => {
  it("emits at least one hint key", () => {
    expect(GATEWAY_HINT_KEYS.length).toBeGreaterThan(0);
  });

  it("every emittable hint key has English text", () => {
    const hints = en.gatewayHints as Record<string, string>;
    for (const key of GATEWAY_HINT_KEYS) {
      expect(hints[key], `missing en.gatewayHints.${key}`).toBeTruthy();
    }
  });

  it("every emittable hint key has Chinese text", () => {
    const hints = zh.gatewayHints as Record<string, string>;
    for (const key of GATEWAY_HINT_KEYS) {
      expect(hints[key], `missing zh.gatewayHints.${key}`).toBeTruthy();
    }
  });
});
