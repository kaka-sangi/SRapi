import { describe, it, expect } from "vitest";
import { cn } from "@/lib/cn";

describe("cn", () => {
  it("joins truthy classes", () => {
    expect(cn("a", false, "b", null, undefined, "c")).toBe("a b c");
  });

  it("last-write-wins for conflicting tailwind utilities", () => {
    expect(cn("px-2", "px-4")).toBe("px-4");
  });
});
