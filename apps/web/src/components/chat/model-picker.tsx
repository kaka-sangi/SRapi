"use client";

import { useState } from "react";
import { Sparkles, ChevronDown, Plus, Check } from "lucide-react";
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover";
import { useLanguage } from "@/context/LanguageContext";

/** Model selector with search + free-text custom entry. Shared by the admin
 * copilot and the user playground. */
export function ModelPicker({
  value,
  models,
  onChange,
}: {
  value: string;
  models: string[];
  onChange: (v: string) => void;
}) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const filtered = draft.trim() ? models.filter((m) => m.toLowerCase().includes(draft.trim().toLowerCase())) : models;

  function choose(v: string) {
    const next = v.trim();
    if (!next) return;
    onChange(next);
    setOpen(false);
    setDraft("");
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="inline-flex max-w-[12rem] items-center gap-1 rounded-full border border-srapi-border bg-srapi-card-muted/60 px-2.5 py-1 text-xs text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted"
        >
          <Sparkles className="size-3.5 shrink-0 text-srapi-text-tertiary" />
          <span className="truncate font-mono">{value || t("chat.selectModel")}</span>
          <ChevronDown className="size-3 shrink-0 text-srapi-text-tertiary" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 p-2">
        <input
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              choose(draft || value);
            }
          }}
          placeholder={t("chat.customModel")}
          className="mb-1 w-full rounded-md border border-srapi-border bg-srapi-bg px-2 py-1.5 font-mono text-xs text-srapi-text-primary outline-none focus:border-srapi-text-tertiary"
        />
        <div className="max-h-52 overflow-y-auto">
          {filtered.length === 0 && draft.trim() ? (
            <button
              type="button"
              onClick={() => choose(draft)}
              className="flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-xs text-srapi-text-secondary hover:bg-srapi-card-muted"
            >
              <Plus className="size-3.5" />
              <span className="truncate font-mono">{draft.trim()}</span>
            </button>
          ) : null}
          {filtered.map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => choose(m)}
              className={`flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-srapi-card-muted ${
                m === value ? "text-srapi-text-primary" : "text-srapi-text-secondary"
              }`}
            >
              <span className="truncate font-mono">{m}</span>
              {m === value ? <Check className="size-3.5 shrink-0 text-srapi-primary" /> : null}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  );
}
