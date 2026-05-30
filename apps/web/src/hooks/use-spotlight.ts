"use client";

import * as React from "react";

/**
 * Cursor spotlight: writes `--mx`/`--my` (pointer position relative to the
 * element) so the CSS `.spotlight` glow can follow the cursor. Returns a ref to
 * attach to the `.spotlight` container.
 *
 * No-ops under `prefers-reduced-motion: reduce` (the glow simply stays at its
 * CSS default position). Updates are rAF-throttled to one write per frame.
 */
export function useSpotlight<T extends HTMLElement>() {
  const ref = React.useRef<T | null>(null);

  React.useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      return;
    }

    let raf = 0;
    const onMove = (event: PointerEvent) => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        const rect = el.getBoundingClientRect();
        el.style.setProperty("--mx", `${event.clientX - rect.left}px`);
        el.style.setProperty("--my", `${event.clientY - rect.top}px`);
      });
    };

    el.addEventListener("pointermove", onMove);
    return () => {
      el.removeEventListener("pointermove", onMove);
      cancelAnimationFrame(raf);
    };
  }, []);

  return ref;
}
