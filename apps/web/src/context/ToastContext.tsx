"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";
import {
  Toast,
  ToastClose,
  ToastDescription,
  ToastProvider,
  ToastTitle,
  ToastViewport,
} from "@/components/ui/toast";

export type ToastTone = "default" | "success" | "error";

interface ToastInput {
  title: string;
  description?: string;
  tone?: ToastTone;
}

interface ToastItem extends ToastInput {
  id: number;
  tone: ToastTone;
}

interface ToastContextValue {
  toast: (input: ToastInput) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

/**
 * Imperative toast store, mirroring LanguageContext's shape. Holds the active
 * queue in state and renders them through the Radix toast primitives. Pages call
 * `useToast().toast({ title, tone })` after a mutation succeeds or fails.
 */
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
        {items.map((item) => (
          <Toast
            key={item.id}
            tone={item.tone}
            onOpenChange={(open) => {
              if (!open) remove(item.id);
            }}
          >
            <div className="flex-1">
              <ToastTitle>{item.title}</ToastTitle>
              {item.description ? <ToastDescription>{item.description}</ToastDescription> : null}
            </div>
            <ToastClose />
          </Toast>
        ))}
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
