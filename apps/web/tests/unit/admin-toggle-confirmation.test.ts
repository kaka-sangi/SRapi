import { describe, expect, it } from "vitest";
import {
  accountToggleIdentifier,
  canConfirmToggle,
  toggleActionFromStatus,
  toggleActionLabel,
  userToggleIdentifier,
} from "@/lib/admin-toggle-confirmation";

describe("admin-toggle-confirmation", () => {
  it("enables disabled resources and disables every other status", () => {
    expect(toggleActionFromStatus("disabled")).toBe("enable");
    expect(toggleActionFromStatus("active")).toBe("disable");
    expect(toggleActionFromStatus("pending")).toBe("disable");
  });

  it("uses stable human identifiers for confirmation", () => {
    expect(userToggleIdentifier({ email: "admin@srapi.local", name: "Admin" })).toBe("admin@srapi.local");
    expect(accountToggleIdentifier({ id: "acct-1", name: "openai-primary" })).toBe("openai-primary");
  });

  it("requires exact confirmation after trimming outer whitespace", () => {
    expect(canConfirmToggle("admin@srapi.local", " admin@srapi.local ")).toBe(true);
    expect(canConfirmToggle("admin@srapi.local", "Admin@srapi.local")).toBe(false);
    expect(canConfirmToggle("", "")).toBe(false);
  });

  it("builds explicit action labels", () => {
    expect(toggleActionLabel("disable", "User")).toBe("Disable User");
    expect(toggleActionLabel("enable", "Account")).toBe("Enable Account");
  });
});
