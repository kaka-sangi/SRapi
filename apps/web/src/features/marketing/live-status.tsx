"use client";

import * as React from "react";
import { Server } from "lucide-react";
import { useRuntimeStatus } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";

/**
 * Compact API connectivity pill backed by the shared `useRuntimeStatus` query
 * (deduped/cached across the app). Shows a calm "checking" state until the
 * first probe resolves.
 */
export function LiveStatus({ className }: { className?: string }) {
  const { t } = useLanguage();
  const { data, isLoading } = useRuntimeStatus();
  const offline = data ? !data.connected : false;
  const checking = isLoading || !data;

  return (
    <span
      title={data?.apiBaseUrl ?? ""}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 font-mono text-2xs font-bold uppercase tracking-wider",
        checking
          ? "border-srapi-border bg-srapi-card-muted/60 text-srapi-text-secondary"
          : offline
            ? "border-srapi-error/30 bg-srapi-error/5 text-srapi-error"
            : "border-srapi-success/30 bg-srapi-success/5 text-srapi-success",
        className,
      )}
    >
      {checking ? (
        <Server size={11} aria-hidden="true" />
      ) : (
        <span
          aria-hidden="true"
          className={cn(
            "pulse-dot",
            offline ? "bg-srapi-error" : "bg-srapi-success",
          )}
        />
      )}
      {checking ? "…" : offline ? t("apiOffline") : t("liveApi")}
    </span>
  );
}
