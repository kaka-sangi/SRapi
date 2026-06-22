"use client";

import { useLanguage } from "@/context/LanguageContext";
import { Card } from "@/components/ui/card";
import { DataPill } from "@/components/ui/data-pill";
import { cn } from "@/lib/cn";
import {
  PLATFORM_ICON_COLORS,
  PLATFORM_ICONS,
  type PlatformPreset,
} from "./presets";

// ---------------------------------------------------------------------------
// Step 1: Platform grid — DiscoveryCard-style tiles with icon bubble, title,
// description, and a footer row of auth-type/model-count pills.
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
          className="block h-full text-left focus:outline-none"
        >
          <Card
            className={cn(
              "group h-full",
              p.custom && "border-dashed border-srapi-primary/30 bg-srapi-accent-soft/20",
            )}
          >
            <div className="flex h-full flex-col gap-4 p-5">
              <div className="flex items-start gap-3">
                <div
                  className={cn(
                    "grid size-11 shrink-0 place-items-center rounded-xl font-mono text-xs font-bold tracking-tight transition-transform duration-200 group-hover:scale-105",
                    p.custom
                      ? "bg-srapi-card-muted text-srapi-text-secondary"
                      : PLATFORM_ICON_COLORS[p.key] ?? "bg-srapi-card-muted text-srapi-text-secondary",
                  )}
                >
                  {PLATFORM_ICONS[p.key] ?? p.key.slice(0, 2).toUpperCase()}
                </div>
                <div className="min-w-0 flex-1">
                  <h3 className="text-base font-semibold tracking-tight text-srapi-text-primary">
                    {p.name}
                  </h3>
                  <p className="mt-1 text-sm leading-relaxed text-srapi-text-secondary line-clamp-2">
                    {p.description}
                  </p>
                </div>
              </div>
              <div className="mt-auto flex flex-wrap gap-1.5 border-t border-srapi-border/70 pt-3">
                {p.authTypes.map((a) => (
                  <DataPill key={a} tone="neutral" size="sm">
                    {a}
                  </DataPill>
                ))}
                {p.defaultModels.length > 0 && (
                  <DataPill tone="neutral" size="sm">
                    {p.defaultModels.length} {t("adminAccounts.models")}
                  </DataPill>
                )}
              </div>
            </div>
          </Card>
        </button>
      ))}
    </div>
  );
}
