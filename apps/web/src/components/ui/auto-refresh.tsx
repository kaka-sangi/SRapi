"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { RefreshCw, ChevronDown, Check } from "lucide-react";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";

// Interval choices in seconds; 0 = off. Kept short so live dashboards stay fresh
// without hammering the API.
const OPTIONS = [0, 15, 30, 60] as const;

function readStored(storageKey: string | undefined, fallback: number): number {
  if (!storageKey || typeof window === "undefined") return fallback;
  const raw = window.localStorage.getItem(storageKey);
  if (raw == null) return fallback;
  const n = Number(raw);
  return (OPTIONS as readonly number[]).includes(n) ? n : fallback;
}

/**
 * AutoRefreshControl — a manual refresh button plus an interval picker that
 * re-runs `onRefresh` on a visible countdown. It owns the timer (so the
 * countdown is accurate) and pauses while the tab is hidden. Pages that adopt
 * it should drop any hardcoded react-query `refetchInterval` so polling has a
 * single source of truth.
 */
export function AutoRefreshControl({
  onRefresh,
  isRefreshing,
  storageKey,
  defaultSec = 0,
  className,
}: {
  onRefresh: () => void | Promise<void>;
  isRefreshing?: boolean;
  storageKey?: string;
  defaultSec?: number;
  className?: string;
}) {
  const { t } = useLanguage();
  const [intervalSec, setIntervalSec] = useState(() => readStored(storageKey, defaultSec));
  const [secondsLeft, setSecondsLeft] = useState(intervalSec);
  // Keep the latest onRefresh in a ref so the ticking interval doesn't reset on
  // every parent render (pages pass an inline arrow). Updated via effect, never
  // mutated during render.
  const onRefreshRef = useRef(onRefresh);
  useEffect(() => {
    onRefreshRef.current = onRefresh;
  });

  const fire = useCallback(() => {
    void onRefreshRef.current();
    setSecondsLeft(intervalSec); // reset the countdown after a manual refresh
  }, [intervalSec]);

  useEffect(() => {
    if (!intervalSec) return;
    const id = window.setInterval(() => {
      if (typeof document !== "undefined" && document.hidden) return; // pause when hidden
      setSecondsLeft((s) => {
        if (s <= 1) {
          void onRefreshRef.current();
          return intervalSec;
        }
        return s - 1;
      });
    }, 1000);
    return () => window.clearInterval(id);
  }, [intervalSec]);

  function choose(sec: number) {
    setIntervalSec(sec);
    setSecondsLeft(sec);
    if (storageKey && typeof window !== "undefined") {
      window.localStorage.setItem(storageKey, String(sec));
    }
  }

  const optionLabel = (sec: number) => (sec === 0 ? t("common.off") : `${sec}s`);

  return (
    <div className={cn("flex items-center gap-1.5", className)}>
      <button
        type="button"
        onClick={fire}
        aria-label={t("common.refreshNow")}
        className="flex size-8 items-center justify-center rounded-lg border border-srapi-border bg-srapi-card text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:text-srapi-text-primary"
      >
        <RefreshCw className={cn("size-3.5", isRefreshing && "animate-spin")} />
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label={t("common.autoRefresh")}
            className={cn(
              "flex h-8 items-center gap-1.5 rounded-lg border px-2.5 font-mono text-2xs transition-colors",
              intervalSec
                ? "border-srapi-primary/30 bg-srapi-primary/5 text-srapi-primary"
                : "border-srapi-border bg-srapi-card text-srapi-text-tertiary hover:text-srapi-text-secondary",
            )}
          >
            <span className="tabular">{intervalSec ? `${secondsLeft}s` : t("common.off")}</span>
            <ChevronDown className="size-3" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-[8rem]">
          <div className="px-2 pb-1 pt-1 font-mono text-[10px] uppercase tracking-wide text-srapi-text-tertiary">
            {t("common.autoRefresh")}
          </div>
          {OPTIONS.map((sec) => (
            <DropdownMenuItem key={sec} onClick={() => choose(sec)} className="justify-between">
              <span>{optionLabel(sec)}</span>
              {sec === intervalSec ? <Check className="size-3.5 text-srapi-primary" /> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
