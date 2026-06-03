import { cn } from "@/lib/cn";

/**
 * Restrained accent for a single emphasis word in a serif headline.
 * Per design system §1.1 this stays inside the warm family — a solid clay
 * accent, NOT a rainbow gradient. Kept as a named component so emphasis is
 * consistent and easy to retune in one place.
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
