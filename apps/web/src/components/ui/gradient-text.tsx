import { cn } from "@/lib/cn";

/**
 * Restrained accent for a single emphasis word inside a headline. Stays inside
 * the warm family — a solid terracotta accent, NOT a rainbow gradient. Kept as
 * a named component so emphasis remains consistent and easy to retune in one
 * place.
 */
export function GradientText({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <span className={cn("text-srapi-primary", className)}>{children}</span>;
}
