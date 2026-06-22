import * as React from "react";
import { Rows2, Rows3 } from "lucide-react";
import { SegmentedControl } from "./segmented-control";

export type DensityValue = "compact" | "regular";

export interface DensityToggleProps {
  value: DensityValue;
  onChange: (v: DensityValue) => void;
  className?: string;
}

/**
 * Two-state list density toggle. Persistence (e.g. localStorage) is the
 * caller's responsibility.
 */
export function DensityToggle({ value, onChange, className }: DensityToggleProps) {
  return (
    <SegmentedControl<DensityValue>
      value={value}
      onChange={onChange}
      ariaLabel="list density"
      size="sm"
      className={className}
      options={[
        {
          value: "compact",
          label: <span className="sr-only">Compact</span>,
          icon: <Rows3 aria-hidden />,
        },
        {
          value: "regular",
          label: <span className="sr-only">Regular</span>,
          icon: <Rows2 aria-hidden />,
        },
      ]}
    />
  );
}
DensityToggle.displayName = "DensityToggle";
