"use client";

import { useQuery } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { apiService } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import { useLanguage } from "@/context/LanguageContext";

/**
 * Site-wide banner that surfaces the maintenance flag pushed by the admin
 * settings page. Polls /api/v1/site-config every 60s so the banner appears
 * and disappears in close to real time without forcing a hard reload.
 *
 * The query shares its cache key with `useSiteConfig` so navigating to a
 * page that consumes site-config does not double-fetch.
 */
export function MaintenanceBanner() {
  const { t, language } = useLanguage();
  const { data } = useQuery({
    queryKey: queryKeys.siteConfig(),
    queryFn: () => apiService.getSiteConfig(),
    staleTime: 30_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: true,
  });
  const maintenance = data?.maintenance;
  if (!maintenance?.enabled) return null;

  const message = (maintenance.message ?? "").trim();
  const recoveryAt = maintenance.expected_recovery_at;
  const recoveryHint = recoveryAt ? formatRecoveryHint(t, recoveryAt, language) : null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="border-b border-srapi-warning/40 bg-srapi-warning/10 px-4 py-2 text-xs text-srapi-text-primary"
    >
      <div className="mx-auto flex max-w-7xl items-start gap-2">
        <AlertTriangle className="mt-0.5 size-4 shrink-0 text-srapi-warning" aria-hidden />
        <div className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-semibold">{t("adminSettings.maintenance.bannerTitle")}</span>
          {message ? <span className="text-srapi-text-secondary">{message}</span> : null}
          {recoveryHint ? (
            <span className="font-mono text-2xs text-srapi-text-tertiary">{recoveryHint}</span>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function formatRecoveryHint(
  t: (key: string, vars?: Record<string, string>) => string,
  iso: string,
  language: string,
): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const formatter = new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  });
  return t("adminSettings.maintenance.bannerRecoveryHint", { time: formatter.format(date) });
}
