"use client";

import { useState } from "react";
import { Brain, ChevronDown } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";

/** Collapsible chain-of-thought block (default open). Shared. */
export function ReasoningBlock({ text }: { text: string }) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(true);
  return (
    <div className="rounded-xl border border-srapi-border bg-srapi-card-muted/30 text-xs">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
      >
        <Brain className="size-3.5 shrink-0" />
        <span className="flex-1">{t("chat.reasoning")}</span>
        <ChevronDown className={`size-3.5 shrink-0 transition-transform ${open ? "rotate-180" : ""}`} />
      </button>
      {open ? (
        <div className="border-t border-srapi-border px-3 py-2">
          <div className="whitespace-pre-wrap break-words text-xs leading-relaxed text-srapi-text-tertiary">
            {text}
          </div>
        </div>
      ) : null}
    </div>
  );
}
