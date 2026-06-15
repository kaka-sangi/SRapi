import { describe, expect, it } from "vitest";
import { createApiKeySchema, updateApiKeySchema } from "@/lib/schemas/api-key";

// Minimal valid payload: the four list fields are required arrays (empty is OK);
// the numeric/string limits are optional.
const base = {
  name: "My Key",
  allowedModels: [] as string[],
  groupIds: [] as string[],
  allowedIps: [] as string[],
  deniedIps: [] as string[],
};

describe("createApiKeySchema", () => {
  it("accepts an empty allowedModels list (= all models, matches the OpenAPI default [])", () => {
    // Regression guard: create used to force >=1 model, which blocked the common
    // "one key that works for everything" case. Empty must stay valid.
    expect(createApiKeySchema.safeParse(base).success).toBe(true);
  });

  it("accepts a bounded list of specific models", () => {
    expect(
      createApiKeySchema.safeParse({ ...base, allowedModels: ["gpt-4o", "claude-sonnet-4-6"] })
        .success,
    ).toBe(true);
  });

  it("rejects more than 16 models", () => {
    const tooMany = Array.from({ length: 17 }, (_, i) => `m${i}`);
    expect(createApiKeySchema.safeParse({ ...base, allowedModels: tooMany }).success).toBe(false);
  });

  it("rejects a name shorter than 2 characters", () => {
    expect(createApiKeySchema.safeParse({ ...base, name: "a" }).success).toBe(false);
  });

  it("rejects a name with disallowed characters", () => {
    expect(createApiKeySchema.safeParse({ ...base, name: "bad!name" }).success).toBe(false);
  });

  it("accepts empty group bindings (= no group restriction, all accounts)", () => {
    expect(createApiKeySchema.safeParse({ ...base, groupIds: [] }).success).toBe(true);
  });
});

describe("updateApiKeySchema", () => {
  it("also accepts an empty allowedModels list", () => {
    expect(updateApiKeySchema.safeParse(base).success).toBe(true);
  });
});
