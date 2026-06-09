"use client";

import { ListChecks } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
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
              icon={ListChecks}
              title={t("availableChannels.emptyTitle")}
              description={t("availableChannels.emptyBody")}
            />
          ) : (
            <AvailableModelsTable models={models} />
          )
        }
      </PageQueryState>
    </>
  );
}

function AvailableModelsTable({ models }: { models: AvailableModelSummary[] }) {
  const { t } = useLanguage();
  const rows = models.flatMap((model) => model.channels.map((channel) => ({ model, channel })));

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("availableChannels.models")}</CardTitle>
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {t("availableChannels.modelCount", { count: models.length })}
        </span>
      </CardHeader>
      <CardContent className="p-0">
        <TableScroll minWidth={920}>
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
                  <TableCell>
                    <div className="font-medium text-srapi-text-primary">{model.name}</div>
                    <div className="mt-1 font-mono text-2xs text-srapi-text-tertiary">{model.id}</div>
                  </TableCell>
                  <TableCell>
                    <div className="text-srapi-text-primary">{channel.provider_display_name}</div>
                    <div className="mt-1 font-mono text-2xs text-srapi-text-tertiary">
                      {channel.protocol} · {channel.upstream_model}
                    </div>
                  </TableCell>
                  <TableCell>
                    <QuietBadge status={statusTone(channel.status)} label={t(`availableChannels.${channel.status}`)} />
                  </TableCell>
                  <TableCell>
                    <PricingText channel={channel} />
                  </TableCell>
                  <TableCell className="text-right font-mono text-sm tabular text-srapi-text-secondary">
                    {channel.active_account_count}/{channel.total_account_count}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableScroll>
      </CardContent>
    </Card>
  );
}

function PricingText({ channel }: { channel: AvailableModelChannelSummary }) {
  const { t } = useLanguage();
  const pricing = channel.pricing;
  if (pricing.billing_mode === "per_request" || pricing.billing_mode === "image") {
    return (
      <div className="font-mono text-xs text-srapi-text-secondary">
        {formatMoney(pricing.per_request_price, pricing.currency)}
        <span className="ml-1 text-srapi-text-tertiary">{t(`availableChannels.${pricing.billing_mode}`)}</span>
      </div>
    );
  }
  return (
    <div className="space-y-1 font-mono text-xs text-srapi-text-secondary">
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
