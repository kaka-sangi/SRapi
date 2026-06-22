import * as React from "react";
import { cn } from "@/lib/cn";

/**
 * Table primitives. Wrap in <TableScroll> for the §7.2 mobile "侧滑防护罩":
 * horizontal scroll guard with a min-width so dense tables never blow out the
 * viewport on narrow screens.
 */
export function TableScroll({
  minWidth = 560,
  className,
  children,
}: {
  minWidth?: number;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("w-full overflow-x-auto", className)}>
      <div style={{ minWidth }}>{children}</div>
    </div>
  );
}

export const Table = React.forwardRef<HTMLTableElement, React.HTMLAttributes<HTMLTableElement>>(
  ({ className, ...props }, ref) => (
    <table ref={ref} className={cn("w-full text-sm", className)} {...props} />
  ),
);
Table.displayName = "Table";

export const TableHeader = React.forwardRef<
  HTMLTableSectionElement,
  React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
  <thead
    ref={ref}
    className={cn(
      // Modern table header: small caps eyebrow row sitting directly on the card
      // surface, separated from rows by a single hairline rule.
      "text-left text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary [&_th]:border-b [&_th]:border-srapi-border",
      className,
    )}
    {...props}
  />
));
TableHeader.displayName = "TableHeader";

export const TableBody = React.forwardRef<
  HTMLTableSectionElement,
  React.HTMLAttributes<HTMLTableSectionElement>
>(({ className, ...props }, ref) => (
  <tbody ref={ref} className={cn(className)} {...props} />
));
TableBody.displayName = "TableBody";

export const TableRow = React.forwardRef<
  HTMLTableRowElement,
  React.HTMLAttributes<HTMLTableRowElement>
>(({ className, ...props }, ref) => (
  <tr
    ref={ref}
    className={cn(
      "border-b border-srapi-border/60 transition-colors last:border-0 hover:bg-srapi-card-muted/50",
      className,
    )}
    {...props}
  />
));
TableRow.displayName = "TableRow";

/** numeric: 等宽 + tabular-nums + 右对齐。给数字列防止位数跳动 / 视觉抖。 */
type NumericProp = { numeric?: boolean };

export const TableHead = React.forwardRef<
  HTMLTableCellElement,
  React.ThHTMLAttributes<HTMLTableCellElement> & NumericProp
>(({ className, numeric, ...props }, ref) => (
  <th
    ref={ref}
    className={cn(
      "whitespace-nowrap px-4 py-3 text-left align-middle",
      numeric && "text-right tabular",
      className,
    )}
    {...props}
  />
));
TableHead.displayName = "TableHead";

export const TableCell = React.forwardRef<
  HTMLTableCellElement,
  React.TdHTMLAttributes<HTMLTableCellElement> & NumericProp
>(({ className, numeric, ...props }, ref) => (
  <td
    ref={ref}
    className={cn(
      "px-4 py-3 text-sm align-middle",
      numeric && "text-right tabular",
      className,
    )}
    {...props}
  />
));
TableCell.displayName = "TableCell";
