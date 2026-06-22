"use client";

import { Search } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { cn } from "@/lib/cn";

/** Search box for an admin list toolbar. Controlled by `useAdminList`. */
export function SearchInput({
  value,
  onChange,
  placeholder,
  className,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}) {
  return (
    <div className={cn("relative w-full sm:max-w-xs", className)}>
      <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-srapi-text-tertiary" />
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-10 rounded-xl pl-9"
        type="search"
      />
    </div>
  );
}

const ALL = "__all__";

export interface FilterOption {
  value: string;
  label: string;
}

/**
 * Dropdown filter for an admin list. `value === undefined` means "all"; picking
 * the all-row clears the filter. Map the chosen value into the page's SDK query.
 */
export function FilterSelect({
  value,
  onChange,
  options,
  allLabel,
  placeholder,
  className,
}: {
  value: string | undefined;
  onChange: (value: string | undefined) => void;
  options: FilterOption[];
  allLabel: string;
  placeholder?: string;
  className?: string;
}) {
  return (
    <Select
      value={value ?? ALL}
      onValueChange={(next) => onChange(next === ALL ? undefined : next)}
    >
      <SelectTrigger className={cn("h-10 w-auto min-w-[8.5rem] gap-2 rounded-xl", className)}>
        <SelectValue placeholder={placeholder ?? allLabel} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>{allLabel}</SelectItem>
        {options.map((opt) => (
          <SelectItem key={opt.value} value={opt.value}>
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/**
 * Horizontal toolbar wrapper: search on the left, filters trailing.
 *
 * When `sticky` (default), the toolbar pins to the top of its parent Card
 * on scroll with a translucent backdrop-blur surface so filters/search stay
 * reachable while skimming long lists.
 */
export function ListToolbar({
  children,
  sticky = true,
}: {
  children: React.ReactNode;
  sticky?: boolean;
}) {
  return (
    <div
      className={cn(
        "flex flex-col gap-2 border-b border-srapi-border/70 bg-srapi-card px-4 py-3 sm:flex-row sm:flex-wrap sm:items-center sm:gap-2",
        sticky &&
          "sticky top-0 z-10 bg-srapi-card/95 backdrop-blur-sm border-b border-srapi-border",
      )}
    >
      {children}
    </div>
  );
}

const SEVERITY_ALL_VALUE = "all";

const DEFAULT_SEVERITY_OPTIONS: Array<{ value: string; label: string }> = [
  { value: "all", label: "All" },
  { value: "critical", label: "Critical" },
  { value: "error", label: "Error" },
  { value: "warning", label: "Warning" },
];

/**
 * Severity quick-filter for list toolbars. Wraps SegmentedControl with a
 * default {All / Critical / Error / Warning} option set. Selecting "All"
 * surfaces `undefined` to the consumer so a single filter param drives the
 * query state.
 */
function ToolbarSeverityFilter({
  value,
  onChange,
  options = DEFAULT_SEVERITY_OPTIONS,
}: {
  value: string | undefined;
  onChange: (v: string | undefined) => void;
  options?: Array<{ value: string; label: string }>;
}) {
  const current = value ?? SEVERITY_ALL_VALUE;
  return (
    <SegmentedControl
      ariaLabel="severity filter"
      size="sm"
      value={current}
      onChange={(next) => onChange(next === SEVERITY_ALL_VALUE ? undefined : next)}
      options={options.map((o) => ({ value: o.value, label: o.label }))}
    />
  );
}
