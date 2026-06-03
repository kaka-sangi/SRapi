import * as React from "react";
import { cn } from "@/lib/cn";

export const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ className, ...props }, ref) => (
  <textarea
    ref={ref}
    className={cn(
      "min-h-20 w-full rounded-xl border border-srapi-border bg-srapi-card-muted px-3.5 py-2.5 text-sm text-srapi-text-primary transition-colors",
      "placeholder:text-srapi-text-secondary/60 hover:border-srapi-text-tertiary focus-visible:border-srapi-text-secondary",
      "disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:border-srapi-border",
      className,
    )}
    {...props}
  />
));
Textarea.displayName = "Textarea";
