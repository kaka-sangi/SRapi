"use client";

import { type FormEvent, useMemo, useState } from "react";
import { Activity, BarChart3, GitCompareArrows, RefreshCw, Route, Target } from "lucide-react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { AdminShell } from "@/components/admin/admin-shell";
import {
  AdminBarList,
  AdminEmptyState,
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminSection,
  AdminStatCard,
  AdminTable,
} from "@/components/admin/admin-primitives";
import { Button, Input, Label } from "@/components/ui";
import { useAdminSchedulerReplay } from "@/hooks/admin-queries";
import type {
  SchedulerReplayItem,
  SchedulerReplayRequest,
  SchedulerReplayResult,
  SchedulerStrategyName,
} from "../../../../../packages/sdk/typescript/src/types.gen";

const STRATEGY_OPTIONS: SchedulerStrategyName[] = [
  "balanced",
  "cost_saver",
  "latency_first",
  "quota_protect",
  "sticky_first",
  "cache_affinity_first",
  "premium_quality",
];

type StrategyFormState = {
  currentStrategy: "original" | SchedulerStrategyName;
  shadowStrategy: SchedulerStrategyName;
  limit: string;
  shadowRolloutPercent: string;
  since: string;
  until: string;
  model: string;
  requestId: string;
};

type ReplayChartPoint = {
  label: string;
  current: number | null;
  shadow: number | null;
};

function emptyFormState(): StrategyFormState {
  return {
    currentStrategy: "original",
    shadowStrategy: "cost_saver",
    limit: "100",
    shadowRolloutPercent: "100",
    since: "",
    until: "",
    model: "",
    requestId: "",
  };
}

function strategyLabel(value?: string | null): string {
  if (!value) {
    return "original";
  }
  return value
    .split("_")
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatInteger(value: number): string {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 0 }).format(value);
}

function formatScore(value?: number | null): string {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "n/a";
  }
  return value.toFixed(3);
}

function formatPercentRatio(numerator: number, denominator: number): string {
  if (denominator <= 0) {
    return "0.0%";
  }
  return `${((numerator / denominator) * 100).toFixed(1)}%`;
}

function clampInteger(raw: string, fallback: number, min: number, max: number): number {
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed)) {
    return fallback;
  }
  return Math.max(min, Math.min(max, parsed));
}

function optionalTrimmed(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed ? trimmed : undefined;
}

function optionalLocalDateTime(value: string): string | undefined {
  if (!value) {
    return undefined;
  }
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
}

function scoreFromDecision(item: SchedulerReplayItem, side: "current" | "shadow"): number | null {
  const finalScore = item[side].scores.final;
  return typeof finalScore === "number" && Number.isFinite(finalScore) ? finalScore : null;
}

function selectedAccountLabel(value?: string | null): string {
  return value || "none";
}

function buildReplayRequest(form: StrategyFormState): SchedulerReplayRequest {
  const rollout = form.shadowRolloutPercent.trim();
  const rolloutPercent = rollout === "" ? undefined : clampInteger(rollout, 100, 0, 100);

  return {
    current_strategy: form.currentStrategy === "original" ? undefined : form.currentStrategy,
    shadow_strategy: form.shadowStrategy,
    limit: clampInteger(form.limit, 100, 1, 500),
    shadow_rollout_percent: rolloutPercent,
    since: optionalLocalDateTime(form.since),
    until: optionalLocalDateTime(form.until),
    model: optionalTrimmed(form.model),
    request_id: optionalTrimmed(form.requestId),
  };
}

function chartPoints(result?: SchedulerReplayResult): ReplayChartPoint[] {
  if (!result) {
    return [];
  }

  return result.items.slice(0, 50).map((item, index) => ({
    label: `${index + 1}`,
    current: scoreFromDecision(item, "current"),
    shadow: scoreFromDecision(item, "shadow"),
  }));
}

function winCountItems(counts: Record<string, unknown>) {
  return Object.entries(counts)
    .map(([label, value]) => ({
      label: selectedAccountLabel(label),
      value: typeof value === "number" && Number.isFinite(value) ? value : 0,
      detail: formatInteger(typeof value === "number" ? value : 0),
    }))
    .filter((item) => item.value > 0)
    .sort((left, right) => right.value - left.value)
    .slice(0, 8);
}

