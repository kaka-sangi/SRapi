"use client";

import * as React from "react";

interface RevealOptions {
  /** Reveal only once, then stop observing. Default true. */
  once?: boolean;
  /** IntersectionObserver rootMargin. Default reveals slightly before fully in view. */
  rootMargin?: string;
  /** Visibility threshold. Default 0.12. */
  threshold?: number;
}

/**
 * Attach the returned ref to an element that also carries the `.reveal` class.
 * When it scrolls into view the hook adds `is-visible`, triggering the CSS
 * transition. Degrades gracefully: with reduced-motion or no IntersectionObserver
 * the element is shown immediately.
 */
export function useReveal<T extends HTMLElement = HTMLDivElement>(options?: RevealOptions) {
  const ref = React.useRef<T | null>(null);
  const once = options?.once ?? true;
  const rootMargin = options?.rootMargin ?? "0px 0px -10% 0px";
  const threshold = options?.threshold ?? 0.12;

  React.useEffect(() => {
    const el = ref.current;
    if (!el) return;

    const prefersReduced =
      typeof window !== "undefined" &&
      typeof window.matchMedia === "function" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    if (prefersReduced || typeof IntersectionObserver === "undefined") {
      el.classList.add("is-visible");
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            entry.target.classList.add("is-visible");
            if (once) observer.unobserve(entry.target);
          } else if (!once) {
            entry.target.classList.remove("is-visible");
          }
        }
      },
      { rootMargin, threshold },
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, [once, rootMargin, threshold]);

  return ref;
}
