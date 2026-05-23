import { describe, it, expect } from "vitest";
import { cn } from "@/lib/cn";

describe("cn", () => {
  it("joins class names", () => {
    expect(cn("a", "b")).toBe("a b");
  });

  it("filters falsy values", () => {
    expect(cn("a", false && "b", null, undefined, "c")).toBe("a c");
  });

  it("merges conflicting tailwind utilities last-write-wins", () => {
    expect(cn("p-2", "p-4")).toBe("p-4");
    expect(cn("text-srapi-text-primary", "text-srapi-error")).toBe("text-srapi-error");
  });

  it("preserves orthogonal utilities", () => {
    expect(cn("flex items-center", "gap-2")).toBe("flex items-center gap-2");
  });
});
