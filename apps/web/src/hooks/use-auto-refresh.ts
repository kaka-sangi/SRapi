"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const PREFIX = "srapi_autorefresh_";
const DEFAULT_INTERVALS = [5, 10, 15, 30, 60];

// ---------------------------------------------------------------------------
// localStorage helpers
// ---------------------------------------------------------------------------

function readStored(
  storageKey: string,
  intervals: number[],
  defaultEnabled: boolean,
  defaultInterval: number,
): { enabled: boolean; interval: number } {
  if (typeof window === "undefined") {
    return { enabled: defaultEnabled, interval: defaultInterval };
  }
  const raw = window.localStorage.getItem(PREFIX + storageKey);
  if (raw == null) {
    return { enabled: defaultEnabled, interval: defaultInterval };
  }
  try {
    const parsed = JSON.parse(raw);
    if (
      typeof parsed === "object" &&
      parsed !== null &&
      typeof parsed.enabled === "boolean" &&
      typeof parsed.interval === "number"
    ) {
      return {
        enabled: parsed.enabled,
        interval: intervals.includes(parsed.interval)
          ? parsed.interval
          : defaultInterval,
      };
    }
  } catch {
    // corrupted – fall through
  }
  return { enabled: defaultEnabled, interval: defaultInterval };
}

function persist(storageKey: string, enabled: boolean, interval: number) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(
    PREFIX + storageKey,
    JSON.stringify({ enabled, interval }),
  );
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface UseAutoRefreshOptions {
  defaultEnabled?: boolean;
  defaultInterval?: number; // seconds
  intervals?: number[]; // available intervals
  storageKey?: string;
}

export interface UseAutoRefreshReturn {
  enabled: boolean;
  interval: number;
  toggle: () => void;
  setInterval: (seconds: number) => void;
  intervalOptions: number[];
  timeUntilRefresh: number;
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useAutoRefresh(
  onRefresh: () => void | Promise<void>,
  options?: UseAutoRefreshOptions,
): UseAutoRefreshReturn {
  const {
    defaultEnabled = false,
    defaultInterval = 30,
    intervals = DEFAULT_INTERVALS,
    storageKey = "default",
  } = options ?? {};

  // Keep the callback ref-stable so the timer never goes stale.
  const onRefreshRef = useRef(onRefresh);
  useEffect(() => {
    onRefreshRef.current = onRefresh;
  });

  const [enabled, setEnabled] = useState<boolean>(() =>
    readStored(storageKey, intervals, defaultEnabled, defaultInterval).enabled,
  );
  const [interval, setIntervalState] = useState<number>(() =>
    readStored(storageKey, intervals, defaultEnabled, defaultInterval).interval,
  );
  const [timeUntilRefresh, setTimeUntilRefresh] = useState(interval);

  // --- toggle ---------------------------------------------------------------

  const toggle = useCallback(() => {
    setEnabled((prev) => {
      const next = !prev;
      persist(storageKey, next, interval);
      if (next) setTimeUntilRefresh(interval);
      return next;
    });
  }, [storageKey, interval]);

  // --- setInterval ----------------------------------------------------------

  const setInterval = useCallback(
    (seconds: number) => {
      setIntervalState(seconds);
      setTimeUntilRefresh(seconds);
      persist(storageKey, enabled, seconds);
    },
    [storageKey, enabled],
  );

  // --- countdown + refresh --------------------------------------------------

  useEffect(() => {
    if (!enabled || !interval) return;

    // Countdown is seeded by the state initializer and reset in toggle()/setInterval(),
    // so this effect only owns the tick loop.
    const id = window.setInterval(() => {
      // Pause while tab is hidden.
      if (typeof document !== "undefined" && document.hidden) return;

      setTimeUntilRefresh((prev) => {
        if (prev <= 1) {
          void onRefreshRef.current();
          return interval;
        }
        return prev - 1;
      });
    }, 1000);

    return () => window.clearInterval(id);
  }, [enabled, interval]);

  return {
    enabled,
    interval,
    toggle,
    setInterval,
    intervalOptions: intervals,
    timeUntilRefresh,
  };
}
