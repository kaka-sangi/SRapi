"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import { cn } from "@/lib/cn";

/**
 * §5 流式打字输出：逐字渲染调度决策日志行，行末微光游标。
 * 尊重 prefers-reduced-motion：直接整块显示，无逐字与游标动画。
 *
 * State setters are only ever called from inside a setTimeout callback (never
 * synchronously in the effect body), satisfying react-hooks/set-state-in-effect.
 */
export function SchedulerDecisionStream({
  lines,
  className,
  charDelayMs = 10,
  lineDelayMs = 200,
}: {
  lines: string[];
  className?: string;
  charDelayMs?: number;
  lineDelayMs?: number;
}) {
  const reduced = usePrefersReducedMotion();
  // Number of lines fully rendered, and how many chars of the in-progress line.
  const [lineIdx, setLineIdx] = useState(0);
  const [charIdx, setCharIdx] = useState(0);

  // Re-arm the animation whenever the source lines change.
  const key = lines.join("\n");
  useEffect(() => {
    if (reduced) return;
    let cancelled = false;
    let li = 0;
    let ci = 0;
    let timer: ReturnType<typeof setTimeout>;

    const tick = () => {
      if (cancelled) return;
      if (li >= lines.length) return;
      const line = lines[li];
      if (ci < line.length) {
        ci += 1;
        setCharIdx(ci);
        timer = setTimeout(tick, charDelayMs + Math.random() * 8);
      } else {
        li += 1;
        ci = 0;
        setLineIdx(li);
        setCharIdx(0);
        timer = setTimeout(tick, lineDelayMs);
      }
    };

    timer = setTimeout(tick, lineDelayMs);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
    // key re-arms when the decision changes; setters reset via the closure.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, reduced, charDelayMs, lineDelayMs]);

  const baseClass = cn(
    "rounded-xl bg-srapi-card-muted p-4 font-mono text-xs leading-relaxed text-srapi-text-secondary",
    className,
  );

  if (reduced) {
    return (
      <div className={baseClass} role="log" aria-live="polite">
        {lines.map((line, i) => (
          <div key={i} className="stream-line">
            {highlight(line)}
          </div>
        ))}
      </div>
    );
  }

  const rendered = lines.slice(0, lineIdx);
  const typing = lineIdx < lines.length ? lines[lineIdx].slice(0, charIdx) : "";
  const done = lineIdx >= lines.length;

  return (
    <div className={baseClass} role="log" aria-live="polite">
      {rendered.map((line, i) => (
        <div key={i} className="stream-line">
          {highlight(line)}
        </div>
      ))}
      {!done && typing && (
        <div className="stream-line">
          {highlight(typing)}
          <span className="stream-cursor" aria-hidden />
        </div>
      )}
    </div>
  );
}

function highlight(line: string) {
  // Emphasize the key verbs the scheduler emits (selected / received).
  const parts = line.split(/(selected|received)/);
  return parts.map((part, i) =>
    part === "selected" || part === "received" ? (
      <span key={i} className="font-semibold text-srapi-primary">
        {part}
      </span>
    ) : (
      <span key={i}>{part}</span>
    ),
  );
}

function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(
    (cb) => {
      const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
      mq.addEventListener("change", cb);
      return () => mq.removeEventListener("change", cb);
    },
    () => window.matchMedia("(prefers-reduced-motion: reduce)").matches,
    () => false,
  );
}
