"use client";

import * as React from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

/** Copy `text`, preferring the async Clipboard API with a legacy fallback for
 *  insecure / older contexts so the button never silently no-ops. */
async function writeClipboard(text: string): Promise<boolean> {
  try {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to the legacy execCommand path below
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

/**
 * One-click copy affordance. Swaps to a green check + flashes a "Copied" tooltip
 * for ~1.4s, then settles back. Tooltip stays controlled (never flips between
 * controlled/uncontrolled) so hovering and the copied flash compose cleanly.
 * Stops click propagation so it can sit inside a clickable row without
 * triggering the row's handler.
 */
export function CopyButton({
  value,
  label,
  className,
  size = "icon",
}: {
  /** Text placed on the clipboard. */
  value: string;
  /** Accessible name + resting tooltip; defaults to the shared "Copy" string. */
  label?: string;
  className?: string;
  /** `icon` = standalone 28px hit area; `inline` = compact, sits beside text. */
  size?: "icon" | "inline";
}) {
  const { t } = useLanguage();
  const [open, setOpen] = React.useState(false);
  const [copied, setCopied] = React.useState(false);
  const timer = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  React.useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  async function handleCopy(event: React.MouseEvent) {
    event.preventDefault();
    event.stopPropagation();
    if (!(await writeClipboard(value))) return;
    setCopied(true);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 1400);
  }

  const name = copied ? t("common.copied") : (label ?? t("common.copy"));
  const iconSize = size === "icon" ? "size-3.5" : "size-3";

  return (
    <Tooltip open={open || copied} onOpenChange={setOpen}>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={handleCopy}
          aria-label={name}
          className={cn(
            "inline-flex shrink-0 items-center justify-center rounded-md text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary active:scale-95",
            size === "icon" ? "size-7" : "size-5",
            className,
          )}
        >
          {copied ? (
            <Check className={cn(iconSize, "anim-pop-in text-srapi-success")} aria-hidden />
          ) : (
            <Copy className={iconSize} aria-hidden />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent>{name}</TooltipContent>
    </Tooltip>
  );
}

/**
 * A monospaced value paired with a copy button — the standard way to present an
 * ID, key prefix, endpoint URL, or invite code so it both reads cleanly and is
 * one click to grab. Truncates long values; the copy still grabs the full text.
 */
export function CopyableValue({
  value,
  display,
  label,
  className,
  mono = true,
}: {
  value: string;
  /** Optional shorter label to render; the full `value` is what gets copied. */
  display?: React.ReactNode;
  label?: string;
  className?: string;
  mono?: boolean;
}) {
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-1", className)}>
      <span className={cn("truncate", mono && "font-mono")}>{display ?? value}</span>
      <CopyButton value={value} label={label} size="inline" />
    </span>
  );
}
