import * as React from "react";
import { cn } from "@/lib/cn";

export interface KbdProps {
  children: React.ReactNode;
  className?: string;
}

/**
 * Single key cap — used to render `<kbd>` chips in command palettes, tooltip
 * footers, and inline shortcut hints.
 */
export function Kbd({ children, className }: KbdProps) {
  return (
    <kbd
      className={cn(
        "inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-md border border-srapi-border bg-srapi-card-muted px-1.5 font-mono text-[10px] font-semibold text-srapi-text-secondary shadow-[0_1px_0_var(--color-srapi-border-strong)]",
        className,
      )}
    >
      {children}
    </kbd>
  );
}
Kbd.displayName = "Kbd";

export interface KbdShortcutProps {
  keys: React.ReactNode[];
  separator?: React.ReactNode;
  className?: string;
}

/**
 * Composed cluster of `<Kbd>` chips joined by a separator (default " + ").
 */
export function KbdShortcut({ keys, separator = " + ", className }: KbdShortcutProps) {
  return (
    <span className={cn("inline-flex items-center gap-1 text-srapi-text-tertiary", className)}>
      {keys.map((key, index) => (
        <React.Fragment key={index}>
          {index > 0 ? (
            <span aria-hidden className="text-[10px] text-srapi-text-tertiary">
              {separator}
            </span>
          ) : null}
          <Kbd>{key}</Kbd>
        </React.Fragment>
      ))}
    </span>
  );
}
KbdShortcut.displayName = "KbdShortcut";
