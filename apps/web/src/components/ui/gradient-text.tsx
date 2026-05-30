"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface GradientTextProps extends React.HTMLAttributes<HTMLSpanElement> {
  as?: "span" | "strong" | "em";
}

/**
 * Renders inline text with the restrained clay→indigo→violet spectrum. Reserve
 * for one or two accent words in a headline — not whole paragraphs.
 */
export function GradientText({ as = "span", className, ...props }: GradientTextProps) {
  const Comp = as;
  return <Comp className={cn("gradient-text not-italic", className)} {...props} />;
}
