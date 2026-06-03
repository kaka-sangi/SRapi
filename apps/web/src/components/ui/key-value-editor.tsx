"use client";

import { useState } from "react";
import { Plus, X } from "lucide-react";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";

type Row = { id: number; key: string; value: string };

/** Render a stored value into its edit-cell text: strings as-is, everything
 * else (numbers, booleans, nested objects/arrays) as compact JSON. */
function valueToText(value: unknown): string {
  return typeof value === "string" ? value : JSON.stringify(value);
}

/** Interpret a cell back into a value: valid JSON (number/bool/object/array)
 * is parsed; anything else is kept as a plain string. So `https://x` → string,
 * `42` → number, `true` → boolean, `{"a":1}` → object — no JSON typing needed
 * for the common scalar case, but nested values are still expressible. */
function textToValue(text: string): unknown {
  const trimmed = text.trim();
  if (trimmed === "") return "";
  try {
    return JSON.parse(trimmed);
  } catch {
    return text;
  }
}

function objectToRows(object: Record<string, unknown>): Row[] {
  return Object.entries(object).map(([key, value], index) => ({
    id: index,
    key,
    value: valueToText(value),
  }));
}

function rowsToObject(rows: Row[]): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (key) out[key] = textToValue(row.value);
  }
  return out;
}

/**
 * Graphical editor for a flat-ish key→value map (account metadata, plan
 * entitlements, provider config, …) — rows of key + value instead of a raw JSON
 * textarea. Holds its own row buffer (initialised from `value` once), emitting
 * the rebuilt object on every edit.
 */
export function KeyValueEditor({
  value,
  onChange,
  disabled,
  addLabel,
  keyPlaceholder,
  valuePlaceholder,
}: {
  value: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
  disabled?: boolean;
  addLabel: string;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
}) {
  const initialRows = objectToRows(value);
  const [rows, setRows] = useState<Row[]>(initialRows);
  const [nextId, setNextId] = useState(initialRows.length);

  function commit(next: Row[]) {
    setRows(next);
    onChange(rowsToObject(next));
  }

  return (
    <div className="space-y-2">
      {rows.length > 0 ? (
        <div className="space-y-1.5">
          {rows.map((row) => (
            <div key={row.id} className="flex items-center gap-1.5">
              <Input
                className="h-9 flex-1 font-mono text-xs"
                placeholder={keyPlaceholder ?? "key"}
                value={row.key}
                disabled={disabled}
                onChange={(e) =>
                  commit(rows.map((r) => (r.id === row.id ? { ...r, key: e.target.value } : r)))
                }
              />
              <Input
                className="h-9 flex-1 font-mono text-xs"
                placeholder={valuePlaceholder ?? "value"}
                value={row.value}
                disabled={disabled}
                onChange={(e) =>
                  commit(rows.map((r) => (r.id === row.id ? { ...r, value: e.target.value } : r)))
                }
              />
              <button
                type="button"
                disabled={disabled}
                onClick={() => commit(rows.filter((r) => r.id !== row.id))}
                aria-label={`remove ${row.key || "field"}`}
                className="shrink-0 text-srapi-text-tertiary transition-colors hover:text-srapi-error"
              >
                <X className="size-4" />
              </button>
            </div>
          ))}
        </div>
      ) : null}
      <button
        type="button"
        disabled={disabled}
        onClick={() => {
          setRows((prev) => [...prev, { id: nextId, key: "", value: "" }]);
          setNextId((n) => n + 1);
        }}
        className={cn(
          "inline-flex items-center gap-1 rounded-lg border border-dashed border-srapi-border px-2.5 py-1 text-2xs text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:text-srapi-text-primary disabled:opacity-50",
        )}
      >
        <Plus className="size-3.5" /> {addLabel}
      </button>
    </div>
  );
}
