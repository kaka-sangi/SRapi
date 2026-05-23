import { describe, it, expect } from "vitest";
import { applyVariables, flatLookup, messages } from "@/i18n/messages";

describe("i18n messages", () => {
  it("exposes the same keys for en and zh", () => {
    const enKeys = Object.values(messages.en).flatMap((ns) => Object.keys(ns));
    const zhKeys = Object.values(messages.zh).flatMap((ns) => Object.keys(ns));
    expect(zhKeys.sort()).toEqual(enKeys.sort());
  });

  it("flatLookup returns plain string values for known keys", () => {
    const en = flatLookup("en");
    expect(en.verifyOperator).toBe("Sign in to SRapi");
    expect(en.smokeDrawerTitle).toBe("v0.1.0 self-check");
  });

  it("applyVariables substitutes placeholders", () => {
    expect(applyVariables("Hello, {name}!", { name: "world" })).toBe("Hello, world!");
    expect(applyVariables("{a} of {b}", { a: 1, b: 2 })).toBe("1 of 2");
  });

  it("applyVariables is a no-op when no variables given", () => {
    expect(applyVariables("plain")).toBe("plain");
  });

  it("uses calm v0.1.0 wording — never the deprecated phrasing", () => {
    const en = flatLookup("en");
    const banned = [
      "Operator Console",
      "Cryptographic Credentials Vault",
      "Adaptive dispatch",
      "Verify Operator Credentials",
    ];
    for (const phrase of banned) {
      for (const value of Object.values(en)) {
        expect(value).not.toContain(phrase);
      }
    }
  });
});
