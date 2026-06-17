import { describe, expect, it } from "vitest";
import {
  accountToggleIdentifier,
  canConfirmToggle,
  toggleActionFromStatus,
  toggleActionLabel,
  userToggleIdentifier,
} from "@/lib/admin-toggle-confirmation";

// The "type-the-name-to-confirm" disable/enable dialog is the last line of
// defense before an operator yanks an account or a user. canConfirmToggle
// is what the confirm button binds to — a regression where empty input
// passes (e.g. someone "tidies up" by replacing identifier.length > 0
// with !!identifier and the `confirmation.trim() === ""` case slips
// through against an empty identifier) would silently turn the safety
// dialog into a single-click destruct button. Pin the contract.

describe("toggleActionFromStatus", () => {
  it("offers to 'enable' a disabled subject", () => {
    expect(toggleActionFromStatus("disabled")).toBe("enable");
  });
  it("offers to 'disable' anything else (active, pending, …)", () => {
    expect(toggleActionFromStatus("active")).toBe("disable");
    expect(toggleActionFromStatus("pending")).toBe("disable");
    // Even unknown values default to "disable" — the asymmetry is on
    // purpose: only the literal "disabled" string flips to enable.
    expect(toggleActionFromStatus("totally-unknown")).toBe("disable");
  });
});

describe("userToggleIdentifier", () => {
  it("prefers email when present (the canonical user identifier)", () => {
    expect(userToggleIdentifier({ email: "alice@example.com", name: "Alice" })).toBe(
      "alice@example.com",
    );
  });
  it("falls back to name when email is empty", () => {
    expect(userToggleIdentifier({ email: "", name: "Alice" })).toBe("Alice");
  });
});

describe("accountToggleIdentifier", () => {
  it("prefers the account name (the canonical account identifier)", () => {
    expect(accountToggleIdentifier({ name: "primary-pool-a", id: "acct_1" })).toBe(
      "primary-pool-a",
    );
  });
  it("falls back to id when name is empty", () => {
    expect(accountToggleIdentifier({ name: "", id: "acct_1" })).toBe("acct_1");
  });
});

describe("canConfirmToggle", () => {
  it("returns true ONLY when confirmation matches identifier exactly", () => {
    expect(canConfirmToggle("primary-pool-a", "primary-pool-a")).toBe(true);
  });

  it("tolerates leading / trailing whitespace on the user typed input", () => {
    // Real users paste with surrounding whitespace — trim it. Just not
    // case differences, not internal spaces. Pin the asymmetry.
    expect(canConfirmToggle("primary-pool-a", "  primary-pool-a  ")).toBe(true);
    expect(canConfirmToggle("primary-pool-a", "\tprimary-pool-a\n")).toBe(true);
  });

  it("is case-sensitive (a destructive action shouldn't be confusable)", () => {
    expect(canConfirmToggle("primary-pool-a", "Primary-Pool-A")).toBe(false);
  });

  it("rejects mismatched input even after trim", () => {
    expect(canConfirmToggle("primary-pool-a", "wrong-name")).toBe(false);
    expect(canConfirmToggle("primary-pool-a", "primary-pool")).toBe(false);
    expect(canConfirmToggle("primary-pool-a", "primary-pool-a-extra")).toBe(false);
  });

  it("rejects empty confirmation — the load-bearing safety check", () => {
    // If this ever returns true, an operator can clobber an account by
    // clicking confirm on a blank dialog. Pin EVERY way an empty
    // confirmation could sneak through.
    expect(canConfirmToggle("primary-pool-a", "")).toBe(false);
    expect(canConfirmToggle("primary-pool-a", "   ")).toBe(false);
    expect(canConfirmToggle("primary-pool-a", "\t\n")).toBe(false);
  });

  it("rejects everything when identifier is empty (defends against a fallback that returned '')", () => {
    // userToggleIdentifier / accountToggleIdentifier may return "" if
    // BOTH preferred and fallback fields are blank. In that pathological
    // case, the confirm button must stay disabled even if the user
    // types "" — otherwise the toggle fires on a no-op identifier.
    expect(canConfirmToggle("", "")).toBe(false);
    expect(canConfirmToggle("", "anything")).toBe(false);
  });
});

describe("toggleActionLabel", () => {
  it("title-cases the verb and prepends it to the subject", () => {
    expect(toggleActionLabel("enable", "alice@example.com")).toBe(
      "Enable alice@example.com",
    );
    expect(toggleActionLabel("disable", "primary-pool-a")).toBe(
      "Disable primary-pool-a",
    );
  });
});
