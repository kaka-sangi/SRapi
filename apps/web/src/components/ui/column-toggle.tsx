"use client";

import { Columns3, RotateCcw } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import type { ColumnVisibility } from "@/hooks/use-column-visibility";

export interface ColumnDef {
  key: string;
  label?: string;
  header?: string;
}

export function ColumnToggle({
  columns,
  visibility,
}: {
  columns: ColumnDef[];
  visibility: ColumnVisibility;
}) {
  const { t } = useLanguage();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <Columns3 className="size-3.5" />
          <span className="hidden sm:inline">{t("adminCommon.columns")}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-52">
        <DropdownMenuLabel className="flex items-center justify-between">
          <span>{t("adminCommon.columns")}</span>
          <button
            type="button"
            onClick={visibility.reset}
            className="text-2xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-primary"
            title={t("common.reset")}
          >
            <RotateCcw className="size-3" />
          </button>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <div className="max-h-64 space-y-0.5 overflow-y-auto px-1 py-1">
          {columns.map((col) => (
            <label
              key={col.key}
              className="flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-srapi-card-muted"
            >
              <Checkbox
                checked={visibility.isVisible(col.key)}
                onChange={() => visibility.toggle(col.key)}
              />
              <span className="text-srapi-text-secondary">{col.label ?? col.header}</span>
            </label>
          ))}
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
