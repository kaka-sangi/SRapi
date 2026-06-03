import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

// happy-dom shims used across tests
if (typeof window !== "undefined") {
  // matchMedia shim for next-themes
  window.matchMedia =
    window.matchMedia ||
    ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }));
}
