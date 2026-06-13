"use client";

import { useEffect, useRef, useState } from "react";

export interface AdminEvent {
  type: string;
  data: unknown;
}

export type AdminEventHandler = (event: AdminEvent) => void;

export function useAdminEventStream(
  onEvent?: AdminEventHandler,
  enabled = true,
): { connected: boolean } {
  const [connected, setConnected] = useState(false);
  const onEventRef = useRef(onEvent);
  useEffect(() => {
    onEventRef.current = onEvent;
  });

  useEffect(() => {
    if (!enabled || typeof window === "undefined") return;

    const base = (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || "").replace(/\/+$/, "");
    const url = `${base}/api/v1/admin/events`;
    let es: EventSource | null = null;
    let retryTimeout: ReturnType<typeof setTimeout>;
    let unmounted = false;

    function connect() {
      if (unmounted) return;
      es = new EventSource(url, { withCredentials: true });

      es.addEventListener("ping", () => {
        setConnected(true);
      });

      es.addEventListener("circuit_breaker", (e) => {
        try {
          const data = JSON.parse(e.data);
          onEventRef.current?.({ type: "circuit_breaker", data });
        } catch { /* ignore parse errors */ }
      });

      es.addEventListener("health_change", (e) => {
        try {
          const data = JSON.parse(e.data);
          onEventRef.current?.({ type: "health_change", data });
        } catch { /* ignore */ }
      });

      es.addEventListener("cache_clear", (e) => {
        try {
          const data = JSON.parse(e.data);
          onEventRef.current?.({ type: "cache_clear", data });
        } catch { /* ignore */ }
      });

      es.onerror = () => {
        setConnected(false);
        es?.close();
        es = null;
        retryTimeout = setTimeout(connect, 5000);
      };
    }

    connect();

    return () => {
      unmounted = true;
      clearTimeout(retryTimeout);
      es?.close();
      setConnected(false);
    };
  }, [enabled]);

  return { connected };
}
