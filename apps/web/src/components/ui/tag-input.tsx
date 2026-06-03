"use client";

import { useState, type KeyboardEvent } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";

/**
 * Chip/tag input for a list of free-typed strings (e.g. allowed model names,
 * account-group ids). Replaces comma-separated text fields with visible,
 * individually-removable chips — Enter or comma commits, Backspace on an empty
 * input removes the last chip, blur commits a pending draft. Dedupes.
 */
export function TagInput({
  id,
  value,
  onChange,
  placeholder,
  disabled,
}: {
  id?: string;
  value: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
}) {
  const [draft, setDraft] = useState("");

  function add(raw: string) {
    const parts = raw
      .split(",")
      .map((p) => p.trim())
      .filter(Boolean);
    if (parts.length === 0) return;
    const next = [...value];
    for (const p of parts) if (!next.includes(p)) next.push(p);
    onChange(next);
    setDraft("");
  }

  function remove(tag: string) {
    onChange(value.filter((t) => t !== tag));
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter" || event.key === ",") {
      event.preventDefault();
      add(draft);
    } else if (event.key === "Backspace" && draft === "" && value.length > 0) {
      remove(value[value.length - 1]);
    }
  }

  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-1.5 rounded-xl border border-srapi-border bg-srapi-card-muted px-2 py-1.5 transition-colors focus-within:border-srapi-text-secondary",
        disabled && "opacity-50",
      )}
    >
      {value.map((tag) => (
        <span
          key={tag}
          className="inline-flex items-center gap-1 rounded-full border border-srapi-border bg-srapi-card px-2 py-0.5 font-mono text-2xs text-srapi-text-primary"
        >
          {tag}
          <button
            type="button"
            disabled={disabled}
            onClick={() => remove(tag)}
            aria-label={`remove ${tag}`}
            className="text-srapi-text-tertiary transition-colors hover:text-srapi-error"
          >
            <X className="size-3" />
          </button>
        </span>
      ))}
      <input
        id={id}
        type="text"
        value={draft}
        disabled={disabled}
        placeholder={value.length === 0 ? placeholder : undefined}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={onKeyDown}
        onBlur={() => {
          if (draft.trim()) add(draft);
        }}
        className="min-w-[8rem] flex-1 bg-transparent px-1 py-1 text-sm text-srapi-text-primary outline-none placeholder:text-srapi-text-tertiary"
      />
    </div>
  );
}
