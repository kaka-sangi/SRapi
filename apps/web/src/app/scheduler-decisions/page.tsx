"use client";

import { useState } from "react";
import { GitBranch } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useSchedulerDecisions } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { SchedulerDecisionStream } from "@/components/ui/scheduler-decision-stream";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { decisionToLines } from "@/lib/format-decision";
import type { SchedulerDecisionSummary } from "@/lib/srapi-types";

export default function SchedulerDecisionsPage() {
  return (
    <AppShell allowedRole="admin">
      <SchedulerContent />
    </AppShell>
  );
}

function SchedulerContent() {
  const { t } = useLanguage();
  const decisions = useSchedulerDecisions();
  const [selectedId, setSelectedId] = useState<string | null>(null);

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionGateway")}
        title={t("scheduler.title")}
        actions={
          <Button variant="outline" size="sm" onClick={() => decisions.refetch()}>
            {t("common.refresh")}
          </Button>
        }
      />

      <PageQueryState
        query={decisions}
        isEmpty={(d) => d.length === 0}
        skeleton={<Skeleton className="h-96 rounded-xl" />}
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
              selected={data.find((d) => d.request_id === selectedId) ?? data[0]}
              onSelect={setSelectedId}
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
}: {
  decisions: SchedulerDecisionSummary[];
  selected: SchedulerDecisionSummary;
  onSelect: (id: string) => void;
}) {
  const { t } = useLanguage();

  return (
    <div className="grid gap-6 lg:grid-cols-12">
      {/* Left: decision list */}
      <div className="lg:col-span-5">
        <Card>
          <CardHeader>
            <CardTitle>{t("scheduler.title")}</CardTitle>
            <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
              {decisions.length}
            </span>
          </CardHeader>
          <div className="max-h-[640px] divide-y divide-srapi-border overflow-y-auto">
            {decisions.map((d) => {
              const active = d.request_id === selected.request_id;
              return (
                <button
                  key={d.request_id}
                  onClick={() => onSelect(d.request_id)}
                  className={`flex w-full items-center gap-3 px-5 py-3 text-left transition-colors ${
                    active ? "bg-srapi-card-muted" : "hover:bg-srapi-card-muted/50"
                  }`}
                >
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm text-srapi-text-primary">{d.model}</div>
                    <div className="truncate font-mono text-2xs text-srapi-text-secondary">
                      {d.request_id} · {d.created_at.replace("T", " ").slice(0, 16)}
                    </div>
                  </div>
                  <QuietBadge status="active" label={d.selected_account_name} />
                </button>
              );
            })}
          </div>
        </Card>
      </div>

      {/* Right: trace stream (sticky on desktop) */}
      <div className="lg:col-span-7">
        <Card className="lg:sticky lg:top-24">
          <CardHeader>
            <CardTitle>{t("scheduler.traceLog")}</CardTitle>
            <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
              {selected.request_id}
            </span>
          </CardHeader>
          <CardContent>
            <SchedulerDecisionStream key={selected.request_id} lines={decisionToLines(selected)} />
            {selected.warnings.length > 0 && (
              <div className="mt-4 space-y-1 border-t border-srapi-border pt-4">
                {selected.warnings.map((w, i) => (
                  <div key={i} className="font-mono text-2xs text-srapi-warning">
                    ⚠ {w}
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
