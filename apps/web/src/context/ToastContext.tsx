"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";
import { CheckCircle2, XCircle, AlertTriangle, Info } from "lucide-react";
import {
  Toast,
  ToastClose,
  ToastDescription,
  ToastProvider,
  ToastTitle,
  ToastViewport,
} from "@/components/ui/toast";
import { cn } from "@/lib/cn";

type ToastTone = "default" | "success" | "error" | "warning" | "info";

interface ToastInput {
  title: string;
  description?: string;
  tone?: ToastTone;
  /** Auto-dismiss delay (ms). Defaults longer for errors so they don't vanish before they're read. */
  duration?: number;
}

interface ToastItem extends ToastInput {
  id: number;
  tone: ToastTone;
}

interface ToastContextValue {
  toast: (input: ToastInput) => void;
}

const TONE_ICON: Record<ToastTone, React.ElementType | null> = {
  default: null,
  success: CheckCircle2,
  error: XCircle,
  warning: AlertTriangle,
  info: Info,
};

const TONE_ICON_CLASS: Record<ToastTone, string> = {
  default: "",
  success: "text-srapi-success",
  error: "text-srapi-error",
  warning: "text-srapi-warning",
  info: "text-srapi-primary",
};

const TONE_BORDER: Record<ToastTone, string> = {
  default: "border-l-srapi-border-strong",
  success: "border-l-srapi-success",
  error: "border-l-srapi-error",
  warning: "border-l-srapi-warning",
  info: "border-l-srapi-primary",
};

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastUIProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const idRef = useRef(0);

  const remove = useCallback((id: number) => {
    setItems((prev) => prev.filter((item) => item.id !== id));
  }, []);

  const toast = useCallback((input: ToastInput) => {
    idRef.current += 1;
    const id = idRef.current;
    setItems((prev) => [...prev, { tone: "default", ...input, id }]);
  }, []);

  const value = useMemo(() => ({ toast }), [toast]);

  return (
    <ToastContext.Provider value={value}>
      <ToastProvider swipeDirection="right" duration={4000}>
        {children}
        {items.map((item) => {
          const IconCmp = TONE_ICON[item.tone];
          const duration = item.duration ?? (item.tone === "error" ? 7000 : 4000);
          return (
            <Toast
              key={item.id}
              tone={item.tone === "warning" || item.tone === "info" ? "default" : item.tone}
              className={TONE_BORDER[item.tone]}
              duration={duration}
              onOpenChange={(open) => {
                if (!open) remove(item.id);
              }}
            >
              {IconCmp ? (
                <IconCmp className={cn("mt-0.5 size-4 shrink-0", TONE_ICON_CLASS[item.tone])} />
              ) : null}
              <div className="flex-1">
                <ToastTitle>{item.title}</ToastTitle>
                {item.description ? <ToastDescription>{item.description}</ToastDescription> : null}
              </div>
              <ToastClose />
              <div
                className="absolute inset-x-0 bottom-0 h-0.5 origin-left bg-srapi-border-strong"
                style={{ animation: `toast-progress ${duration}ms linear forwards` }}
              />
            </Toast>
          );
        })}
        <ToastViewport />
      </ToastProvider>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastUIProvider");
  return ctx;
}
