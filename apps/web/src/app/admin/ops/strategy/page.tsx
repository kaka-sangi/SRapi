"use client";

import { useState } from "react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { useReplaySchedulerStrategy } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { adminErrorMessage } from "@/lib/admin-api";
import type { SchedulerReplayResult, SchedulerStrategyName } from "@/lib/sdk-types";

const STRATEGIES: SchedulerStrategyName[] = [
  "balanced",
  "cost_saver",
  "latency_first",
  "quota_protect",
  "sticky_first",
  "cache_affinity_first",
  "premium_quality",
];

const NONE = "__none__";

export default function OpsStrategyPage() {
  return (
    <AdminShell>
      <StrategyContent />
    </AdminShell>
  );
}

function StrategyContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const replayMut = useReplaySchedulerStrategy();

  const [shadow, setShadow] = useState<SchedulerStrategyName>("balanced");
  const [current, setCurrent] = useState<string>(NONE);
  const [limit, setLimit] = useState("100");
  const [model, setModel] = useState("");
  const [rolloutPercent, setRolloutPercent] = useState("");
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");
  const [result, setResult] = useState<SchedulerReplayResult | null>(null);

  // Clear any stored result whenever inputs change so a stale result never
  // appears to match the current form.
  function withClear<T>(setter: (v: T) => void) {
    return (v: T) => {
      setResult(null);
      setter(v);
    };
  }

  async function run() {
    try {
      const parsedRollout = rolloutPercent.trim() === "" ? undefined : Number(rolloutPercent);
      const res = await replayMut.mutateAsync({
        shadow_strategy: shadow,
        current_strategy: current === NONE ? undefined : (current as SchedulerStrategyName),
        limit: Number(limit) || 100,
        model: model.trim() || undefined,
        ...(parsedRollout !== undefined && Number.isFinite(parsedRollout)
          ? { shadow_rollout_percent: parsedRollout }
          : {}),
        ...(since.trim() ? { since: since.trim() } : {}),
        ...(until.trim() ? { until: until.trim() } : {}),
      });
      setResult(res);
      toast({ title: t("feedback.updated"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminOps.strategyTitle")}
        description={t("adminOps.strategySubtitle")}
      />

      <Card>
        <CardContent className="space-y-5">
          <div className="grid gap-5 sm:grid-cols-2">
            <div>
              <Label htmlFor="shadow">{t("adminOps.shadowStrategy")}</Label>
              <Select
                value={shadow}
                onValueChange={withClear((v) => setShadow(v as SchedulerStrategyName))}
              >
                <SelectTrigger id="shadow">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {STRATEGIES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {t(`adminOps.strategies.${s}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="current">{t("adminOps.currentStrategy")}</Label>
              <Select value={current} onValueChange={withClear(setCurrent)}>
                <SelectTrigger id="current">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>—</SelectItem>
                  {STRATEGIES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {t(`adminOps.strategies.${s}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="limit">{t("adminOps.limit")}</Label>
              <Input
                id="limit"
                type="number"
                value={limit}
                onChange={(e) => withClear(setLimit)(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="model">{t("adminOps.model")}</Label>
              <Input
                id="model"
                value={model}
                onChange={(e) => withClear(setModel)(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="rolloutPercent">{t("adminOps.rolloutPercent")}</Label>
              <Input
                id="rolloutPercent"
                type="number"
                value={rolloutPercent}
                onChange={(e) => withClear(setRolloutPercent)(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="since">{t("adminOps.since")}</Label>
              <Input
                id="since"
                type="datetime-local"
                value={since}
                onChange={(e) => withClear(setSince)(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="until">{t("adminOps.until")}</Label>
              <Input
                id="until"
                type="datetime-local"
                value={until}
                onChange={(e) => withClear(setUntil)(e.target.value)}
              />
            </div>
          </div>
          <div className="flex justify-end">
            <Button variant="primary" loading={replayMut.isPending} onClick={run}>
              {t("adminOps.run")}
            </Button>
          </div>
        </CardContent>
      </Card>

      {result ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label={t("adminOps.requested")} value={String(result.requested)} />
          <StatCard label={t("adminOps.replayed")} value={String(result.replayed)} />
          <StatCard label={t("adminOps.skipped")} value={String(result.skipped)} />
          <StatCard label={t("adminOps.winnerChanged")} value={String(result.winner_changed)} />
          <StatCard
            className="sm:col-span-2"
            label={t("adminOps.avgScoreDelta")}
            value={result.average_final_score_delta.toFixed(3)}
            hint={
              <div className="flex flex-wrap gap-x-4 gap-y-1">
                <span>cost {result.average_cost_score_delta.toFixed(3)}</span>
                <span>latency {result.average_latency_score_delta.toFixed(3)}</span>
                <span>quality {result.average_quality_score_delta.toFixed(3)}</span>
                <span>risk {result.average_risk_penalty_delta.toFixed(3)}</span>
              </div>
            }
          />
          <WinCounts label={t("adminOps.currentWins")} counts={result.current_win_counts} />
          <WinCounts label={t("adminOps.shadowWins")} counts={result.shadow_win_counts} />
        </div>
      ) : null}
    </>
  );
}

function WinCounts({ label, counts }: { label: string; counts: Record<string, unknown> }) {
  const entries = Object.entries(counts);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{label}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-1.5">
        {entries.length === 0 ? (
          <span className="font-mono text-2xs text-srapi-text-tertiary">—</span>
        ) : (
          entries.map(([key, value]) => (
            <div key={key} className="flex items-center justify-between gap-3 text-sm">
              <span className="truncate font-mono text-2xs text-srapi-text-tertiary">{key}</span>
              <span className="font-serif tabular text-srapi-text-primary">{String(value)}</span>
            </div>
          ))
        )}
      </CardContent>
    </Card>
  );
}
