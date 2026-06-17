import { describe, expect, it } from "vitest";
import { escapeCsv } from "@/lib/csv";

// escapeCsv is the shared helper underneath both the admin and per-user
// usage CSV exports. The two behaviours we lock down here are both
// security/correctness-critical, not stylistic:
//
// 1. Spreadsheet formula injection: a malicious user can name an API key
//    `=2+2` and the next operator opening an export in Excel runs the
//    formula. The guard is a leading single quote.
//
// 2. RFC 4180 quoting around delimiters, line breaks, and embedded quotes
//    — so a payload that legitimately contains a comma doesn't break the
//    column count of the resulting file.
describe("escapeCsv", () => {
  it("returns plain values unchanged when no escape is needed", () => {
    expect(escapeCsv("hello")).toBe("hello");
    expect(escapeCsv("user@example")).toBe("user@example"); // @ in middle is fine
    expect(escapeCsv(42)).toBe("42");
    expect(escapeCsv("")).toBe("");
  });

  it("guards against spreadsheet formula injection", () => {
    // Leading =, +, -, @, \t, \r all run as formulas in Excel without the
    // single-quote prefix.
    expect(escapeCsv("=2+2")).toBe("'=2+2");
    expect(escapeCsv("+phish")).toBe("'+phish");
    expect(escapeCsv("-evil")).toBe("'-evil");
    expect(escapeCsv("@SUM(A1:A99)")).toBe("'@SUM(A1:A99)");
    expect(escapeCsv("\tindented")).toBe("'\tindented");
    // \r triggers BOTH guards: the formula-prefix AND the RFC-4180 quote
    // wrap (because \r is in the [",\n\r] line-break class). Real-world:
    // CSV needs the quoting anyway since a bare \r would split the row.
    expect(escapeCsv("\rreturn")).toBe('"\'\rreturn"');
  });

  it("wraps + escapes when the cell contains commas / newlines / quotes", () => {
    expect(escapeCsv("a,b")).toBe('"a,b"');
    expect(escapeCsv("line\nbreak")).toBe('"line\nbreak"');
    expect(escapeCsv('he said "hi"')).toBe('"he said ""hi"""');
  });

  it("composes both guards — formula-prefix then quote-wrap", () => {
    // A name like `=cmd|"calc"!A1` gets the formula-guard prefix AND the
    // outer quote-wrap because it also contains a literal quote and a comma
    // would otherwise split it.
    expect(escapeCsv('=cmd|"calc"!A1')).toBe('"\'=cmd|""calc""!A1"');
    expect(escapeCsv("=a,b")).toBe('"\'=a,b"');
  });
});
