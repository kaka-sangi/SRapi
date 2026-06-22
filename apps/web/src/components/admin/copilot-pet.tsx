"use client";

import { useState } from "react";
import { usePathname } from "next/navigation";
import Link from "next/link";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X, ExternalLink, Keyboard } from "lucide-react";
import { CopilotChat } from "@/components/admin/copilot-chat";
import { useAdminCopilotConfig } from "@/hooks/admin-queries";
import { useCopilotSession } from "@/context/CopilotSessionContext";
import { useLanguage } from "@/context/LanguageContext";
import { ADMIN_ROUTES } from "@/lib/routes";
import { Kbd, KbdShortcut } from "@/components/ui/kbd";

/**
 * 小r — the admin AI copilot as a pixel-art crab pinned to the bottom-right.
 * Clicking it expands a non-modal floating panel (the full copilot: history
 * sidebar + chat) anchored at the corner; closing collapses back to the pet.
 * This is the only entry to the copilot (it's no longer in the sidebar).
 */
export function CopilotPet() {
  const { t } = useLanguage();
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const { data: cfg } = useAdminCopilotConfig();
  const { running } = useCopilotSession();

  // The /admin/copilot full page already IS the copilot — don't double up there.
  if (pathname === ADMIN_ROUTES.copilot) return null;
  // Only show 小r when the copilot is actually usable.
  if (!cfg?.enabled || !cfg?.configured) return null;

  return (
    <DialogPrimitive.Root open={open} onOpenChange={setOpen} modal={false}>
      {!open ? (
        <DialogPrimitive.Trigger asChild>
          <button type="button" aria-label={t("copilot.petOpen")} className="group fixed bottom-6 right-6 z-[55] outline-none">
            <span className="absolute right-full top-1/2 mr-2 -translate-y-1/2 whitespace-nowrap rounded-full bg-srapi-invert px-2.5 py-1 text-[11px] font-medium text-srapi-invert-fg opacity-0 shadow-sm transition-opacity group-hover:opacity-100">
              {t("copilot.petName")}
            </span>
            <span className="grid size-16 place-items-center rounded-full border border-srapi-border bg-srapi-card shadow-[0_10px_30px_-12px_rgba(28,26,23,0.28)] transition-transform duration-200 ease-[cubic-bezier(0.34,1.4,0.5,1)] group-hover:-translate-y-1 group-hover:scale-105 group-active:scale-95">
              <span className="srapi-pet-breathe block">
                <PetFace running={running} size={48} />
              </span>
            </span>
          </button>
        </DialogPrimitive.Trigger>
      ) : null}

      <DialogPrimitive.Portal>
        <DialogPrimitive.Content
          className="srapi-anim-pet-panel card-raised glass-frosted-strong fixed bottom-6 right-6 z-50 flex h-[min(760px,84vh)] w-[min(920px,92vw)] flex-col overflow-hidden rounded-2xl border border-srapi-border outline-none"
          onInteractOutside={(e) => e.preventDefault()}
        >
          <DialogPrimitive.Title className="sr-only">
            {t("copilot.petName")} · {t("copilot.title")}
          </DialogPrimitive.Title>
          <header className="flex shrink-0 items-center justify-between border-b border-srapi-border px-4 py-3">
            <div className="flex items-center gap-2.5">
              <span className="grid size-9 place-items-center rounded-xl bg-srapi-accent-soft">
                <PetFace size={24} />
              </span>
              <div className="leading-tight">
                <div className="text-sm font-semibold tracking-tight text-srapi-text-primary">
                  {t("copilot.petName")}
                </div>
                <div className="text-[11px] text-srapi-text-tertiary">{t("copilot.title")}</div>
              </div>
            </div>
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => setShowShortcuts((v) => !v)}
                aria-label={t("copilot.petShortcuts")}
                aria-pressed={showShortcuts}
                title={t("copilot.petShortcuts")}
                className="rounded-lg p-2 text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary aria-[pressed=true]:bg-srapi-accent-soft aria-[pressed=true]:text-srapi-primary"
              >
                <Keyboard className="size-4" />
              </button>
              <Link
                href={ADMIN_ROUTES.copilot}
                onClick={() => setOpen(false)}
                aria-label={t("copilot.petOpenFull")}
                title={t("copilot.petOpenFull")}
                className="rounded-lg p-2 text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary"
              >
                <ExternalLink className="size-4" />
              </Link>
              <button
                type="button"
                onClick={() => setOpen(false)}
                aria-label={t("copilot.petClose")}
                className="rounded-lg p-2 text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary"
              >
                <X className="size-4" />
              </button>
            </div>
          </header>
          {showShortcuts ? <ShortcutsList /> : null}
          <div className="min-h-0 flex-1 px-4 pb-3">
            <CopilotChat models={cfg.models ?? []} defaultModel={cfg.model ?? ""} />
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

/** Compact shortcut reference revealed under the header on demand. Pure
 * recall-aid — keyboard binding is implemented in the chat composer itself. */
function ShortcutsList() {
  const { t } = useLanguage();
  const rows: Array<{ label: string; keys: React.ReactNode[] }> = [
    { label: t("copilot.shortcutSend"), keys: ["Enter"] },
    { label: t("copilot.shortcutNewline"), keys: ["⇧", "Enter"] },
    { label: t("copilot.shortcutStop"), keys: ["Esc"] },
    { label: t("copilot.shortcutNewChat"), keys: ["⌘", "K"] },
  ];
  return (
    <div className="anim-rise-sm shrink-0 border-b border-srapi-border bg-srapi-card-muted/40 px-4 py-2.5">
      <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
        {t("copilot.petShortcuts")}
      </div>
      <ul className="grid grid-cols-2 gap-x-4 gap-y-1.5 text-[11px] text-srapi-text-secondary">
        {rows.map((row) => (
          <li key={row.label} className="flex items-center justify-between gap-2">
            <span className="truncate">{row.label}</span>
            {row.keys.length === 1 ? (
              <Kbd>{row.keys[0]}</Kbd>
            ) : (
              <KbdShortcut keys={row.keys} separator=" " />
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

// 小r — an original 8-bit crab sprite (warm coral body, side claws, little legs,
// a small cap). Drawn as a pixel grid: each char maps to a 1×1 <rect>.
const PET_PALETTE: Record<string, string> = {
  B: "#c9785c", // coral body
  D: "#a85a3e", // darker coral (bottom shade + legs)
  E: "#2b211c", // eye
  W: "#ffffff", // eye highlight
  H: "#3f6f63", // cap
  b: "#f0d9a8", // cap brim
};
const PET_GRID = [
  ".......HHHH.......",
  "......HHHHHH......",
  ".....bbbbbbbb.....",
  ".....BB....BB.....",
  "...BBWEBBBBWEBB...",
  "BB.BBBBBBBBBBBB.BB",
  "B...BBBBBBBBBB...B",
  "...DBBBBBBBBBBD...",
  "....DDDDDDDDDD....",
  "....D..D..D..D....",
];
const PET_COLS = PET_GRID[0].length;
const PET_ROWS = PET_GRID.length;

/** The 小r pixel crab. `size` is the rendered width; height follows the sprite
 * aspect. While the copilot is working, a thinking indicator bobs above it. */
function PetFace({ running, size = 56 }: { running?: boolean; size?: number }) {
  const width = size;
  const height = (size * PET_ROWS) / PET_COLS;
  return (
    <span className="relative inline-block" style={{ width, height }}>
      {running ? (
        <span className="absolute -top-2 left-1/2 flex -translate-x-1/2 gap-0.5">
          {[0, 1, 2].map((i) => (
            <span
              key={i}
              className="size-1 rounded-full bg-srapi-primary motion-safe:animate-bounce"
              style={{ animationDelay: `${i * 120}ms` }}
            />
          ))}
        </span>
      ) : null}
      <svg
        viewBox={`0 0 ${PET_COLS} ${PET_ROWS}`}
        width={width}
        height={height}
        shapeRendering="crispEdges"
        className="overflow-visible"
        aria-hidden="true"
      >
        {PET_GRID.map((row, y) =>
          row.split("").map((ch, x) => {
            const fill = PET_PALETTE[ch];
            return fill ? <rect key={`${x}-${y}`} x={x} y={y} width={1} height={1} fill={fill} /> : null;
          }),
        )}
      </svg>
    </span>
  );
}
