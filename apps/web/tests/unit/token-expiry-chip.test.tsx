import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LanguageProvider } from "@/context/LanguageContext";
import { TooltipProvider } from "@/components/ui/tooltip";
import { TokenExpiryChip } from "@/app/admin/accounts/token-expiry-chip";
import type { ProviderAccount } from "@/lib/sdk-types";

// LanguageProvider reads persisted locale from localStorage; happy-dom in this
// setup doesn't provide it, so shim a minimal in-memory store (mirrors
// resource-form-validation.test.tsx).
const storage = new Map<string, string>();
Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() {
      return storage.size;
    },
  },
});

// Minimal stub builder — every property the chip touches plus the required
// shape the SDK type expects. The chip itself only consults runtime_class,
// token_expires_at and needs_reauth_at; the rest is filler so TypeScript
// accepts the value.
function makeAccount(overrides: Partial<ProviderAccount> = {}): ProviderAccount {
  return {
    id: "1",
    provider_id: "1",
    name: "oauth-account",
    runtime_class: "oauth_refresh",
    status: "active",
    priority: 0,
    weight: 1,
    group_ids: [],
    created_at: "2026-06-16T00:00:00Z",
    ...overrides,
  } as ProviderAccount;
}

function renderChip(account: ProviderAccount, now?: Date) {
  // Production tree wraps everything in TooltipProvider at the root
  // (apps/web/src/providers/index.tsx); mirror that here so the chip can
  // render its Radix Tooltip without throwing.
  return render(
    <LanguageProvider>
      <TooltipProvider>
        <TokenExpiryChip account={account} now={now} />
      </TooltipProvider>
    </LanguageProvider>,
  );
}

describe("TokenExpiryChip", () => {
  const now = new Date("2026-06-16T12:00:00Z");

  it("renders nothing for non-OAuth accounts", () => {
    const { container } = renderChip(
      makeAccount({ runtime_class: "api_key", token_expires_at: "2026-06-16T13:00:00Z" }),
      now,
    );
    expect(container.textContent ?? "").toBe("");
  });

  it("renders nothing when no token_expires_at and no needs_reauth_at", () => {
    const { container } = renderChip(makeAccount({}), now);
    expect(container.textContent ?? "").toBe("");
  });

  it("renders 'Refreshes in <duration>' when expiry is in the future", () => {
    const expiry = new Date(now.getTime() + 45 * 60 * 1000).toISOString(); // +45m
    const { container } = renderChip(makeAccount({ token_expires_at: expiry }), now);
    expect(container.textContent).toContain("45m");
    expect(container.textContent).toMatch(/Refreshes in|后刷新/);
  });

  it("renders 'Expired <duration> ago' when expiry is in the past and not flagged needs_reauth", () => {
    const expiry = new Date(now.getTime() - 2 * 60 * 60 * 1000).toISOString(); // -2h
    const { container } = renderChip(makeAccount({ token_expires_at: expiry }), now);
    expect(container.textContent).toContain("2h");
    expect(container.textContent).toMatch(/Expired|已过期/);
  });

  it("renders 'Needs reauth' (red tone) when needs_reauth_at is set, regardless of expiry", () => {
    const expiry = new Date(now.getTime() + 60 * 60 * 1000).toISOString(); // +1h
    const flagged = new Date(now.getTime() - 5 * 60 * 1000).toISOString();
    const { container } = renderChip(
      makeAccount({ token_expires_at: expiry, needs_reauth_at: flagged }),
      now,
    );
    expect(container.textContent).toMatch(/Needs reauth|需要重新授权/);
    // needs_reauth uses the error tone (.text-srapi-error on the glyph span).
    expect(container.querySelector(".text-srapi-error")).not.toBeNull();
    // It must NOT also render the "Refreshes in" copy.
    expect(container.textContent).not.toMatch(/Refreshes in|后刷新/);
  });

  it("wraps the chip in a Tooltip trigger when refresh_last_error is present", () => {
    // When operators see "Needs reauth" they need to know WHY — invalid_grant
    // (token revoked) vs network 5xx (transient) vs client-misconfig.
    // refresh_last_error is the single most useful signal; pin that it shows
    // up in the tooltip trigger DOM tree even when content is portalled.
    const flagged = new Date(now.getTime() - 5 * 60 * 1000).toISOString();
    const { container } = renderChip(
      makeAccount({
        needs_reauth_at: flagged,
        refresh_last_error: "invalid_grant: refresh token revoked by upstream",
        refresh_attempts: 5,
      }),
      now,
    );
    // The chip itself still renders the label (visible).
    expect(container.textContent).toMatch(/Needs reauth|需要重新授权/);
    // Radix wraps the trigger in a button element with data-state.
    const trigger = container.querySelector("[data-state]");
    expect(trigger).not.toBeNull();
  });

  it("does NOT wrap when needs_reauth_at fires with no error and no attempts (cleaner cold-start)", () => {
    // If the account just got manually disabled or migrated, refresh_attempts
    // can be 0 and refresh_last_error empty. The tooltip would be empty —
    // skip the wrapper so the chip renders identically to the legacy shape.
    const flagged = new Date(now.getTime() - 5 * 60 * 1000).toISOString();
    const { container } = renderChip(
      makeAccount({ needs_reauth_at: flagged }),
      now,
    );
    expect(container.textContent).toMatch(/Needs reauth|需要重新授权/);
    // No tooltip trigger should be present.
    expect(container.querySelector("[data-state]")).toBeNull();
  });
});