function replayRows(items: SchedulerReplayItem[]) {
  return items.slice(0, 25).map((item) => ({
    rowKey: item.snapshot_id,
    created: (
      <div className="text-srapi-text-secondary font-mono text-2xs">
        {new Date(item.created_at).toLocaleString()}
      </div>
    ),
    request: (
      <div className="text-srapi-text-primary max-w-[220px] truncate font-mono text-xs">
        {item.request_id}
      </div>
    ),
    strategies: (
      <div className="space-y-1 text-xs">
        <div className="text-srapi-text-primary">
          {strategyLabel(item.current.strategy)} {"->"} {strategyLabel(item.shadow.strategy)}
        </div>
        <div className="text-srapi-text-secondary font-mono text-2xs">
          original {strategyLabel(item.original_strategy)}
        </div>
      </div>
    ),
    winners: (
      <div className="space-y-1 font-mono text-2xs">
        <div className="text-srapi-text-primary truncate">
          current {selectedAccountLabel(item.diff.current_selected_account_id)}
        </div>
        <div className="text-srapi-text-secondary truncate">
          shadow {selectedAccountLabel(item.diff.shadow_selected_account_id)}
        </div>
      </div>
    ),
    delta: (
      <div className="text-srapi-text-secondary space-y-1 font-mono text-2xs">
        <div>final {formatScore(item.diff.final_score_delta)}</div>
        <div>cost {formatScore(item.diff.cost_score_delta)}</div>
        <div>latency {formatScore(item.diff.latency_score_delta)}</div>
        <div>quality {formatScore(item.diff.quality_score_delta)}</div>
      </div>
    ),
    rollout: item.rollout.enabled ? (
      <div className="text-srapi-text-secondary space-y-1 font-mono text-2xs">
        <div>{item.rollout.shadow_selected ? "shadow" : "current"}</div>
        <div>bucket {item.rollout.bucket.toFixed(2)}</div>
      </div>
    ) : (
      <span className="text-srapi-text-secondary font-mono text-2xs">off</span>
    ),
  }));
}

function ScoreTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: Array<{ name?: string; value?: number | null; color?: string }>;
  label?: string;
}) {
  if (!active || !payload?.length) {
    return null;
  }

  return (
    <div className="border-srapi-border bg-srapi-card rounded-xl border px-3 py-2 text-xs shadow-sm">
      <div className="text-srapi-text-secondary mb-1 font-mono text-2xs">Replay #{label}</div>
      {payload.map((point) => (
        <div key={point.name} className="flex items-center justify-between gap-6">
          <span className="text-srapi-text-secondary">{point.name}</span>
          <span className="text-srapi-text-primary font-mono">{formatScore(point.value)}</span>
        </div>
      ))}
    </div>
  );
}

function StrategySelect({
  id,
  value,
  onChange,
  includeOriginal = false,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  includeOriginal?: boolean;
}) {
  return (
    <select
      id={id}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="border-srapi-border bg-srapi-card text-srapi-text-primary focus:border-srapi-primary h-10 w-full rounded-xl border px-3 font-mono text-xs transition outline-none"
    >
      {includeOriginal ? <option value="original">Original snapshot strategy</option> : null}
      {STRATEGY_OPTIONS.map((strategy) => (
        <option key={strategy} value={strategy}>
          {strategyLabel(strategy)}
        </option>
      ))}
    </select>
  );
}

