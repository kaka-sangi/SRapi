"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ExternalLink, GitBranch } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useSchedulerDecisions } from "@/hooks/queries";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { SchedulerDecisionStream } from "@/components/ui/scheduler-decision-stream";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { decisionToLines } from "@/lib/format-decision";
import { formatDateTime, formatMoney } from "@/lib/admin-format";
import {
  adminAccountsHealthHref,
  adminErrorInvestigationHref,
  adminRequestEvidenceHref,
  adminProvidersHref,
} from "@/lib/admin-log-links";
import type { SchedulerDecisionSummary } from "@/lib/srapi-types";

export function SchedulerDecisionsPanel() {
  const { t } = useLanguage();
  const searchParams = useSearchParams();
  const requestIDFilter = searchParams?.get("f_request_id")?.trim() || undefined;
  const accountIDFilter = searchParams?.get("f_account_id")?.trim() || undefined;
  const providerIDFilter = searchParams?.get("f_provider_id")?.trim() || undefined;
  const modelFilter = searchParams?.get("f_model")?.trim() || undefined;
  const decisions = useSchedulerDecisions({
    request_id: requestIDFilter,
    account_id: accountIDFilter,
    provider_id: providerIDFilter,
    model: modelFilter,
  });
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdminOps")}
        title={t("scheduler.title")}
        actions={
          <AutoRefreshControl
            onRefresh={() => void decisions.refetch()}
            isRefreshing={decisions.isFetching}
            storageKey="srapi.autorefresh.scheduler-decisions"
          />
        }
      />

      <PageQueryState
        query={decisions}
        isEmpty={(d) => d.length === 0}
        skeleton={<DialogListSkeleton rows={10} />}
      >
        {(data) =>
          data.length === 0 ? (
            <EmptyState
              icon={GitBranch}
              title={t("scheduler.emptyTitle")}
              description={t("scheduler.emptyBody")}
            />
          ) : (
            <SchedulerBody
              decisions={data}
              selected={data.find((d) => decisionKey(d) === selectedId) ?? data[0]}
              onSelect={setSelectedId}
              filters={{
                requestID: requestIDFilter,
                accountID: accountIDFilter,
                providerID: providerIDFilter,
                model: modelFilter,
              }}
            />
          )
        }
      </PageQueryState>
    </>
  );
}

