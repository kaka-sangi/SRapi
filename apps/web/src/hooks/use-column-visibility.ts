"use client";

import { useCallback, useState } from "react";

const PREFIX = "srapi_cols_";

function readStored(storageKey: string, defaultHidden: string[]): Set<string> {
  if (typeof window === "undefined") return new Set(defaultHidden);
  const raw = window.localStorage.getItem(PREFIX + storageKey);
  if (raw == null) return new Set(defaultHidden);
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? new Set(parsed) : new Set(defaultHidden);
  } catch {
    return new Set(defaultHidden);
  }
}

function persist(storageKey: string, hidden: Set<string>) {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(PREFIX + storageKey, JSON.stringify([...hidden]));
}

export interface ColumnVisibility {
  hidden: Set<string>;
  toggle: (key: string) => void;
  isVisible: (key: string) => boolean;
  reset: () => void;
}

export function useColumnVisibility(
  storageKey: string,
  defaultHidden: string[] = [],
): ColumnVisibility {
  const [hidden, setHidden] = useState<Set<string>>(() =>
    readStored(storageKey, defaultHidden),
  );

  const toggle = useCallback(
    (key: string) => {
      setHidden((prev) => {
        const next = new Set(prev);
        if (next.has(key)) {
          next.delete(key);
        } else {
          next.add(key);
        }
        persist(storageKey, next);
        return next;
      });
    },
    [storageKey],
  );

  const isVisible = useCallback((key: string) => !hidden.has(key), [hidden]);

  const reset = useCallback(() => {
    const fresh = new Set(defaultHidden);
    setHidden(fresh);
    persist(storageKey, fresh);
  }, [storageKey, defaultHidden]);

  return { hidden, toggle, isVisible, reset };
}
