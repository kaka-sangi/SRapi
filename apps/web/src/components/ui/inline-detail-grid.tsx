import * as React from "react";
import { cn } from "@/lib/cn";

export type InlineDetailTone = "default" | "success" | "warning" | "error" | "muted";

export interface InlineDetailRow {
  label: string;
  value: React.ReactNode;
  mono?: boolean;
  tone?: InlineDetailTone;
}

export interface InlineDetailSection {
  title: string;
  rows: InlineDetailRow[];
}

export interface InlineDetailGridProps {
  sections: InlineDetailSection[];
  actions?: React.ReactNode;
  className?: string;
}

const toneClasses: Record<InlineDetailTone, string> = {
  default: "text-srapi-text-primary",
  success: "text-srapi-success",
  warning: "text-srapi-warning",
  error: "text-srapi-error",
  muted: "text-srapi-text-tertiary",
};

function toneClass(tone?: InlineDetailTone): string {
  return toneClasses[tone ?? "default"];
}

/**
 * Hover-expand row content scaffold. Designed to live inside `ExpandableRow`,
 * laying out 1–3 columns of label/value pairs grouped into titled sections,
 * with an optional right-aligned action row.
 */
export function InlineDetailGrid({ sections, actions, className }: InlineDetailGridProps) {
  return (
    <div
      className={cn(
        "border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4",
        className,
      )}
    >
      <div className="grid gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
        {sections.map((section) => (
          <div key={section.title}>
            <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {section.title}
            </div>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
              {section.rows.map((row, index) => (
                <React.Fragment key={`${section.title}-${index}`}>
                  <dt className="text-[11px] text-srapi-text-tertiary">{row.label}</dt>
                  <dd
                    className={cn(
                      "text-right text-[12px] font-medium tabular",
                      row.mono && "font-mono break-all",
                      toneClass(row.tone),
                    )}
                  >
                    {row.value}
                  </dd>
                </React.Fragment>
              ))}
            </dl>
          </div>
        ))}
      </div>
      {actions ? <div className="mt-4 flex justify-end gap-2">{actions}</div> : null}
    </div>
  );
}
InlineDetailGrid.displayName = "InlineDetailGrid";