function SchedulerBody({
  decisions,
  selected,
  onSelect,
  filters,
}: {
  decisions: SchedulerDecisionSummary[];
  selected: SchedulerDecisionSummary;
  onSelect: (id: string) => void;
  filters: SchedulerDecisionPanelFilters;
}) {
  const { t } = useLanguage();
  const accountLookup = useAccountNameLookup();
  const providerLookup = useProviderNameLookup();
  const selectedOutcome = decisionOutcome(selected);

  return (
    <div className="grid gap-6 lg:grid-cols-12">
      <div className="lg:col-span-5">
        <Card>
          <CardHeader>
            <CardTitle>{t("scheduler.title")}</CardTitle>
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <SchedulerFilterChips filters={filters} />
              <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
                {decisions.length}
              </span>
            </div>
          </CardHeader>
          <div className="max-h-[640px] divide-y divide-srapi-border overflow-y-auto">
            {decisions.map((d) => {
              const key = decisionKey(d);
              const active = key === decisionKey(selected);
              const selectedAccount = selectedAccountLabel(d, accountLookup.get, t("scheduler.noAccount"));
              const primaryReject = primaryRejectReason(d);
              const outcome = decisionOutcome(d);
              return (
                <button
                  key={key}
                  onClick={() => onSelect(key)}
                  className={`grid w-full gap-2 px-5 py-3 text-left transition-colors ${
                    active ? "bg-srapi-card-muted" : "hover:bg-srapi-card-muted/50"
                  }`}
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm text-srapi-text-primary">{d.model}</div>
                      <div className="truncate font-mono text-2xs text-srapi-text-secondary">
                        {d.request_id} · #{d.attempt_no} · {formatDateTime(d.created_at)}
                      </div>
                    </div>
                    <QuietBadge status={outcome.status} label={t(outcome.labelKey)} />
                  </div>
                  <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                    <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
                      {d.strategy}
                    </span>
                    <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
                      {d.candidate_count}/{d.rejected_count}
                    </span>
                    {d.fallback_from_decision_id ? (
                      <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-warning">
                        {t("scheduler.fallback")}
                      </span>
                    ) : null}
                    <span className="max-w-44 truncate font-mono text-2xs text-srapi-text-secondary">
                      {selectedAccount}
                    </span>
                    {primaryReject ? (
                      <span className="max-w-40 truncate font-mono text-2xs text-srapi-error">
                        {primaryReject.reason} ×{primaryReject.count}
                      </span>
                    ) : null}
                  </div>
                </button>
              );
            })}
          </div>
        </Card>
      </div>

      <div className="lg:col-span-7">
        <Card className="lg:sticky lg:top-24">
          <CardHeader>
            <div className="min-w-0">
              <CardTitle>{t("scheduler.traceLog")}</CardTitle>
              <div className="mt-1 truncate font-mono text-2xs text-srapi-text-tertiary tabular">
                {selected.request_id} · #{selected.attempt_no}
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <QuietBadge
                status={selectedOutcome.status}
                label={t(selectedOutcome.labelKey)}
              />
              {selected.sticky_hit ? <QuietBadge status="active" label={t("scheduler.stickyHit")} /> : null}
              {selected.cache_affinity_hit ? (
                <QuietBadge status="active" label={t("scheduler.cacheHit")} />
              ) : null}
            </div>
          </CardHeader>
          <CardContent>
            <DecisionInvestigationSummary
              decision={selected}
              accountName={selectedAccountLabel(selected, accountLookup.get, t("scheduler.noAccount"))}
              providerName={providerLookup.get(selected.selected_provider_id)}
            />
            <SchedulerDecisionStream key={decisionKey(selected)} lines={decisionToLines(selected)} />
            {selected.warnings.length > 0 && (
              <div className="mt-4 space-y-1 border-t border-srapi-border pt-4">
                {selected.warnings.map((w, i) => (
                  <div key={i} className="font-mono text-2xs text-srapi-warning">
                    ! {w}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

interface SchedulerDecisionPanelFilters {
  requestID?: string;
  accountID?: string;
  providerID?: string;
  model?: string;
}

function SchedulerFilterChips({ filters }: { filters: SchedulerDecisionPanelFilters }) {
  const entries = [
    filters.requestID ? ["req", filters.requestID] : null,
    filters.accountID ? ["acct", filters.accountID] : null,
    filters.providerID ? ["prov", filters.providerID] : null,
    filters.model ? ["model", filters.model] : null,
  ].filter((entry): entry is [string, string] => Boolean(entry));
  return entries.map(([label, value]) => (
    <span
      key={`${label}:${value}`}
      className="max-w-56 truncate rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary"
      title={`${label}:${value}`}
    >
      {label}:{value}
    </span>
  ));
}

function DecisionInvestigationSummary({
  decision,
  accountName,
  providerName,
}: {
  decision: SchedulerDecisionSummary;
  accountName: string;
  providerName: string;
}) {
  const { t } = useLanguage();
  const errorHref = adminErrorInvestigationHref({
    account_id: decision.selected_account_id,
    provider_id: decision.selected_provider_id,
    model: decision.model,
    source_endpoint: decision.source_endpoint,
  });
  const requestEvidenceHref = adminRequestEvidenceHref({ request_id: decision.request_id });
  const accountHealthHref = adminAccountsHealthHref({
    account_id: decision.selected_account_id,
    provider_id: decision.selected_provider_id,
  });
  const providerHref = adminProvidersHref(decision.selected_provider_id);

  return (
    <div className="mb-5 grid gap-4 border-b border-srapi-border pb-5">
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <DecisionStat label={t("scheduler.strategy")} value={strategyLabel(decision)} />
        <DecisionStat label={t("scheduler.selectedAccount")} value={accountName} tone={decision.selected_account_id ? "normal" : "error"} />
        <DecisionStat label={t("scheduler.selectedProvider")} value={providerName} />
        <DecisionStat label={t("scheduler.estimatedCost")} value={formatMoney(decision.estimated_cost, decision.currency)} />
      </div>
      <div className="grid gap-2 sm:grid-cols-3">
        <DecisionStat label={t("scheduler.protocol")} value={`${decision.source_protocol} -> ${decision.target_protocol || "-"}`} />
        <DecisionStat label={t("scheduler.candidateSummary")} value={`${decision.candidate_count} / ${decision.rejected_count}`} />
        <DecisionStat
          label={t("scheduler.primaryReject")}
          value={primaryRejectReason(decision)?.reason ?? t("scheduler.none")}
          tone={primaryRejectReason(decision) ? "warning" : "muted"}
        />
      </div>
      {decision.selection_rationale ? (
        <div className="rounded-md border border-srapi-border bg-srapi-card-muted px-3 py-2 text-xs text-srapi-text-secondary">
          <div className="mb-1 font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("scheduler.rationale")}
          </div>
          {decision.selection_rationale}
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {errorHref ? (
          <Button asChild variant="outline" size="sm">
            <Link href={errorHref}>
              {t("scheduler.errorLogs")}
              <ExternalLink aria-hidden />
            </Link>
          </Button>
        ) : null}
        {requestEvidenceHref ? (
          <Button asChild variant="ghost" size="sm">
            <Link href={requestEvidenceHref}>
              {t("scheduler.requestEvidence")}
              <ExternalLink aria-hidden />
            </Link>
          </Button>
        ) : null}
        <Button asChild variant="ghost" size="sm">
          <Link href={accountHealthHref}>
            {t("scheduler.accountHealth")}
            <ExternalLink aria-hidden />
          </Link>
        </Button>
        <Button asChild variant="ghost" size="sm">
          <Link href={providerHref}>
            {t("scheduler.provider")}
            <ExternalLink aria-hidden />
          </Link>
        </Button>
      </div>
      <ScoreBreakdown scores={decision.scores} />
      <RejectReasons reasons={decision.rejected_reasons} />
    </div>
  );
}

function DecisionStat({
  label,
  value,
  tone = "normal",
}: {
  label: string;
  value: string;
  tone?: "normal" | "muted" | "warning" | "error";
}) {
  const toneClass =
    tone === "error"
      ? "text-srapi-error"
      : tone === "warning"
        ? "text-srapi-warning"
        : tone === "muted"
          ? "text-srapi-text-tertiary"
          : "text-srapi-text-primary";
  return (
    <div className="min-w-0 rounded-md border border-srapi-border px-3 py-2">
      <div className="mb-1 font-mono text-2xs uppercase text-srapi-text-tertiary">{label}</div>
      <div className={`truncate text-sm ${toneClass}`} title={value}>
        {value || "-"}
      </div>
    </div>
  );
}

function ScoreBreakdown({ scores }: { scores: SchedulerDecisionSummary["scores"] }) {
  const { t } = useLanguage();
  return (
    <div className="min-w-0 overflow-hidden rounded-md border border-srapi-border">
      <div className="flex items-center justify-between border-b border-srapi-border px-3 py-2">
        <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
          {t("scheduler.scoreBreakdown")}
        </div>
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">{scores.length}</span>
      </div>
      {scores.length === 0 ? (
        <div className="px-3 py-3 text-xs text-srapi-text-tertiary">{t("scheduler.noScores")}</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[680px] text-left text-xs">
            <thead className="bg-srapi-card-muted font-mono text-2xs uppercase text-srapi-text-tertiary">
              <tr>
                <th className="px-3 py-2 font-medium">{t("scheduler.account")}</th>
                <th className="px-2 py-2 text-right font-medium">{t("scheduler.finalScore")}</th>
                <th className="px-2 py-2 text-right font-medium">health</th>
                <th className="px-2 py-2 text-right font-medium">quota</th>
                <th className="px-2 py-2 text-right font-medium">latency</th>
                <th className="px-2 py-2 text-right font-medium">cost</th>
                <th className="px-2 py-2 text-right font-medium">quality</th>
                <th className="px-2 py-2 text-right font-medium">risk</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-srapi-border">
              {scores.slice(0, 8).map((score) => (
                <tr key={`${score.account}-${score.score}`}>
                  <td className="max-w-44 truncate px-3 py-2 font-mono text-2xs text-srapi-text-secondary">
                    {score.account}
                    {score.pareto_frontier ? (
                      <span className="ml-2 rounded bg-srapi-card-muted px-1.5 py-0.5 text-srapi-success">
                        frontier
                      </span>
                    ) : null}
                  </td>
                  <ScoreCell value={score.score} strong />
                  <ScoreCell value={score.health} />
                  <ScoreCell value={score.quota} />
                  <ScoreCell value={score.latency} />
                  <ScoreCell value={score.cost} />
                  <ScoreCell value={score.quality} />
                  <ScoreCell value={score.risk_penalty + score.saturation_penalty} danger />
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function ScoreCell({ value, strong = false, danger = false }: { value: number; strong?: boolean; danger?: boolean }) {
  return (
    <td
      className={`px-2 py-2 text-right font-mono text-2xs tabular ${
        danger && value > 0
          ? "text-srapi-error"
          : strong
            ? "text-srapi-text-primary"
            : "text-srapi-text-tertiary"
      }`}
    >
      {value.toFixed(2)}
    </td>
  );
}

function RejectReasons({ reasons }: { reasons: SchedulerDecisionSummary["rejected_reasons"] }) {
  const { t } = useLanguage();
  return (
    <div className="rounded-md border border-srapi-border px-3 py-2">
      <div className="mb-2 flex items-center justify-between">
        <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
          {t("scheduler.rejectReasons")}
        </div>
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">{reasons.length}</span>
      </div>
      {reasons.length === 0 ? (
        <div className="text-xs text-srapi-text-tertiary">{t("scheduler.noRejects")}</div>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {reasons.slice(0, 12).map((reason, index) => (
            <span
              key={`${reason.account}-${reason.reason}-${index}`}
              className="inline-flex min-w-0 items-center gap-1.5 rounded-md border border-srapi-border bg-srapi-card-muted px-2 py-1 font-mono text-2xs"
            >
              <span className="max-w-28 truncate text-srapi-text-secondary">{reason.account}</span>
              <span className="max-w-44 truncate text-srapi-error">{reason.reason}</span>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function decisionKey(decision: SchedulerDecisionSummary): string {
  return decision.id || `${decision.request_id}:${decision.attempt_no}`;
}

function selectedAccountLabel(
  decision: SchedulerDecisionSummary,
  lookup: (id: string | number | null | undefined) => string,
  emptyLabel: string,
): string {
  return decision.selected_account_id ? lookup(decision.selected_account_id) : emptyLabel;
}

function strategyLabel(decision: SchedulerDecisionSummary): string {
  return decision.strategy_version ? `${decision.strategy}@${decision.strategy_version}` : decision.strategy;
}

function decisionOutcome(decision: SchedulerDecisionSummary): { status: QuietStatus; labelKey: string } {
  if (decision.selected_account_id) {
    return {
      status: decision.fallback_from_decision_id ? "limited" : "active",
      labelKey: decision.fallback_from_decision_id ? "scheduler.fallback" : "scheduler.selected",
    };
  }
  return { status: "error", labelKey: "scheduler.noAccount" };
}

function primaryRejectReason(decision: SchedulerDecisionSummary): { reason: string; count: number } | null {
  const counts = new Map<string, number>();
  for (const item of decision.rejected_reasons) {
    if (!item.reason) continue;
    counts.set(item.reason, (counts.get(item.reason) ?? 0) + 1);
  }
  let best: { reason: string; count: number } | null = null;
  for (const [reason, count] of counts) {
    if (!best || count > best.count || (count === best.count && reason < best.reason)) {
      best = { reason, count };
    }
  }
  return best;
}
