import * as React from "react";
import { cn } from "@/lib/cn";

export interface ExpandableRowProps {
  expanded: boolean;
  children: React.ReactNode;
  className?: string;
}

/**
 * Inline row-expansion container. Animates grid-template-rows 0fr → 1fr via the
 * `.row-expand` utility class (see globals.css §7.9b).
 */
export function ExpandableRow({ expanded, children, className }: ExpandableRowProps) {
  return (
    <div
      className={cn("row-expand", className)}
      data-open={expanded ? "true" : "false"}
      aria-hidden={!expanded}
    >
      <div>{children}</div>
    </div>
  );
}
ExpandableRow.displayName = "ExpandableRow";
