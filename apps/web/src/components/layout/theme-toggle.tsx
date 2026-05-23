"use client";

import * as React from "react";
import { useTheme } from "next-themes";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";

const subscribeToNothing = () => () => {};
const getServerSnapshot = () => false;
const getClientSnapshot = () => true;

/**
 * SRapi v0.1.0 theme toggle. Light/dark only; honors system preference until
 * the user opts in. Backed by `next-themes` so there is no first-paint flash.
 *
 * `useSyncExternalStore` returns false during server render and true after
 * hydration, which lets us hold off on rendering theme-dependent UI until
 * the client knows the resolved theme. This avoids the lint warning that
 * comes from the legacy `useEffect(() => setMounted(true))` pattern.
 */
export function ThemeToggle({ className }: { className?: string }) {
  const { resolvedTheme, setTheme } = useTheme();
  const { t } = useLanguage();
  const mounted = React.useSyncExternalStore(
    subscribeToNothing,
    getClientSnapshot,
    getServerSnapshot,
  );

  const isDark = mounted && resolvedTheme === "dark";

  return (
    <button
      type="button"
      role="switch"
      aria-checked={isDark}
      aria-label={t("themeToggle")}
      title={t("themeToggle")}
      onClick={() => setTheme(isDark ? "light" : "dark")}
      className={cn(
        "relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border border-srapi-border bg-srapi-card-muted transition-colors",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary focus-visible:ring-offset-2 focus-visible:ring-offset-srapi-bg",
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "pointer-events-none inline-block h-4 w-4 transform rounded-full bg-srapi-text-primary shadow-sm transition-transform",
          isDark ? "translate-x-4" : "translate-x-0",
        )}
      />
    </button>
  );
}
