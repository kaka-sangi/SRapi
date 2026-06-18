// <input type="datetime-local"> values are local wall-clock strings without a
// timezone. Parsing through Date preserves that local meaning before serializing
// the absolute instant for the API.
export function localDateTimeInputToIso(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const date = new Date(trimmed);
  if (Number.isNaN(date.getTime())) return null;
  return date.toISOString();
}
