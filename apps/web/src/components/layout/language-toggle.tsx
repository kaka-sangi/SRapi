"use client";

import * as React from "react";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";

export function LanguageToggle({ className }: { className?: string }) {
  const { language, toggleLanguage } = useLanguage();
  return (
    <button
      type="button"
      onClick={toggleLanguage}
      title="Switch language"
      aria-label="Switch language"
      className={cn(
        "rounded-xl border border-srapi-border px-2.5 py-1.5 font-mono text-[11px] font-bold uppercase tracking-wider text-srapi-text-secondary",
        "transition-all hover:bg-srapi-card-muted hover:text-srapi-text-primary",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary focus-visible:ring-offset-2 focus-visible:ring-offset-srapi-bg",
        className,
      )}
    >
      {language === "en" ? "中文" : "EN"}
    </button>
  );
}
