import { describe, expect, it } from "vitest";
import { safeJson } from "@/lib/admin-format";

// safeJson is the formatter audit-log + payment-order-audit dialogs use to
// render before/after JSON payloads. It MUST not throw — the alternative
// would be a blank error-boundary card replacing the entire dialog.
// Specifically:
//
// - null / undefined return "{}" so the JsonBlock helper's "is empty"
//   check (iter 64) treats them as collapsible.
// - Circular refs (a real possibility on operator-supplied resource data)
//   degrade to the toString() representation rather than crashing.
describe("safeJson", () => {
  it("returns the {} placeholder for null and undefined", () => {
    expect(safeJson(null)).toBe("{}");
    expect(safeJson(undefined)).toBe("{}");
  });

  it("pretty-prints plain objects with 2-space indent", () => {
    expect(safeJson({ a: 1, b: "x" })).toBe('{\n  "a": 1,\n  "b": "x"\n}');
  });

  it("survives circular references by falling back to String(value)", () => {
    type Cycle = { name: string; self?: Cycle };
    const cyclic: Cycle = { name: "cycle" };
    cyclic.self = cyclic;
    // JSON.stringify on a cycle throws TypeError; safeJson must not.
    expect(() => safeJson(cyclic)).not.toThrow();
    // String(object) yields "[object Object]" — that's the graceful fallback.
    expect(safeJson(cyclic)).toBe("[object Object]");
  });

  it("handles primitive values correctly", () => {
    expect(safeJson(42)).toBe("42");
    expect(safeJson("hello")).toBe('"hello"');
    expect(safeJson(true)).toBe("true");
  });
});
