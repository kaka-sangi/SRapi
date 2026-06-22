import * as React from "react";

export interface UseListKeyboardNavProps {
  rowIds: string[];
  onActivate?: (id: string) => void;
  enabled?: boolean;
}

export interface UseListKeyboardNavReturn {
  active: string | null;
  setActive: (id: string | null) => void;
  bindRoot: {
    onKeyDown: React.KeyboardEventHandler;
    tabIndex: 0;
  };
}

/**
 * j/k/Enter/Esc/Home/End list-navigation hook.
 *
 * - ArrowDown / j: next row
 * - ArrowUp   / k: previous row
 * - Home: first row
 * - End:  last row
 * - Enter: invoke `onActivate(active)`
 * - Escape: clear active selection
 *
 * `bindRoot` is intended to be spread onto a focusable container element.
 */
export function useListKeyboardNav({
  rowIds,
  onActivate,
  enabled = true,
}: UseListKeyboardNavProps): UseListKeyboardNavReturn {
  const [active, setActive] = React.useState<string | null>(null);

  // Keep a ref of the latest rowIds so the keyDown handler stays referentially
  // stable across re-renders while still seeing fresh data.
  const rowIdsRef = React.useRef(rowIds);
  React.useEffect(() => {
    rowIdsRef.current = rowIds;
  }, [rowIds]);

  // If the active id is removed, drop the selection.
  React.useEffect(() => {
    if (active != null && !rowIds.includes(active)) {
      setActive(null);
    }
  }, [active, rowIds]);

  const enabledRef = React.useRef(enabled);
  React.useEffect(() => {
    enabledRef.current = enabled;
  }, [enabled]);

  const onActivateRef = React.useRef(onActivate);
  React.useEffect(() => {
    onActivateRef.current = onActivate;
  }, [onActivate]);

  const onKeyDown = React.useCallback<React.KeyboardEventHandler>((event) => {
    if (!enabledRef.current) return;
    const ids = rowIdsRef.current;
    if (ids.length === 0) return;

    const move = (next: string | null) => {
      event.preventDefault();
      setActive(next);
    };

    const key = event.key;
    const currentIndex = active != null ? ids.indexOf(active) : -1;

    if (key === "ArrowDown" || key === "j") {
      const nextIndex = currentIndex < 0 ? 0 : Math.min(currentIndex + 1, ids.length - 1);
      move(ids[nextIndex] ?? null);
      return;
    }
    if (key === "ArrowUp" || key === "k") {
      const nextIndex = currentIndex < 0 ? ids.length - 1 : Math.max(currentIndex - 1, 0);
      move(ids[nextIndex] ?? null);
      return;
    }
    if (key === "Home") {
      move(ids[0] ?? null);
      return;
    }
    if (key === "End") {
      move(ids[ids.length - 1] ?? null);
      return;
    }
    if (key === "Enter") {
      if (active != null) {
        event.preventDefault();
        onActivateRef.current?.(active);
      }
      return;
    }
    if (key === "Escape") {
      if (active != null) {
        event.preventDefault();
        setActive(null);
      }
    }
  }, [active]);

  return {
    active,
    setActive,
    bindRoot: { onKeyDown, tabIndex: 0 },
  };
}
