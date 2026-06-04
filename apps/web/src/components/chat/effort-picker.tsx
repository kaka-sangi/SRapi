"use client";

import { Brain, ChevronDown, Check } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { useLanguage } from "@/context/LanguageContext";
import { EFFORTS, type ReasoningEffort } from "./types";

/** Thinking-effort selector (off/low/medium/high). Shared. */
export function EffortPicker({ value, onChange }: { value: ReasoningEffort; onChange: (v: ReasoningEffort) => void }) {
  const { t } = useLanguage();
  const label: Record<ReasoningEffort, string> = {
    off: t("chat.effortOff"),
    low: t("chat.effortLow"),
    medium: t("chat.effortMedium"),
    high: t("chat.effortHigh"),
  };
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs transition-colors ${
            value === "off"
              ? "border-srapi-border bg-srapi-card-muted/60 text-srapi-text-secondary hover:bg-srapi-card-muted"
              : "border-srapi-primary/40 bg-srapi-primary/10 text-srapi-primary"
          }`}
        >
          <Brain className="size-3.5 shrink-0" />
          <span className="truncate">{label[value]}</span>
          <ChevronDown className="size-3 shrink-0 opacity-70" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {EFFORTS.map((e) => (
          <DropdownMenuItem key={e} onClick={() => onChange(e)}>
            <Check className={`size-3.5 ${e === value ? "opacity-100 text-srapi-primary" : "opacity-0"}`} />
            {label[e]}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
