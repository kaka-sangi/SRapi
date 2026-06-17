// CSV cell escaping for the admin + user usage exports. Shared so both
// hooks stay in sync — they had two copies that drifted is an obvious
// trap. The two behaviours that matter and that the tests pin down:
//
// 1. Spreadsheet formula injection guard: cells that start with =, +, -,
//    @, \t or \r are prefixed with a single quote so Excel / Sheets won't
//    interpret them as a formula when an operator opens the CSV. CWE-1236.
//
// 2. RFC 4180 quoting: cells containing the comma, newline, CR or a
//    literal quote are wrapped in quotes with any internal quotes doubled.

export function escapeCsv(value: string | number): string {
  const str = String(value);
  const needsFormulaGuard = /^[=+\-@\t\r]/.test(str);
  const guarded = needsFormulaGuard ? `'${str}` : str;
  if (/[",\n\r]/.test(guarded)) {
    return `"${guarded.replace(/"/g, '""')}"`;
  }
  return guarded;
}

export interface CsvColumn<T> {
  header: string;
  value: (row: T) => string | number;
}

// Build a CSV blob string: header row + body rows joined by CRLF, with a
// leading UTF-8 BOM so Excel opens the file without mangling non-ASCII.
export function rowsToCsv<T>(rows: readonly T[], columns: readonly CsvColumn<T>[]): string {
  const header = columns.map((c) => escapeCsv(c.header)).join(",");
  const body = rows.map((row) => columns.map((c) => escapeCsv(c.value(row))).join(","));
  return `﻿${[header, ...body].join("\r\n")}\r\n`;
}

// Trigger a browser download for a CSV payload. Same machinery the
// admin + per-user export hooks used inline.
export function triggerCsvDownload(csv: string, filename: string): void {
  const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
