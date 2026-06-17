import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { writeClipboard } from "@/components/ui/copy-button";

// The iter-107 export. The whole point of this helper is the execCommand
// fallback for insecure contexts where navigator.clipboard.writeText
// silently rejects — once-only secrets (api key plaintext, redeem codes,
// backup snapshots) rely on it. These tests pin both paths down.
//
// JSDom/happy-dom doesn't ship a real Clipboard API and execCommand is
// deprecated, so each test stubs the minimum it needs.

describe("writeClipboard", () => {
  beforeEach(() => {
    // happy-dom doesn't ship document.execCommand at all — define a no-op
    // baseline so vi.spyOn can replace it per-test. The real browser API
    // returns boolean; we mirror the signature.
    if (typeof document.execCommand !== "function") {
      Object.defineProperty(document, "execCommand", {
        configurable: true,
        writable: true,
        value: () => false,
      });
    }
  });
  afterEach(() => {
    vi.restoreAllMocks();
    // happy-dom carries clipboard state across tests if not reset.
    if ("clipboard" in navigator) {
      // @ts-expect-error — delete the test stub between cases.
      delete (navigator as { clipboard?: unknown }).clipboard;
    }
  });

  it("returns true when the Async Clipboard API succeeds", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });

    const ok = await writeClipboard("hello");
    expect(ok).toBe(true);
    expect(writeText).toHaveBeenCalledWith("hello");
  });

  it("falls through to execCommand when the Async API rejects (insecure context)", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("not allowed"));
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const execSpy = vi.spyOn(document, "execCommand").mockReturnValue(true);

    const ok = await writeClipboard("hello");
    expect(ok).toBe(true);
    expect(execSpy).toHaveBeenCalledWith("copy");
  });

  it("uses execCommand when navigator.clipboard lacks writeText (old browser)", async () => {
    // Install a clipboard without the writeText method — simulates a browser
    // / locked-down WebView where the property exists but the secure-context
    // API is gated off. The implementation's optional-chain falls through.
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {},
    });
    const execSpy = vi.spyOn(document, "execCommand").mockReturnValue(true);

    const ok = await writeClipboard("hello");
    expect(ok).toBe(true);
    expect(execSpy).toHaveBeenCalledWith("copy");
  });

  it("returns false when both paths fail (so callers don't flash false success)", async () => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error("nope")) },
    });
    vi.spyOn(document, "execCommand").mockReturnValue(false);

    const ok = await writeClipboard("hello");
    expect(ok).toBe(false);
  });
});
