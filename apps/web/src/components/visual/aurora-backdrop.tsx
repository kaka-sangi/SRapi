/**
 * AuroraBackdrop — stripped to no-op. The aurora gradient blobs were removed
 * in the design cleanup. This component is kept so callers don't break.
 */
export function AuroraBackdrop(_props: {
  tone?: "hero" | "card" | "rail";
  className?: string;
  intensity?: number;
}) {
  return null;
}
