import { describe, it, expect } from "vitest";
import { translate, applyVariables } from "@/i18n/messages";

describe("translate", () => {
  it("returns the localized string for a known key", () => {
    expect(translate("en", "login.signIn")).toBe("Sign in");
    expect(translate("zh", "login.signIn")).toBe("登录");
  });

  it("falls back to the key when missing", () => {
    expect(translate("en", "does.not.exist")).toBe("does.not.exist");
  });

  it("interpolates variables", () => {
    expect(applyVariables("{filtered} / {total}", { filtered: 3, total: 9 })).toBe("3 / 9");
  });
});
