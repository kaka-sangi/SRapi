"use client";

import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import {
  PLATFORM_ICON_COLORS,
  PLATFORM_ICONS,
  type PlatformPreset,
} from "./presets";

// ---------------------------------------------------------------------------
// Step 1: Platform grid
// ---------------------------------------------------------------------------

export function PlatformGrid({
  platforms,
  onSelect,
}: {
  platforms: PlatformPreset[];
  onSelect: (p: PlatformPreset) => void;
}) {
  const { t } = useLanguage();

  if (platforms.length === 0) {
    return (
      <p className="mt-6 text-sm text-srapi-text-tertiary">
        {t("adminQuickSetup.noPresets")}
      </p>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {platforms.map((p) => (
        <button
          key={p.key}
          type="button"
          onClick={() => onSelect(p)}
          className={cn(
            "group relative flex items-start gap-4 rounded-xl border bg-srapi-card p-5 text-left transition-all",
            p.custom
              ? "border-dashed border-srapi-border hover:border-srapi-primary/50 hover:bg-srapi-primary/5"
              : "border-srapi-border hover:border-srapi-text-tertiary hover:shadow-sm",
            "active:scale-[0.985]",
          )}
        >
          <div
            className={cn(
              "flex size-11 shrink-0 items-center justify-center rounded-lg font-mono text-xs font-bold tracking-tight transition-colors",
              p.custom
                ? "bg-srapi-primary/10 text-srapi-primary group-hover:bg-srapi-primary/20"
                : PLATFORM_ICON_COLORS[p.key] ?? "bg-srapi-card-muted text-srapi-text-secondary",
            )}
          >
            {PLATFORM_ICONS[p.key] ?? p.key.slice(0, 2).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-srapi-text-primary">
              {p.name}
            </div>
            <div className="mt-0.5 text-xs leading-relaxed text-srapi-text-tertiary">
              {p.description}
            </div>
            <div className="mt-2.5 flex flex-wrap gap-1.5">
              {p.authTypes.map((a) => (
                <span
                  key={a}
                  className="rounded-md bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary"
                >
                  {a}
                </span>
              ))}
              {p.defaultModels.length > 0 && (
                <span className="rounded-md bg-srapi-card-muted px-1.5 py-0.5 text-2xs text-srapi-text-tertiary">
                  {p.defaultModels.length} {t("adminAccounts.models")}
                </span>
              )}
            </div>
          </div>
        </button>
      ))}
    </div>
  );
}
