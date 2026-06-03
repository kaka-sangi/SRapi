"use client";

import { useEffect, useState } from "react";

/**
 * Debounce a fast-changing value (e.g. a search box) so list queries fire only
 * after the user pauses typing. Returns the latest value once `delayMs` has
 * elapsed without a change.
 */
export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const handle = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(handle);
  }, [value, delayMs]);

  return debounced;
}
