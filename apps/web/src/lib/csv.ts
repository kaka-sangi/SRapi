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