export function AdminOpsStrategyPage() {
  const [form, setForm] = useState<StrategyFormState>(() => emptyFormState());
  const replay = useAdminSchedulerReplay();
  const result = replay.data;
  const points = useMemo(() => chartPoints(result), [result]);
  const winnerChangeRate = result
    ? formatPercentRatio(result.winner_changed, result.replayed)
    : "0.0%";

  const submitReplay = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    replay.mutate(buildReplayRequest(form));
  };

  return (
    <AdminShell>
      <div className="space-y-8">
        <AdminPageHeader
          title="Strategy Comparison"
          description="Replay sanitized Scheduler snapshots against two strategy choices without creating leases or new decisions."
          actions={
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => replay.mutate(buildReplayRequest(form))}
              disabled={replay.isPending}
            >
              <RefreshCw size={12} />
              Replay
            </Button>
          }
        />

        <AdminSection
          title="Replay Controls"
          description="Filters are sent to /api/v1/admin/scheduler/replay."
        >
          <form className="grid grid-cols-1 gap-4 lg:grid-cols-6" onSubmit={submitReplay}>
            <div className="space-y-2 lg:col-span-2">
              <Label htmlFor="strategy-current">Current Strategy</Label>
              <StrategySelect
                id="strategy-current"
                includeOriginal
                value={form.currentStrategy}
                onChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    currentStrategy: value as StrategyFormState["currentStrategy"],
                  }))
                }
              />
            </div>
            <div className="space-y-2 lg:col-span-2">
              <Label htmlFor="strategy-shadow">Shadow Strategy</Label>
              <StrategySelect
                id="strategy-shadow"
                value={form.shadowStrategy}
                onChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    shadowStrategy: value as SchedulerStrategyName,
                  }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="strategy-limit">Limit</Label>
              <Input
                id="strategy-limit"
                type="number"
                min="1"
                max="500"
                value={form.limit}
                onChange={(event) =>
                  setForm((current) => ({ ...current, limit: event.target.value }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="strategy-rollout">Rollout %</Label>
              <Input
                id="strategy-rollout"
                type="number"
                min="0"
                max="100"
                value={form.shadowRolloutPercent}
                onChange={(event) =>
                  setForm((current) => ({ ...current, shadowRolloutPercent: event.target.value }))
                }
              />
            </div>
            <div className="space-y-2 lg:col-span-2">
              <Label htmlFor="strategy-since">Since</Label>
              <Input
                id="strategy-since"
                type="datetime-local"
                value={form.since}
                onChange={(event) =>
                  setForm((current) => ({ ...current, since: event.target.value }))
                }
              />
            </div>
            <div className="space-y-2 lg:col-span-2">
              <Label htmlFor="strategy-until">Until</Label>
              <Input
                id="strategy-until"
                type="datetime-local"
                value={form.until}
                onChange={(event) =>
                  setForm((current) => ({ ...current, until: event.target.value }))
                }
              />
            </div>
            <div className="space-y-2 lg:col-span-2">
              <Label htmlFor="strategy-model">Model</Label>
              <Input
                id="strategy-model"
                value={form.model}
                onChange={(event) =>
                  setForm((current) => ({ ...current, model: event.target.value }))
                }
                placeholder="optional model filter"
              />
            </div>
            <div className="space-y-2 lg:col-span-3">
              <Label htmlFor="strategy-request">Request ID</Label>
              <Input
                id="strategy-request"
                value={form.requestId}
                onChange={(event) =>
                  setForm((current) => ({ ...current, requestId: event.target.value }))
                }
                placeholder="optional exact request id"
              />
            </div>
            <div className="flex items-end lg:col-span-1">
              <Button type="submit" className="w-full" disabled={replay.isPending}>
                <GitCompareArrows size={14} />
                Compare
              </Button>
            </div>
          </form>
        </AdminSection>

        {replay.isError ? <AdminErrorState error={replay.error} /> : null}
        {replay.isPending ? <AdminLoadingState label="Running strategy replay..." /> : null}

        {result ? (
          <>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
              <AdminStatCard
                label="Snapshots"
                value={formatInteger(result.replayed)}
                detail={`${formatInteger(result.requested)} requested / ${formatInteger(result.skipped)} skipped`}
                icon={<Route size={16} />}
              />
              <AdminStatCard
                label="Winner Changed"
                value={formatInteger(result.winner_changed)}
                detail={winnerChangeRate}
                icon={<Target size={16} />}
                tone={result.winner_changed > 0 ? "accent" : "neutral"}
              />
              <AdminStatCard
                label="Avg Final Delta"
                value={formatScore(result.average_final_score_delta)}
                detail={`risk ${formatScore(result.average_risk_penalty_delta)}`}
                icon={<Activity size={16} />}
              />
              <AdminStatCard
                label="Cost / Latency / Quality"
                value={formatScore(result.average_cost_score_delta)}
                detail={`${formatScore(result.average_latency_score_delta)} / ${formatScore(result.average_quality_score_delta)}`}
                icon={<BarChart3 size={16} />}
              />
            </div>

            <AdminSection
              title="Score Curves"
              description="Final score traces from the replayed snapshots."
            >
              {points.length ? (
                <div className="border-srapi-border bg-srapi-bg/40 h-[320px] rounded-2xl border p-4">
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart data={points} margin={{ top: 10, right: 18, left: 0, bottom: 10 }}>
                      <CartesianGrid stroke="var(--srapi-border)" strokeDasharray="3 3" />
                      <XAxis
                        dataKey="label"
                        tick={{ fontSize: 12 }}
                        stroke="var(--srapi-text-secondary)"
                      />
                      <YAxis
                        tick={{ fontSize: 12 }}
                        stroke="var(--srapi-text-secondary)"
                        width={44}
                      />
                      <Tooltip content={<ScoreTooltip />} />
                      <Line
                        type="monotone"
                        dataKey="current"
                        name="Current"
                        stroke="var(--srapi-text-secondary)"
                        strokeWidth={2}
                        dot={false}
                        connectNulls
                      />
                      <Line
                        type="monotone"
                        dataKey="shadow"
                        name="Shadow"
                        stroke="var(--srapi-primary)"
                        strokeWidth={2}
                        dot={false}
                        connectNulls
                      />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              ) : (
                <AdminEmptyState
                  title="No replayable snapshots matched"
                  description="The replay endpoint returned no comparable Scheduler snapshots for the current filter."
                />
              )}
            </AdminSection>

            <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
              <AdminSection title="Current Winners">
                <AdminBarList
                  emptyLabel="No current winners"
                  items={winCountItems(result.current_win_counts)}
                />
              </AdminSection>
              <AdminSection title="Shadow Winners">
                <AdminBarList
                  emptyLabel="No shadow winners"
                  items={winCountItems(result.shadow_win_counts)}
                />
              </AdminSection>
            </div>

            <AdminSection
              title="Replay Evidence"
              description="Rows show the first 25 replayed snapshots returned by the API."
            >
              <AdminTable
                empty={<AdminEmptyState title="No replay evidence" />}
                columns={[
                  { key: "created", header: "Created" },
                  { key: "request", header: "Request" },
                  { key: "strategies", header: "Strategies" },
                  { key: "winners", header: "Winners" },
                  { key: "delta", header: "Delta" },
                  { key: "rollout", header: "Rollout" },
                ]}
                rows={replayRows(result.items)}
                getRowKey={(row, index) => `${row.rowKey ?? index}`}
              />
            </AdminSection>
          </>
        ) : (
          <AdminEmptyState
            title="No replay run yet"
            description="Run a comparison to inspect historical Scheduler snapshots with the selected strategies."
          />
        )}
      </div>
    </AdminShell>
  );
}
