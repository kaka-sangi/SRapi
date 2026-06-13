"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { X, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";

export interface TourStep {
  target: string;
  title: string;
  content: string;
  placement?: "top" | "bottom" | "left" | "right";
}

interface TourContextValue {
  start: (steps: TourStep[]) => void;
  isActive: boolean;
}

const TourContext = createContext<TourContextValue | null>(null);

export function useTour() {
  const ctx = useContext(TourContext);
  if (!ctx) throw new Error("useTour must be used within TourProvider");
  return ctx;
}

const STORAGE_PREFIX = "srapi_tour_done_";

export function TourProvider({ children }: { children: React.ReactNode }) {
  const [steps, setSteps] = useState<TourStep[]>([]);
  const [currentIndex, setCurrentIndex] = useState(0);
  const [rect, setRect] = useState<DOMRect | null>(null);
  const [mounted, setMounted] = useState(false);
  const overlayRef = useRef<HTMLDivElement>(null);

  const isActive = steps.length > 0;
  const step = isActive ? steps[currentIndex] : null;

  const updateRect = useCallback(() => {
    if (!step) return;
    const el = document.querySelector(step.target);
    if (el) {
      const r = el.getBoundingClientRect();
      setRect(r);
      el.scrollIntoView({ behavior: "smooth", block: "nearest" });
    } else {
      setRect(null);
    }
  }, [step]);

  useEffect(() => {
    if (!isActive) return;
    updateRect();
    const onResize = () => updateRect();
    window.addEventListener("resize", onResize);
    window.addEventListener("scroll", onResize, true);
    return () => {
      window.removeEventListener("resize", onResize);
      window.removeEventListener("scroll", onResize, true);
    };
  }, [isActive, currentIndex, updateRect]);

  useEffect(() => setMounted(true), []);

  const start = useCallback((newSteps: TourStep[]) => {
    if (newSteps.length === 0) return;
    setSteps(newSteps);
    setCurrentIndex(0);
  }, []);

  function next() {
    if (currentIndex < steps.length - 1) {
      setCurrentIndex((i) => i + 1);
    } else {
      finish();
    }
  }

  function prev() {
    if (currentIndex > 0) setCurrentIndex((i) => i - 1);
  }

  function finish() {
    setSteps([]);
    setCurrentIndex(0);
    setRect(null);
  }

  useEffect(() => {
    if (!isActive) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") finish();
      if (e.key === "ArrowRight" || e.key === "Enter") next();
      if (e.key === "ArrowLeft") prev();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const value = useMemo(() => ({ start, isActive }), [start, isActive]);

  const popoverStyle = useMemo(() => {
    if (!rect || !step) return {};
    const placement = step.placement ?? "bottom";
    const gap = 12;
    const style: React.CSSProperties = { position: "fixed", zIndex: 10002 };
    if (placement === "bottom") {
      style.top = rect.bottom + gap;
      style.left = rect.left + rect.width / 2;
      style.transform = "translateX(-50%)";
    } else if (placement === "top") {
      style.bottom = window.innerHeight - rect.top + gap;
      style.left = rect.left + rect.width / 2;
      style.transform = "translateX(-50%)";
    } else if (placement === "right") {
      style.top = rect.top + rect.height / 2;
      style.left = rect.right + gap;
      style.transform = "translateY(-50%)";
    } else {
      style.top = rect.top + rect.height / 2;
      style.right = window.innerWidth - rect.left + gap;
      style.transform = "translateY(-50%)";
    }
    return style;
  }, [rect, step]);

  return (
    <TourContext.Provider value={value}>
      {children}
      {mounted && isActive && step
        ? createPortal(
            <>
              {/* Overlay with spotlight cutout */}
              <div
                ref={overlayRef}
                className="fixed inset-0 z-[10000] transition-opacity"
                style={{
                  background: rect
                    ? `radial-gradient(ellipse ${rect.width + 24}px ${rect.height + 24}px at ${rect.left + rect.width / 2}px ${rect.top + rect.height / 2}px, transparent 50%, rgba(0,0,0,0.5) 100%)`
                    : "rgba(0,0,0,0.5)",
                }}
                onClick={finish}
              />
              {/* Spotlight ring */}
              {rect ? (
                <div
                  className="pointer-events-none fixed z-[10001] rounded-lg ring-2 ring-srapi-primary/60"
                  style={{
                    top: rect.top - 4,
                    left: rect.left - 4,
                    width: rect.width + 8,
                    height: rect.height + 8,
                  }}
                />
              ) : null}
              {/* Popover card */}
              <div
                className="z-[10002] w-80 rounded-xl border border-srapi-border bg-srapi-card p-5 shadow-lg"
                style={popoverStyle}
                onClick={(e) => e.stopPropagation()}
              >
                <div className="flex items-start justify-between gap-2">
                  <h3 className="font-serif text-lg text-srapi-text-primary">
                    {step.title}
                  </h3>
                  <button
                    type="button"
                    onClick={finish}
                    className="shrink-0 text-srapi-text-tertiary transition-colors hover:text-srapi-text-primary"
                  >
                    <X className="size-4" />
                  </button>
                </div>
                <p className="mt-2 text-sm leading-relaxed text-srapi-text-secondary">
                  {step.content}
                </p>
                <div className="mt-4 flex items-center justify-between">
                  <span className="font-mono text-2xs text-srapi-text-tertiary">
                    {currentIndex + 1} / {steps.length}
                  </span>
                  <div className="flex gap-2">
                    {currentIndex > 0 && (
                      <Button variant="ghost" size="sm" onClick={prev}>
                        <ChevronLeft className="size-3.5" />
                      </Button>
                    )}
                    <Button variant="primary" size="sm" onClick={next}>
                      {currentIndex === steps.length - 1 ? "Done" : (
                        <>
                          Next <ChevronRight className="size-3.5" />
                        </>
                      )}
                    </Button>
                  </div>
                </div>
                {/* Step dots */}
                <div className="mt-3 flex justify-center gap-1">
                  {steps.map((_, i) => (
                    <span
                      key={i}
                      className={cn(
                        "size-1.5 rounded-full transition-colors",
                        i === currentIndex ? "bg-srapi-primary" : "bg-srapi-border",
                      )}
                    />
                  ))}
                </div>
              </div>
            </>,
            document.body,
          )
        : null}
    </TourContext.Provider>
  );
}

export function useAutoTour(tourId: string, steps: TourStep[], enabled = true) {
  const { start } = useTour();
  const hasRun = useRef(false);

  useEffect(() => {
    if (!enabled || hasRun.current) return;
    if (typeof window === "undefined") return;
    const key = STORAGE_PREFIX + tourId;
    if (localStorage.getItem(key) === "1") return;
    hasRun.current = true;
    const timer = setTimeout(() => {
      start(steps);
      localStorage.setItem(key, "1");
    }, 1000);
    return () => clearTimeout(timer);
  }, [enabled, tourId, steps, start]);
}
