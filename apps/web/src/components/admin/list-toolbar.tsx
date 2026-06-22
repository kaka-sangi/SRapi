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

/** Horizontal toolbar wrapper: search on the left, filters trailing. */
export function ListToolbar({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-2 border-b border-srapi-border/70 bg-srapi-card px-4 py-3 sm:flex-row sm:flex-wrap sm:items-center sm:gap-2">
      {children}
    </div>
  );
}
