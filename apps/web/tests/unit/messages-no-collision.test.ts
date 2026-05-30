import { describe, it, expect } from "vitest";
import { messages } from "@/i18n/messages";

/**
 * The runtime `t(key)` API flattens every namespace into one lookup
 * (`buildFlatLookup`, last-write-wins). A key defined in two namespaces would
 * silently shadow the other. This guard fails fast on any such collision so
 * new copy (e.g. the `marketing` namespace) can't quietly clobber existing keys.
 */
describe("i18n message namespaces", () => {
  it("has no flat-key collision across namespaces", () => {
    const owner = new Map<string, string>();
    const collisions: string[] = [];
    for (const [namespace, bucket] of Object.entries(messages.en)) {
      for (const key of Object.keys(bucket)) {
        const previous = owner.get(key);
        if (previous) {
          collisions.push(`"${key}" defined in both "${previous}" and "${namespace}"`);
        } else {
          owner.set(key, namespace);
        }
      }
    }
    expect(collisions).toEqual([]);
  });
});
