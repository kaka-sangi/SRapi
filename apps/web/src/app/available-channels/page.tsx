"use client";

import { useState, useMemo } from "react";
import { Search, CheckCircle2, AlertTriangle, XCircle, ChevronDown } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { SectionHero } from "@/components/visual/section-hero";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { Card, CardContent } from "@/components/ui/card";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import { useAvailableModels } from "@/hooks/queries";
import { formatMoney } from "@/lib/admin-format";
import type { AvailableModelChannelSummary, AvailableModelSummary } from "@/lib/srapi-types";

export default function AvailableChannelsPage() {
  return (
    <AppShell allowedRole="user">
      <AvailableChannelsContent />
    </AppShell>
  );
}

function AvailableChannelsContent() {
  const { t } = useLanguage();
  const availableModels = useAvailableModels();
  const [search, setSearch] = useState("");

  return (
    <>
      <SectionHero
        eyebrow="Workspace · Channels"
        title={t("availableChannels.title")}
        description={t("availableChannels.subtitle")}
        metrics={
          availableModels.data && availableModels.data.length > 0
            ? [
                { label: "Models", value: availableModels.data.length },
                {
                  label: "Channels",
                  value: availableModels.data.reduce((n, m) => n + m.channels.length, 0),
                },
              ]
            : undefined
        }
      />
      <PageQueryState query={availableModels} skeleton={<DialogListSkeleton rows={6} />}>
        {(models) =>
          models.length === 0 ? (
            <IllustratedEmptyState
              illust="inbox"
              title={t("availableChannels.emptyTitle")}
              description={t("availableChannels.emptyBody")}
            />
          ) : (
            <StatusBoard models={models} search={search} onSearchChange={setSearch} />
          )
        }
      </PageQueryState>
    </>
  );
}

function overallStatus(models: AvailableModelSummary[]): "operational" | "degraded" | "outage" {
  const total = models.length;
  if (total === 0) return "operational";
  const unavailable = models.filter((m) => m.status === "unavailable").length;
  const limited = models.filter((m) => m.status === "limited").length;
  if (unavailable === total) return "outage";
  if (unavailable > 0 || limited > 0) return "degraded";
  return "operational";
}

function StatusBoard({
  models,
  search,
  onSearchChange,
}: {
  models: AvailableModelSummary[];
  search: string;
  onSearchChange: (v: string) => void;
}) {
  const { t } = useLanguage();
  const filtered = useMemo(() => {
    if (!search.trim()) return models;
    const q = search.trim().toLowerCase();
    return models.filter(
      (m) =>
        m.name.toLowerCase().includes(q) ||
        m.id.toLowerCase().includes(q) ||
        m.channels.some((c) => c.provider_display_name.toLowerCase().includes(q)),
    );
  }, [models, search]);

  const status = overallStatus(models);

  return (
    <div className="space-y-4">
      {/* Overall status banner */}
      <Card className={cn(
        "border-l-4",
        status === "operational" && "border-l-emerald-500",
        status === "degraded" && "border-l-amber-500",
        status === "outage" && "border-l-red-500",
      )}>
        <CardContent className="flex items-center gap-3 py-4">
          {status === "operational" ? (
            <CheckCircle2 className="size-6 text-emerald-500" />
          ) : status === "degraded" ? (
            <AlertTriangle className="size-6 text-amber-500" />
          ) : (
            <XCircle className="size-6 text-red-500" />
          )}
          <div>
            <div className="text-sm font-semibold text-srapi-text-primary">
              {t(`availableChannels.overall_${status}`)}
            </div>
            <div className="text-xs text-srapi-text-tertiary">
              {t("availableChannels.overallSub", {
                available: models.filter((m) => m.status === "available").length,
                total: models.length,
              })}
            </div>
          </div>
          <div className="ml-auto">
            <div className="relative w-48 sm:w-56">
              <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-srapi-text-tertiary" />
              <Input
                value={search}
                onChange={(e) => onSearchChange(e.target.value)}
                placeholder={t("common.search")}
                className="pl-8 text-xs"
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Model status rows */}
      <div className="space-y-2">
        {filtered.map((model) => (
          <ModelStatusRow key={model.id} model={model} />
        ))}
      </div>

      {filtered.length === 0 && search && (
        <IllustratedEmptyState
          illust="search"
          title={t("adminCommon.noResults")}
          description={t("adminCommon.noResultsBody")}
          action={
            <Button variant="outline" size="sm" onClick={() => onSearchChange("")}>
              {t("adminCommon.clearFilters")}
            </Button>
          }
        />
      )}
    </div>
  );
}

