/**
 * Quiet editorial backdrop for the sign-in surface.
 *
 * Design system §1.1 forbids "AI tech-slop" (glow, pulsing dots, color gradients).
 * This is a single barely-there warm vignette at the top — atmosphere, not
 * spectacle. Pure CSS, CSP-safe, no animation.
 */
export function AmbientCanvas() {
  return (
    <div
      aria-hidden
      className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[45vh]"
      style={{
        background:
          "radial-gradient(ellipse 75% 100% at 50% 0%, color-mix(in srgb, var(--color-srapi-primary) 5%, transparent), transparent 70%)",
      }}
    />
  );
}
