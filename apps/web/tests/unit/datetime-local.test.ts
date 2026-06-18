import { describe, expect, it } from "vitest";
import { localDateTimeInputToIso } from "@/lib/datetime-local";

function isoToLocalInput(iso: string): string {
  const date = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours(),
  )}:${pad(date.getMinutes())}`;
}

describe("localDateTimeInputToIso", () => {
  it("preserves datetime-local wall-clock components", () => {
    const input = "2026-09-30T14:45";
    const iso = localDateTimeInputToIso(input);

    expect(iso).toBeTruthy();
    expect(isoToLocalInput(iso ?? "")).toBe(input);
  });

  it("returns null for empty or invalid input", () => {
    expect(localDateTimeInputToIso("")).toBeNull();
    expect(localDateTimeInputToIso("   ")).toBeNull();
    expect(localDateTimeInputToIso("not-a-date")).toBeNull();
  });
});