function ModelStatusRow({ model }: { model: AvailableModelSummary }) {
  const { t } = useLanguage();
  const [expanded, setExpanded] = useState(false);

  return (
    <Card className="overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-srapi-card-muted/50"
      >
        <StatusDot status={model.status} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium text-srapi-text-primary">{model.name}</div>
          <div className="truncate text-[11px] text-srapi-text-tertiary tabular">{model.id}</div>
        </div>

        {/* Mini bar chart — one segment per channel */}
        <div className="hidden items-center gap-[2px] sm:flex">
          {model.channels.map((ch, i) => (
            <div
              key={i}
              className={cn(
                "h-6 w-1.5 rounded-full",
                ch.status === "available" && "bg-emerald-500",
                ch.status === "limited" && "bg-amber-400",
                ch.status === "unavailable" && "bg-red-400",
              )}
              title={`${ch.provider_display_name}: ${ch.status}`}
            />
          ))}
        </div>

        <span className={cn(
          "shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium",
          model.status === "available" && "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
          model.status === "limited" && "bg-amber-500/10 text-amber-700 dark:text-amber-400",
          model.status === "unavailable" && "bg-red-500/10 text-red-700 dark:text-red-400",
        )}>
          {t(`availableChannels.${model.status}`)}
        </span>
        <ChevronDown className={cn("size-4 text-srapi-text-tertiary transition-transform", expanded && "rotate-180")} />
      </button>

      {expanded && (
        <div className="border-t border-srapi-border bg-srapi-card-muted/30 px-4 py-2">
          <div className="space-y-1.5">
            {model.channels.map((ch, i) => (
              <div key={i} className="flex items-center gap-3 rounded-lg px-2 py-1.5 text-xs">
                <StatusDot status={ch.status} size="sm" />
                <span className="min-w-0 flex-1 truncate font-medium text-srapi-text-primary">
                  {ch.provider_display_name}
                </span>
                <span className={cn(
                  "shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium",
                  ch.status === "available" && "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
                  ch.status === "limited" && "bg-amber-500/10 text-amber-700 dark:text-amber-400",
                  ch.status === "unavailable" && "bg-red-500/10 text-red-700 dark:text-red-400",
                )}>
                  {t(`availableChannels.${ch.status}`)}
                </span>
                <PricingChip channel={ch} />
              </div>
            ))}
          </div>
        </div>
      )}
    </Card>
  );
}

function StatusDot({ status, size = "md" }: { status: string; size?: "sm" | "md" }) {
  const s = size === "sm" ? "size-2" : "size-2.5";
  return (
    <span className={cn(
      "shrink-0 rounded-full",
      s,
      status === "available" && "bg-emerald-500",
      status === "limited" && "bg-amber-400",
      status === "unavailable" && "bg-red-400",
    )} />
  );
}

function PricingChip({ channel }: { channel: AvailableModelChannelSummary }) {
  const pricing = channel.pricing;
  if (pricing.billing_mode === "per_request" || pricing.billing_mode === "image") {
    return (
      <span className="shrink-0 rounded bg-srapi-card-muted px-1.5 py-0.5 text-[10px] tabular text-srapi-text-tertiary">
        {formatMoney(pricing.per_request_price, pricing.currency)}/req
      </span>
    );
  }
  return (
    <span className="shrink-0 rounded bg-srapi-card-muted px-1.5 py-0.5 text-[10px] tabular text-srapi-text-tertiary">
      {formatMoney(pricing.input_price_per_million_tokens, pricing.currency)}/{formatMoney(pricing.output_price_per_million_tokens, pricing.currency)} /M
    </span>
  );
}
