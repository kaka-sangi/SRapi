"use client";

import { useState, useMemo } from "react";
import { Search } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { Radio, SearchX } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableScroll,
} from "@/components/ui/table";
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
      <PageHeader
        eyebrow={t("nav.sectionWorkspace")}
        title={t("availableChannels.title")}
        description={t("availableChannels.subtitle")}
      />
      <PageQueryState query={availableModels} skeleton={<DialogListSkeleton rows={6} />}>
        {(models) =>
          models.length === 0 ? (
            <EmptyState
              icon={Radio}
              title={t("availableChannels.emptyTitle")}
              description={t("availableChannels.emptyBody")}
            />
          ) : (
            <AvailableModelsTable models={models} search={search} onSearchChange={setSearch} />
          )
        }
      </PageQueryState>
    </>
  );
}

function AvailableModelsTable({
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
        m.channels.some(
          (c) =>
            c.provider_display_name.toLowerCase().includes(q) ||
            c.upstream_model.toLowerCase().includes(q),
        ),
    );
  }, [models, search]);
  const rows = filtered.flatMap((model) => model.channels.map((channel) => ({ model, channel })));

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-1 items-center gap-3">
          <div className="relative w-full sm:w-56">
            <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-srapi-text-tertiary" />
            <Input
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={t("common.search")}
              className="pl-8 text-xs"
            />
          </div>
          <span className="ml-auto text-[12px] font-medium text-srapi-text-tertiary tabular">
            {rows.length} / {models.flatMap((m) => m.channels).length}
          </span>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {rows.length === 0 ? (
          <EmptyState
            icon={SearchX}
            title={t("adminCommon.noResults")}
            description={t("adminCommon.noResultsBody")}
            action={
              <Button variant="outline" size="sm" onClick={() => onSearchChange("")}>
                {t("adminCommon.clearFilters")}
              </Button>
            }
          />
        ) : (
        <TableScroll minWidth={720}>
          <Table>
            <TableHeader>
              <tr>
                <TableHead>{t("availableChannels.model")}</TableHead>
                <TableHead>{t("availableChannels.channel")}</TableHead>
                <TableHead>{t("availableChannels.status")}</TableHead>
                <TableHead>{t("availableChannels.pricing")}</TableHead>
                <TableHead className="text-right">{t("availableChannels.accounts")}</TableHead>
              </tr>
            </TableHeader>
            <TableBody>
              {rows.map(({ model, channel }) => (
                <TableRow key={`${model.id}:${channel.provider_id}:${channel.upstream_model}`}>
                  <TableCell className="max-w-[180px]">
                    <div className="truncate font-medium text-srapi-text-primary" title={model.name}>{model.name}</div>
                    <div className="mt-1 truncate text-[12px] text-srapi-text-tertiary tabular" title={model.id}>{model.id}</div>
                  </TableCell>
                  <TableCell className="max-w-[200px]">
                    <div className="truncate text-srapi-text-primary" title={channel.provider_display_name}>{channel.provider_display_name}</div>
                    <div className="mt-1 truncate text-[12px] text-srapi-text-tertiary tabular" title={`${channel.protocol} · ${channel.upstream_model}`}>
                      {channel.protocol} · {channel.upstream_model}
                    </div>
                  </TableCell>
                  <TableCell>
                    <QuietBadge status={statusTone(channel.status)} label={t(`availableChannels.${channel.status}`)} />
                  </TableCell>
                  <TableCell>
                    <PricingText channel={channel} />
                  </TableCell>
                  <TableCell className="text-right text-sm font-medium tabular text-srapi-text-secondary">
                    {channel.active_account_count}/{channel.total_account_count}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableScroll>
        )}
      </CardContent>
    </Card>
  );
}

function PricingText({ channel }: { channel: AvailableModelChannelSummary }) {
  const { t } = useLanguage();
  const pricing = channel.pricing;
  if (pricing.billing_mode === "per_request" || pricing.billing_mode === "image") {
    return (
      <div className="text-xs text-srapi-text-secondary tabular">
        <span className="font-medium">{formatMoney(pricing.per_request_price, pricing.currency)}</span>
        <span className="ml-1 text-srapi-text-tertiary">{t(`availableChannels.${pricing.billing_mode}`)}</span>
      </div>
    );
  }
  return (
    <div className="space-y-1 text-xs text-srapi-text-secondary tabular">
      <div>{t("availableChannels.inputPrice", { price: formatMoney(pricing.input_price_per_million_tokens, pricing.currency) })}</div>
      <div>{t("availableChannels.outputPrice", { price: formatMoney(pricing.output_price_per_million_tokens, pricing.currency) })}</div>
    </div>
  );
}

function statusTone(status: AvailableModelSummary["status"]): QuietStatus {
  if (status === "available") return "active";
  if (status === "limited") return "limited";
  return "disabled";
}
