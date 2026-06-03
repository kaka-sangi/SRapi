import { describe, it, expect } from "vitest";
import { en } from "@/i18n/messages/en";
import { zh } from "@/i18n/messages/zh";

function flatKeys(obj: Record<string, unknown>, prefix = ""): string[] {
  return Object.entries(obj).flatMap(([key, value]) => {
    const full = prefix ? `${prefix}.${key}` : key;
    return value && typeof value === "object"
      ? flatKeys(value as Record<string, unknown>, full)
      : [full];
  });
}

describe("i18n message catalogs", () => {
  it("en and zh expose exactly the same keys", () => {
    const enKeys = flatKeys(en).sort();
    const zhKeys = flatKeys(zh).sort();
    expect(zhKeys).toEqual(enKeys);
  });

  it("no message value is empty", () => {
    for (const [k, v] of Object.entries({ ...flatten(en), ...flatten(zh) })) {
      expect(v, `empty value for ${k}`).not.toBe("");
    }
  });
});

function flatten(obj: Record<string, unknown>, prefix = "", out: Record<string, string> = {}) {
  for (const [key, value] of Object.entries(obj)) {
    const full = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === "object") flatten(value as Record<string, unknown>, full, out);
    else out[full] = String(value);
  }
  return out;
}
