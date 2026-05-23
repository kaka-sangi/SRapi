import { describe, it, expect } from "vitest";
import { createApiKeySchema, parseGroupIdsCsv } from "@/lib/schemas/api-key";

describe("createApiKeySchema", () => {
  const validValues = {
    name: "production-web",
    allowedModels: ["gpt-4o-mini"],
    groupIds: ["group-01"],
  };

  it("accepts a clean payload", () => {
    expect(createApiKeySchema.safeParse(validValues).success).toBe(true);
  });

  it("rejects a name shorter than 2 chars", () => {
    const result = createApiKeySchema.safeParse({ ...validValues, name: "x" });
    expect(result.success).toBe(false);
  });

  it("rejects a name with disallowed characters", () => {
    const result = createApiKeySchema.safeParse({ ...validValues, name: "foo!bar" });
    expect(result.success).toBe(false);
  });

  it("rejects an empty allowedModels array", () => {
    const result = createApiKeySchema.safeParse({ ...validValues, allowedModels: [] });
    expect(result.success).toBe(false);
  });

  it("rejects > 16 allowedModels", () => {
    const tooMany = Array.from({ length: 17 }, (_, i) => `m-${i}`);
    const result = createApiKeySchema.safeParse({ ...validValues, allowedModels: tooMany });
    expect(result.success).toBe(false);
  });

  it("trims whitespace on the name", () => {
    const result = createApiKeySchema.safeParse({ ...validValues, name: "  production-web  " });
    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.name).toBe("production-web");
    }
  });
});

describe("parseGroupIdsCsv", () => {
  it("splits and trims", () => {
    expect(parseGroupIdsCsv("a, b ,c")).toEqual(["a", "b", "c"]);
  });

  it("filters empty entries", () => {
    expect(parseGroupIdsCsv(",a,, b ,")).toEqual(["a", "b"]);
  });

  it("returns an empty array for empty input", () => {
    expect(parseGroupIdsCsv("")).toEqual([]);
    expect(parseGroupIdsCsv("   ")).toEqual([]);
  });
});
