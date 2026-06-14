import { useState } from "react";
import { LayoutGrid, List, RefreshCw, Timer } from "lucide-react";
import { useAutoRefresh } from "@/hooks/use-auto-refresh";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import type { AccountListMode } from "./account-types";

export function ViewModeToggle({
  mode,
  onChange,
}: {
  mode: AccountListMode;
  onChange: (mode: AccountListMode) => void;
}) {
  const { t } = useLanguage();
  return (
    <div className="inline-flex h-9 rounded-lg border border-srapi-border-strong bg-srapi-card p-0.5">
      <Button
        type="button"
        variant={mode === "cards" ? "outline" : "ghost"}
        size="sm"
        className={cn(
          "h-7 rounded-md border-0 px-2.5 shadow-none",
          mode !== "cards" && "text-srapi-text-secondary",
        )}
        aria-pressed={mode === "cards"}
        onClick={() => onChange("cards")}
      >
        <LayoutGrid className="size-3.5" />
        <span className="hidden sm:inline">{t("adminAccounts.viewCards")}</span>
      </Button>
      <Button
        type="button"
        variant={mode === "table" ? "outline" : "ghost"}
        size="sm"
        className={cn(
          "h-7 rounded-md border-0 px-2.5 shadow-none",
          mode !== "table" && "text-srapi-text-secondary",
        )}
        aria-pressed={mode === "table"}
        onClick={() => onChange("table")}
      >
        <List className="size-3.5" />
        <span className="hidden sm:inline">{t("adminAccounts.viewTable")}</span>
      </Button>
    </div>
  );
}

export function AutoRefreshButton({
  autoRefresh,
}: {
  autoRefresh: ReturnType<typeof useAutoRefresh>;
}) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  return (
    <div className="relative">
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen((v) => !v)}
        className={cn(autoRefresh.enabled && "border-srapi-success/40")}
      >
        {autoRefresh.enabled ? (
          <RefreshCw className="size-3.5 animate-spin text-srapi-success" style={{ animationDuration: `${autoRefresh.interval}s` }} />
        ) : (
          <Timer className="size-3.5" />
        )}
        <span className="hidden sm:inline">
          {autoRefresh.enabled
            ? `${autoRefresh.timeUntilRefresh}s`
            : t("common.autoRefresh")}
        </span>
      </Button>
      {open ? (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute right-0 z-50 mt-1.5 w-44 rounded-lg border border-srapi-border bg-srapi-card p-1.5 shadow-lg">
            <button
              type="button"
              onClick={() => { autoRefresh.toggle(); setOpen(false); }}
              className="flex w-full items-center justify-between rounded-md px-3 py-2 text-xs text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted"
            >
              <span>{autoRefresh.enabled ? t("common.off") : t("common.autoRefresh")}</span>
              {autoRefresh.enabled ? (
                <span className="size-1.5 rounded-full bg-srapi-success" />
              ) : null}
            </button>
            <div className="my-1 border-t border-srapi-border" />
            {autoRefresh.intervalOptions.map((sec) => (
              <button
                key={sec}
                type="button"
                onClick={() => { autoRefresh.setInterval(sec); if (!autoRefresh.enabled) autoRefresh.toggle(); setOpen(false); }}
                className={cn(
                  "flex w-full items-center justify-between rounded-md px-3 py-1.5 text-xs transition-colors hover:bg-srapi-card-muted",
                  autoRefresh.interval === sec && autoRefresh.enabled
                    ? "font-medium text-srapi-text-primary"
                    : "text-srapi-text-tertiary",
                )}
              >
                <span>{sec}s</span>
                {autoRefresh.interval === sec && autoRefresh.enabled ? (
                  <span className="text-2xs text-srapi-success">&#10003;</span>
                ) : null}
              </button>
            ))}
          </div>
        </>
      ) : null}
    </div>
  );
}
