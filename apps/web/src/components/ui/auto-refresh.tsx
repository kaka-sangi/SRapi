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

const OPTIONS = [0, 5, 10, 15, 30, 60] as const;

function readStored(storageKey: string | undefined, fallback: number): number {
  if (!storageKey || typeof window === "undefined") return fallback;
  const raw = window.localStorage.getItem(storageKey);
  if (raw == null) return fallback;
  const n = Number(raw);
  return (OPTIONS as readonly number[]).includes(n) ? n : fallback;
}

function hasOpenOverlay(): boolean {
  if (typeof document === "undefined") return false;
  return document.querySelector("[role='dialog'][data-state='open'], [role='alertdialog'][data-state='open'], [data-radix-popper-content-wrapper]") !== null;
}

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
  const [paused, setPaused] = useState(false);
  const onRefreshRef = useRef(onRefresh);
  useEffect(() => {
    onRefreshRef.current = onRefresh;
  });

  const fire = useCallback(() => {
    void onRefreshRef.current();
    setSecondsLeft(intervalSec);
  }, [intervalSec]);

  useEffect(() => {
    if (!intervalSec) return;
    const id = window.setInterval(() => {
      if (typeof document !== "undefined" && document.hidden) {
        setPaused(true);
        return;
      }
      if (hasOpenOverlay()) {
        setPaused(true);
        return;
      }
      setPaused(false);
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
  const progress = intervalSec ? secondsLeft / intervalSec : 0;

  return (
    <div className={cn("flex items-center gap-1.5", className)}>
      <button
        type="button"
        onClick={fire}
        aria-label={t("common.refreshNow")}
        className="flex size-8 items-center justify-center rounded-xl border border-srapi-border bg-srapi-card text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:text-srapi-text-primary"
      >
        <RefreshCw className={cn("size-3.5", isRefreshing && "animate-spin")} />
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label={t("common.autoRefresh")}
            className={cn(
              "relative flex h-8 items-center gap-1.5 overflow-hidden rounded-xl border px-2.5 text-[11px] font-medium transition-colors",
              intervalSec
                ? "border-transparent bg-srapi-accent-soft text-srapi-primary"
                : "border-srapi-border bg-srapi-card text-srapi-text-tertiary hover:text-srapi-text-secondary",
            )}
          >
            {intervalSec ? (
              <span
                className="absolute inset-y-0 left-0 bg-srapi-primary/15 transition-[width] duration-1000 ease-linear"
                style={{ width: `${progress * 100}%` }}
              />
            ) : null}
            <span className="relative tabular">
              {intervalSec
                ? paused
                  ? `⏸ ${secondsLeft}s`
                  : `${secondsLeft}s`
                : t("common.off")}
            </span>
            <ChevronDown className="relative size-3" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-[8rem]">
          <div className="px-2 pb-1 pt-1 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
