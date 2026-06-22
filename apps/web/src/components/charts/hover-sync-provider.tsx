import * as React from "react";

export interface HoverSyncValue {
  index: number | null;
  setIndex: (i: number | null) => void;
}

const HoverSyncContext = React.createContext<HoverSyncValue>({
  index: null,
  setIndex: () => {},
});

export interface HoverSyncProviderProps {
  scope?: string;
  children: React.ReactNode;
}

/**
 * Wraps a section so all charts in it share a hovered x-index — hovering one
 * trend line highlights the same bucket on every sibling chart.
 *
 * `scope` is currently a hint for future multi-scope nesting; today a single
 * context entry is shared per provider instance.
 */
export function HoverSyncProvider({ scope, children }: HoverSyncProviderProps) {
  const [index, setIndex] = React.useState<number | null>(null);
  void scope; // reserved for future per-scope routing
  const value = React.useMemo<HoverSyncValue>(() => ({ index, setIndex }), [index]);
  return <HoverSyncContext.Provider value={value}>{children}</HoverSyncContext.Provider>;
}
HoverSyncProvider.displayName = "HoverSyncProvider";

/**
 * Read+write the shared hovered x-index for the nearest `HoverSyncProvider`.
 * When invoked outside any provider, returns a no-op default so charts work
 * standalone.
 */
export function useHoverSync(scope?: string): HoverSyncValue {
  void scope;
  return React.useContext(HoverSyncContext);
}
