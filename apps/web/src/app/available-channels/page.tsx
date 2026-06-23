"use client";

import { useState, useMemo } from "react";
import { Search, CheckCircle2, AlertTriangle, XCircle } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { SectionHero } from "@/components/visual/section-hero";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
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
        eyebrow="Workspace · Status"
        title={t("availableChannels.title")}
        description={t("availableChannels.subtitle")}
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

type OverallStatus = "operational" | "degraded" | "outage";

function overallStatus(models: AvailableModelSummary[]): OverallStatus {
  if (models.length === 0) return "operational";
  const unavail = models.filter((m) => m.status === "unavailable").length;
  const limited = models.filter((m) => m.status === "limited").length;
  if (unavail === models.length) return "outage";
  if (unavail > 0 || limited > 0) return "degraded";
  return "operational";
}

const STATUS_STYLE: Record<OverallStatus, { border: string; bg: string; icon: typeof CheckCircle2; iconColor: string }> = {
  operational: { border: "border-emerald-500", bg: "bg-emerald-500", icon: CheckCircle2, iconColor: "text-emerald-500" },
  degraded:    { border: "border-amber-500",   bg: "bg-amber-500",   icon: AlertTriangle, iconColor: "text-amber-500" },
  outage:      { border: "border-red-500",     bg: "bg-red-500",     icon: XCircle,       iconColor: "text-red-500" },
};

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
  const style = STATUS_STYLE[status];
  const Icon = style.icon;
  const availableCount = models.filter((m) => m.status === "available").length;

  return (
    <div className="space-y-6">
      {/* Overall status banner — like status.claude.com hero */}
      <div className={cn("rounded-xl border-l-4 bg-srapi-card p-5", style.border)}>
        <div className="flex items-center gap-4">
          <Icon className={cn("size-7 shrink-0", style.iconColor)} />
          <div className="flex-1">
            <div className="text-lg font-semibold tracking-tight text-srapi-text-primary">
              {t(`availableChannels.overall_${status}`)}
            </div>
            <div className="mt-0.5 text-sm text-srapi-text-tertiary">
              {t("availableChannels.overallSub", { available: availableCount, total: models.length })}
            </div>
          </div>
          <div className="relative w-52">
            <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-srapi-text-tertiary" />
            <Input
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={t("common.search")}
              className="pl-8 text-xs"
            />
          </div>
        </div>
      </div>

      {/* Service rows — one per model, status.claude.com style */}
      <div className="overflow-hidden rounded-xl border border-srapi-border bg-srapi-card">
        {filtered.map((model, idx) => (
          <ModelStatusRow key={model.id} model={model} isLast={idx === filtered.length - 1} />
        ))}
        {filtered.length === 0 && search && (
          <div className="p-8">
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
          </div>
        )}
      </div>
    </div>
  );
}

function ModelStatusRow({ model, isLast }: { model: AvailableModelSummary; isLast: boolean }) {
  const { t } = useLanguage();
  const [hovered, setHovered] = useState<number | null>(null);

  const channels = model.channels;
  const statusLabel = t(`availableChannels.${model.status}`);

  return (
    <div className={cn("px-5 py-4", !isLast && "border-b border-srapi-border")}>
      {/* Top line: name + status badge */}
      <div className="flex items-center justify-between gap-4">
        <div className="min-w-0">
          <span className="text-sm font-medium text-srapi-text-primary">{model.name}</span>
        </div>
        <span className={cn(
          "shrink-0 text-xs font-medium",
          model.status === "available" && "text-emerald-600 dark:text-emerald-400",
          model.status === "limited" && "text-amber-600 dark:text-amber-400",
          model.status === "unavailable" && "text-red-600 dark:text-red-400",
        )}>
          {statusLabel}
        </span>
      </div>

      {/* Uptime bar — one segment per channel, like days on status.claude.com */}
      {channels.length > 0 && (
        <div className="mt-2.5">
          <div
            className="flex h-8 w-full gap-px overflow-hidden rounded"
            onMouseLeave={() => setHovered(null)}
          >
            {channels.map((ch, i) => (
              <div
                key={i}
                className={cn(
                  "flex-1 cursor-default rounded-sm transition-opacity",
                  ch.status === "available" && "bg-emerald-500",
                  ch.status === "limited" && "bg-amber-400",
                  ch.status === "unavailable" && "bg-red-400",
                  hovered !== null && hovered !== i && "opacity-40",
                )}
                onMouseEnter={() => setHovered(i)}
              />
            ))}
          </div>

          {/* Hover tooltip */}
          {hovered !== null && channels[hovered] && (
            <div className="mt-1.5 flex items-center gap-2 text-[11px] text-srapi-text-secondary">
              <StatusDot status={channels[hovered].status} />
              <span className="font-medium">{channels[hovered].provider_display_name}</span>
              <span className="text-srapi-text-tertiary">·</span>
              <span className={cn(
                "font-medium",
                channels[hovered].status === "available" && "text-emerald-600 dark:text-emerald-400",
                channels[hovered].status === "limited" && "text-amber-600 dark:text-amber-400",
                channels[hovered].status === "unavailable" && "text-red-600 dark:text-red-400",
              )}>
                {t(`availableChannels.${channels[hovered].status}`)}
              </span>
              <span className="text-srapi-text-tertiary">·</span>
              <PricingInline channel={channels[hovered]} />
            </div>
          )}

          {/* Legend line when not hovering */}
          {hovered === null && (
            <div className="mt-1 flex items-center justify-between text-[10px] text-srapi-text-tertiary">
              <span>{channels.length} {channels.length === 1 ? "channel" : "channels"}</span>
              <PricingRange channels={channels} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function StatusDot({ status }: { status: string }) {
  return (
    <span className={cn(
      "inline-block size-2 shrink-0 rounded-full",
      status === "available" && "bg-emerald-500",
      status === "limited" && "bg-amber-400",
      status === "unavailable" && "bg-red-400",
    )} />
  );
}

function PricingInline({ channel }: { channel: AvailableModelChannelSummary }) {
  const p = channel.pricing;
  if (p.billing_mode === "per_request" || p.billing_mode === "image") {
    return <span className="tabular">{formatMoney(p.per_request_price, p.currency)}/req</span>;
  }
  return (
    <span className="tabular">
      {formatMoney(p.input_price_per_million_tokens, p.currency)} / {formatMoney(p.output_price_per_million_tokens, p.currency)} per M
    </span>
  );
}

function PricingRange({ channels }: { channels: AvailableModelChannelSummary[] }) {
  const tokenChannels = channels.filter((c) => c.pricing.billing_mode === "token");
  if (tokenChannels.length === 0) return null;
  const inputs = tokenChannels.map((c) => parseFloat(c.pricing.input_price_per_million_tokens || "0"));
  const minInput = Math.min(...inputs);
  const maxInput = Math.max(...inputs);
  const currency = tokenChannels[0].pricing.currency;
  if (minInput === maxInput) {
    return <span className="tabular">{formatMoney(String(minInput), currency)} /M in</span>;
  }
  return (
    <span className="tabular">
      {formatMoney(String(minInput), currency)} – {formatMoney(String(maxInput), currency)} /M in
    </span>
  );
}
