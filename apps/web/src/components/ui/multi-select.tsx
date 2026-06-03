"use client";

import { useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Check, ChevronDown, Plus, Search, X } from "lucide-react";
import { Popover, PopoverAnchor, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/cn";

export interface MultiSelectOption {
  value: string;
  label: string;
}

/**
 * Searchable multi-select: a tag-input-style box of removable chips that opens a
 * filterable checkbox list. Replaces newline/JSON list textareas for picking
 * from a known set (countries, models, channels). With `allowCustom` the admin
 * can also commit a typed value that isn't in the option list (e.g. a 2-letter
 * country code or an arbitrary channel), so nothing forces raw structured input.
 */
export function MultiSelect({
  id,
  value,
  onChange,
  options,
  placeholder,
  searchPlaceholder,
  emptyText,
  addCustomLabel,
  allowCustom = false,
  disabled,
}: {
  id?: string;
  value: string[];
  onChange: (next: string[]) => void;
  options: MultiSelectOption[];
  placeholder?: string;
  searchPlaceholder?: string;
  emptyText?: string;
  /** Label for the "add free value" row, given the typed query. */
  addCustomLabel?: (query: string) => string;
  allowCustom?: boolean;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [boxWidth, setBoxWidth] = useState<number>();
  const boxRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const listId = useId();

  const labelByValue = useMemo(() => {
    const map = new Map<string, string>();
    for (const opt of options) map.set(opt.value, opt.label);
    return map;
  }, [options]);

  const normalizedQuery = query.trim().toLowerCase();
  const filtered = useMemo(() => {
    if (!normalizedQuery) return options;
    return options.filter(
      (opt) =>
        opt.label.toLowerCase().includes(normalizedQuery) ||
        opt.value.toLowerCase().includes(normalizedQuery),
    );
  }, [options, normalizedQuery]);

  const trimmed = query.trim();
  const canAddCustom =
    allowCustom &&
    trimmed.length > 0 &&
    !value.includes(trimmed) &&
    !options.some((opt) => opt.value.toLowerCase() === trimmed.toLowerCase());

  function toggle(optionValue: string) {
    onChange(
      value.includes(optionValue)
        ? value.filter((v) => v !== optionValue)
        : [...value, optionValue],
    );
  }

  function remove(optionValue: string) {
    onChange(value.filter((v) => v !== optionValue));
  }

  function addCustom() {
    if (!canAddCustom) return;
    onChange([...value, trimmed]);
    setQuery("");
  }

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      if (filtered.length > 0) toggle(filtered[0].value);
      else if (canAddCustom) addCustom();
    } else if (event.key === "Backspace" && query === "" && value.length > 0) {
      remove(value[value.length - 1]);
    }
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (next && boxRef.current) setBoxWidth(boxRef.current.offsetWidth);
        setOpen(next);
        if (!next) setQuery("");
      }}
    >
      <PopoverAnchor asChild>
        <div
          ref={boxRef}
          className={cn(
            "flex min-h-10 flex-wrap items-center gap-1.5 rounded-xl border border-srapi-border bg-srapi-card-muted px-2 py-1.5 transition-colors",
            open ? "border-srapi-text-secondary" : "hover:border-srapi-text-tertiary",
            disabled && "pointer-events-none opacity-50",
          )}
        >
          {value.map((v) => (
            <span
              key={v}
              className="inline-flex items-center gap-1 rounded-full border border-srapi-border bg-srapi-card px-2 py-0.5 font-mono text-2xs text-srapi-text-primary"
            >
              {labelByValue.get(v) ?? v}
              <button
                type="button"
                disabled={disabled}
                onClick={() => remove(v)}
                aria-label={`remove ${labelByValue.get(v) ?? v}`}
                className="text-srapi-text-tertiary transition-colors hover:text-srapi-error"
              >
                <X className="size-3" />
              </button>
            </span>
          ))}
          <PopoverTrigger asChild>
            <button
              type="button"
              id={id}
              disabled={disabled}
              role="combobox"
              aria-expanded={open}
              aria-controls={listId}
              className="flex min-w-[5rem] flex-1 items-center justify-between gap-2 bg-transparent px-1 py-1 text-left text-sm text-srapi-text-primary outline-none"
            >
              {value.length === 0 ? (
                <span className="text-srapi-text-tertiary">{placeholder}</span>
              ) : (
                <span aria-hidden className="sr-only">
                  {value.length}
                </span>
              )}
              <ChevronDown className="size-4 shrink-0 text-srapi-text-tertiary opacity-70" />
            </button>
          </PopoverTrigger>
        </div>
      </PopoverAnchor>

      <PopoverContent
        id={listId}
        className="p-0"
        style={boxWidth ? { width: boxWidth } : undefined}
        onOpenAutoFocus={(e) => {
          // Focus our search input rather than the first list row.
          e.preventDefault();
          searchRef.current?.focus();
        }}
      >
        <div className="flex items-center gap-2 border-b border-srapi-border px-3 py-2">
          <Search className="size-3.5 shrink-0 text-srapi-text-tertiary" />
          <input
            ref={searchRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onSearchKeyDown}
            placeholder={searchPlaceholder}
            className="w-full bg-transparent text-sm text-srapi-text-primary outline-none placeholder:text-srapi-text-tertiary"
          />
        </div>
        <div className="max-h-60 overflow-y-auto p-1">
          {filtered.map((opt) => {
            const on = value.includes(opt.value);
            return (
              <button
                key={opt.value}
                type="button"
                onClick={() => toggle(opt.value)}
                className={cn(
                  "flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors",
                  "hover:bg-srapi-card-muted",
                  on ? "text-srapi-text-primary" : "text-srapi-text-secondary",
                )}
              >
                <span
                  className={cn(
                    "flex size-4 shrink-0 items-center justify-center rounded border transition-colors",
                    on
                      ? "border-srapi-invert bg-srapi-invert text-srapi-invert-fg"
                      : "border-srapi-border",
                  )}
                >
                  {on ? <Check className="size-3" /> : null}
                </span>
                <span className="truncate">{opt.label}</span>
              </button>
            );
          })}
          {canAddCustom ? (
            <button
              type="button"
              onClick={addCustom}
              className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted"
            >
              <Plus className="size-3.5 shrink-0" />
              <span className="truncate">
                {addCustomLabel ? addCustomLabel(trimmed) : trimmed}
              </span>
            </button>
          ) : null}
          {filtered.length === 0 && !canAddCustom ? (
            <p className="px-2.5 py-3 text-center text-2xs text-srapi-text-tertiary">{emptyText}</p>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  );
}
