"use client";

import * as React from "react";
import { useLanguage } from "@/context/LanguageContext";
import { useSmokeStatus } from "@/hooks/queries";
import { cn } from "@/lib/cn";
import { SmokeDrawer } from "./smoke-drawer";

interface PageHeaderProps {
  category: string;
  title: string;
  description: string;
  user: { name: string; balance?: string };
  showSmoke?: boolean;
}

export function PageHeader({ category, title, description, user, showSmoke = false }: PageHeaderProps) {
  const { t } = useLanguage();
  const { data: smokeStatus } = useSmokeStatus(undefined, { enabled: showSmoke });
  const [isSmokeDrawerOpen, setIsSmokeDrawerOpen] = React.useState(false);

  return (
    <>
      <div className="mb-12 flex flex-col justify-between gap-6 border-b border-srapi-border pb-8 animate-bloom delay-100 md:flex-row md:items-end">
        <div className="max-w-3xl space-y-2.5">
          <div className="font-mono text-[11px] font-bold uppercase tracking-widest text-srapi-primary">
            {category}
          </div>
          <h2 className="font-serif text-3xl font-normal leading-tight tracking-tight text-srapi-text-primary md:text-4xl">
            {title}
          </h2>
          <p className="max-w-2xl text-xs leading-relaxed text-srapi-text-secondary">
            {description}
          </p>
        </div>

        <div className="flex shrink-0 flex-col items-start gap-3 md:items-end">
          <div className="space-y-0.5 rounded-xl border border-srapi-border bg-srapi-card-muted/50 px-3 py-1.5 text-right font-mono text-[11px]">
            <div>
              {t("operatorName")}: <span className="font-bold text-srapi-text-primary">{user.name}</span>
            </div>
            {user.balance ? (
              <div>
                {t("availableBalance")}:{" "}
                <span className="font-bold text-srapi-primary">${user.balance} USD</span>
              </div>
            ) : null}
          </div>

          {smokeStatus ? (
            <button
              type="button"
              onClick={() => setIsSmokeDrawerOpen(true)}
              className={cn(
                "quiet-badge cursor-pointer rounded-full transition-all hover:bg-srapi-card-muted",
                smokeStatus.v0_1_smoke_evidence_complete
                  ? "border-srapi-success bg-srapi-success/5 font-semibold text-srapi-success"
                  : "border-srapi-error bg-srapi-error/5 text-srapi-error hover:border-srapi-error/60",
              )}
            >
              <span
                aria-hidden="true"
                className={cn(
                  "h-1.5 w-1.5 rounded-full",
                  smokeStatus.v0_1_smoke_evidence_complete
                    ? "animate-pulse bg-srapi-success"
                    : "bg-srapi-error",
                )}
              />
              {t("smokeEvidence")}:{" "}
              {smokeStatus.v0_1_smoke_evidence_complete ? t("complete") : t("notComplete")}
            </button>
          ) : null}
        </div>
      </div>

      {smokeStatus ? (
        <SmokeDrawer
          open={isSmokeDrawerOpen}
          onClose={() => setIsSmokeDrawerOpen(false)}
          status={smokeStatus}
        />
      ) : null}
    </>
  );
}
