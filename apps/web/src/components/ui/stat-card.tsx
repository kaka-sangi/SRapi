import { cn } from "@/lib/cn";
import { Card } from "./card";
import { Sparkline } from "@/components/charts/sparkline";

export function StatCard({
  label,
  value,
  unit,
  hint,
  trend,
  spark,
  className,
}: {
  label: string;
  value: string;
  unit?: string;
  hint?: React.ReactNode;
  trend?: { dir: "up" | "down"; text: string };
  spark?: number[];
  className?: string;
}) {
  return (
    <Card className={cn("flex flex-col p-5", className)}>
      <div className="flex items-center justify-between">
        <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">{label}</span>
        {trend && (
          <span
            className={cn(
              "font-mono text-2xs tabular",
              trend.dir === "up" ? "text-srapi-success" : "text-srapi-error",
            )}
          >
            {trend.dir === "up" ? "↑" : "↓"} {trend.text}
          </span>
        )}
      </div>
      <div className="mt-3 font-serif text-3xl leading-none text-srapi-text-primary tabular">
        {value}
        {unit && <span className="ml-1.5 text-sm font-sans font-normal text-srapi-text-tertiary">{unit}</span>}
      </div>
      {spark && spark.length >= 2 && (
        <div className="mt-3.5">
          <Sparkline values={spark} ariaLabel={label} className="h-8" />
        </div>
      )}
      {hint && (
        <div className="mt-2.5 font-mono text-2xs text-srapi-text-tertiary">{hint}</div>
      )}
    </Card>
  );
}
