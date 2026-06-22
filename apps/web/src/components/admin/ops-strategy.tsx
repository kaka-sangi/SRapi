"use client";

import { useMemo, useState } from "react";
import { PageHeader } from "@/components/layout/page-header";
import {
  useActivateSchedulerStrategy,
  useCreateSchedulerStrategy,
  useDeprecateSchedulerStrategy,
  useReplaySchedulerStrategy,
  useSchedulerOverview,
  useSchedulerStrategies,
  useSimulateSchedulerStrategy,
  useUpdateSchedulerStrategy,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatCard } from "@/components/ui/stat-card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { DataPill } from "@/components/ui/data-pill";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { FloatingInput } from "@/components/ui/floating-input";
import { cn } from "@/lib/cn";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableScroll } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { adminErrorMessage } from "@/lib/admin-api";
import type {
  SchedulerReplayResult,
  SchedulerSimulationRequest,
  SchedulerSimulationResult,
  SchedulerStrategy,
  SchedulerStrategyMutationRequest,
  SchedulerStrategyName,
  SchedulerStrategyScopeType,
  SchedulerStrategyStatus,
  SchedulerStrategyWeights,
} from "@/lib/sdk-types";

const NONE = "__none__";
const WEIGHT_KEYS = ["health", "quota", "latency", "sticky", "cache", "cost", "fairness", "quality"] as const;
const EMPTY_STRATEGIES: SchedulerStrategy[] = [];

const DEFAULT_SIMULATION = JSON.stringify(
  {
    current_strategy: "balanced",
    shadow_strategy: "cost_saver",
    shadow_rollout_percent: 100,
    rollout_key: "ops-preview",
    request: {
      request_id: "ops-preview",
      user_id: "1",
      api_key_id: "1",
      source_endpoint: "/v1/chat/completions",
      model: "ops-preview-model",
      candidates: [
        {
          account_id: "1",
          provider_id: "1",
          runtime_state: { health_score: 0.95 },
          pricing_override: { relative_cost: 0.9 },
        },
        {
          account_id: "2",
          provider_id: "2",
          runtime_state: { health_score: 0.6 },
          pricing_override: { relative_cost: 0.1 },
        },
      ],
    },
  },
  null,
  2,
);

export function StrategyContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const overview = useSchedulerOverview();
  const strategies = useSchedulerStrategies();
  const createMut = useCreateSchedulerStrategy();
  const updateMut = useUpdateSchedulerStrategy();
  const activateMut = useActivateSchedulerStrategy();
  const deprecateMut = useDeprecateSchedulerStrategy();
  const replayMut = useReplaySchedulerStrategy();
  const simulateMut = useSimulateSchedulerStrategy();

  const strategyItems = strategies.data?.data ?? EMPTY_STRATEGIES;
  const strategyNames = useMemo(() => uniqueStrategyNames(strategyItems), [strategyItems]);
  const [editor, setEditor] = useState<StrategyEditorState | null>(null);
  const [replayResult, setReplayResult] = useState<SchedulerReplayResult | null>(null);
  const [simulationText, setSimulationText] = useState(DEFAULT_SIMULATION);
  const [simulationResult, setSimulationResult] = useState<SchedulerSimulationResult | null>(null);

  const [shadow, setShadow] = useState<SchedulerStrategyName>("balanced");
  const [current, setCurrent] = useState<string>(NONE);
  const [limit, setLimit] = useState("100");
  const [model, setModel] = useState("");
  const [rolloutPercent, setRolloutPercent] = useState("");
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");

  function clearReplay<T>(setter: (v: T) => void) {
    return (v: T) => {
      setReplayResult(null);
      setter(v);
    };
  }

  async function runReplay() {
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
      setReplayResult(res);
      toast({ title: t("feedback.updated"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function runSimulation() {
    try {
      const body = JSON.parse(simulationText) as SchedulerSimulationRequest;
      const res = await simulateMut.mutateAsync(body);
      setSimulationResult(res);
      toast({ title: t("feedback.updated"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function submitStrategy(form: StrategyEditorState) {
    try {
      const body = editorBody(form);
      if (form.mode === "update" && form.id) {
        await updateMut.mutateAsync({ id: form.id, body });
      } else {
        await createMut.mutateAsync(body);
      }
      setEditor(null);
      toast({ title: t("feedback.updated"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function activate(strategy: SchedulerStrategy) {
    if (strategy.source === "seed") {
      return;
    }
    try {
      await activateMut.mutateAsync(strategy.id);
      toast({ title: t("feedback.updated"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function deprecate(strategy: SchedulerStrategy) {
    if (strategy.source === "seed") {
      return;
    }
    try {
      await deprecateMut.mutateAsync(strategy.id);
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

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          label={t("adminOps.totalDecisions")}
          value={overview.data?.total_decisions ?? 0}
          tooltip={{
            title: t("adminOps.totalDecisions"),
            rows: [
              {
                label: t("adminOps.selectedDecisions"),
                value: String(overview.data?.selected_decisions ?? 0),
                tone: "success",
              },
              {
                label: t("adminOps.failedDecisions"),
                value: String(overview.data?.failed_decisions ?? 0),
                tone: "error",
              },
            ],
          }}
        />
        <StatCard
          label={t("adminOps.selectedDecisions")}
          value={overview.data?.selected_decisions ?? 0}
        />
        <StatCard
          label={t("adminOps.failedDecisions")}
          value={overview.data?.failed_decisions ?? 0}
          className={overview.data?.failed_decisions ? "ring-1 ring-srapi-error/20" : undefined}
        />
        <StatCard
          label={t("adminOps.successRate")}
          value={`${Math.round((overview.data?.success_rate ?? 0) * 100)}%`}
          tooltip={{
            title: t("adminOps.successRate"),
            primary: `${((overview.data?.success_rate ?? 0) * 100).toFixed(2)}%`,
            rows: [
              {
                label: t("adminOps.selectedDecisions"),
                value: String(overview.data?.selected_decisions ?? 0),
                tone: "success",
              },
              {
                label: t("adminOps.failedDecisions"),
                value: String(overview.data?.failed_decisions ?? 0),
                tone: "error",
              },
            ],
          }}
        />
      </div>

      <Card>
        <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <CardTitle>{t("adminOps.strategyRegistry")}</CardTitle>
          <Button variant="primary" onClick={() => setEditor(emptyEditor(strategyNames[0] ?? "balanced"))}>
            {t("adminOps.createStrategy")}
          </Button>
        </CardHeader>
        <CardContent>
          <TableScroll minWidth={920}>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("adminOps.strategy")}</TableHead>
                  <TableHead>{t("adminOps.scope")}</TableHead>
                  <TableHead>{t("adminOps.weights")}</TableHead>
                  <TableHead>{t("adminOps.status")}</TableHead>
                  <TableHead className="text-right">{t("adminOps.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {strategyItems.map((strategy) => (
                  <TableRow key={strategy.id}>
                    <TableCell>
                      <div className="min-w-44">
                        <div className="font-medium text-srapi-text-primary">
                          {strategyNameLabel(t, strategy.name)}
                        </div>
                        <div className="text-xs text-srapi-text-tertiary tabular">
                          {strategy.version} · {strategy.config_hash}
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-xs text-srapi-text-secondary">
                        {strategy.scope_type}
                        {strategy.scope_id ? `:${strategy.scope_id}` : ""}
                      </div>
                    </TableCell>
                    <TableCell>
                      <WeightSummary weights={strategy.weights} />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <QuietBadge status={strategyTone(strategy.status)} label={strategy.status} />
                        <QuietBadge status={strategy.source === "seed" ? "disabled" : "active"} label={strategy.source} />
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        <Button size="sm" variant="outline" onClick={() => setEditor(editorFromStrategy(strategy))}>
                          {strategy.source === "seed" ? t("adminOps.copy") : t("common.edit")}
                        </Button>
                        {strategy.source !== "seed" && strategy.status !== "active" ? (
                          <Button size="sm" variant="outline" loading={activateMut.isPending} onClick={() => activate(strategy)}>
                            {t("adminOps.activate")}
                          </Button>
                        ) : null}
                        {strategy.source !== "seed" && strategy.status !== "deprecated" ? (
                          <Button size="sm" variant="danger" loading={deprecateMut.isPending} onClick={() => deprecate(strategy)}>
                            {t("adminOps.deprecate")}
                          </Button>
                        ) : null}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableScroll>
        </CardContent>
      </Card>

      <Tabs defaultValue="simulate">
        <TabsList>
          <TabsTrigger value="simulate">{t("adminOps.simulate")}</TabsTrigger>
          <TabsTrigger value="replay">{t("adminOps.replay")}</TabsTrigger>
        </TabsList>

        <TabsContent value="simulate">
          <Card>
            <CardContent className="space-y-4">
              <Textarea
                className="min-h-[320px] font-mono text-xs"
                value={simulationText}
                onChange={(e) => {
                  setSimulationResult(null);
                  setSimulationText(e.target.value);
                }}
              />
              <div className="flex justify-end">
                <Button variant="primary" loading={simulateMut.isPending} onClick={runSimulation}>
                  {t("adminOps.run")}
                </Button>
              </div>
            </CardContent>
          </Card>
          {simulationResult ? <SimulationResult result={simulationResult} /> : null}
        </TabsContent>

        <TabsContent value="replay">
          <Card>
            <CardContent className="space-y-5">
              <div className="grid gap-5 sm:grid-cols-2">
                <StrategySelect label={t("adminOps.shadowStrategy")} value={shadow} onValueChange={(v) => clearReplay(setShadow)(v as SchedulerStrategyName)} strategies={strategyNames} />
                <div>
                  <Label htmlFor="current">{t("adminOps.currentStrategy")}</Label>
                  <Select value={current} onValueChange={clearReplay(setCurrent)}>
                    <SelectTrigger id="current">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NONE}>-</SelectItem>
                      {strategyNames.map((name) => (
                        <SelectItem key={name} value={name}>
                          {strategyNameLabel(t, name)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <TextInput id="limit" label={t("adminOps.limit")} type="number" value={limit} onChange={clearReplay(setLimit)} />
                <TextInput id="model" label={t("adminOps.model")} value={model} onChange={clearReplay(setModel)} />
                <TextInput id="rolloutPercent" label={t("adminOps.rolloutPercent")} type="number" value={rolloutPercent} onChange={clearReplay(setRolloutPercent)} />
                <TextInput id="since" label={t("adminOps.since")} type="datetime-local" value={since} onChange={clearReplay(setSince)} />
                <TextInput id="until" label={t("adminOps.until")} type="datetime-local" value={until} onChange={clearReplay(setUntil)} />
              </div>
              <div className="flex justify-end">
                <Button variant="primary" loading={replayMut.isPending} onClick={runReplay}>
                  {t("adminOps.run")}
                </Button>
              </div>
            </CardContent>
          </Card>
          {replayResult ? <ReplayResult result={replayResult} /> : null}
        </TabsContent>
      </Tabs>

      {editor ? (
        <StrategyEditor
          value={editor}
          strategyNames={strategyNames}
          pending={createMut.isPending || updateMut.isPending}
          onChange={setEditor}
          onClose={() => setEditor(null)}
          onSubmit={submitStrategy}
        />
      ) : null}
    </>
  );
}

type StrategyEditorState = {
  mode: "create" | "update";
  id?: string;
  name: SchedulerStrategyName;
  version: string;
  status: SchedulerStrategyStatus;
  scopeType: SchedulerStrategyScopeType;
  scopeId: string;
  description: string;
  weights: Record<string, string>;
};

function emptyEditor(name: SchedulerStrategyName): StrategyEditorState {
  return {
    mode: "create",
    name,
    version: "v1",
    status: "active",
    scopeType: "global",
    scopeId: "",
    description: "",
    weights: Object.fromEntries(WEIGHT_KEYS.map((key) => [key, key === "health" ? "1" : "0"])),
  };
}

function editorFromStrategy(strategy: SchedulerStrategy): StrategyEditorState {
  return {
    mode: strategy.source === "seed" ? "create" : "update",
    id: strategy.source === "seed" ? undefined : strategy.id,
    name: strategy.name,
    version: strategy.source === "seed" ? nextVersion(strategy.version) : strategy.version,
    status: strategy.status,
    scopeType: strategy.scope_type,
    scopeId: strategy.scope_id ?? "",
    description: strategy.description ?? "",
    weights: Object.fromEntries(WEIGHT_KEYS.map((key) => [key, String(strategy.weights[key] ?? 0)])),
  };
}

function editorBody(form: StrategyEditorState): SchedulerStrategyMutationRequest {
  const weights: SchedulerStrategyWeights = {};
  for (const key of WEIGHT_KEYS) {
    const parsed = Number(form.weights[key] ?? "0");
    weights[key] = Number.isFinite(parsed) ? parsed : 0;
  }
  return {
    name: form.name,
    version: form.version.trim(),
    status: form.status,
    scope_type: form.scopeType,
    scope_id: form.scopeType === "global" ? undefined : form.scopeId.trim(),
    description: form.description.trim(),
    weights,
  };
}

function StrategyEditor({
  value,
  strategyNames,
  pending,
  onChange,
  onClose,
  onSubmit,
}: {
  value: StrategyEditorState;
  strategyNames: SchedulerStrategyName[];
  pending: boolean;
  onChange: (value: StrategyEditorState) => void;
  onClose: () => void;
  onSubmit: (value: StrategyEditorState) => void;
}) {
  const { t } = useLanguage();
  const weightTotal = WEIGHT_KEYS.reduce(
    (acc, k) => acc + (Number(value.weights[k] ?? "0") || 0),
    0,
  );
  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{value.mode === "update" ? t("adminOps.editStrategy") : t("adminOps.createStrategy")}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4 sm:grid-cols-2">
          <StrategySelect label={t("adminOps.strategy")} value={value.name} onValueChange={(name) => onChange({ ...value, name: name as SchedulerStrategyName })} strategies={strategyNames} />
          <FloatingInput
            id="strategy-version"
            label={t("adminOps.version")}
            value={value.version}
            onChange={(version) => onChange({ ...value, version })}
          />
          <EnumSelect label={t("adminOps.status")} value={value.status} values={["active", "draft", "deprecated"]} onValueChange={(status) => onChange({ ...value, status: status as SchedulerStrategyStatus })} />
          <EnumSelect label={t("adminOps.scope")} value={value.scopeType} values={["global", "api_key", "account_group", "user"]} onValueChange={(scopeType) => onChange({ ...value, scopeType: scopeType as SchedulerStrategyScopeType, scopeId: scopeType === "global" ? "" : value.scopeId })} />
          {value.scopeType !== "global" ? (
            <FloatingInput
              id="strategy-scope-id"
              label={t("adminOps.scopeId")}
              value={value.scopeId}
              onChange={(scopeId) => onChange({ ...value, scopeId })}
            />
          ) : null}
          <div className="sm:col-span-2">
            <Label htmlFor="strategy-description">{t("adminOps.description")}</Label>
            <Textarea id="strategy-description" value={value.description} onChange={(e) => onChange({ ...value, description: e.target.value })} />
          </div>
          <div className="sm:col-span-2 space-y-2">
            <div className="flex items-center justify-between">
              <Label className="!mb-0">{t("adminOps.weights")}</Label>
              <DataTooltip
                title={t("adminOps.weights")}
                primary={weightTotal.toFixed(2)}
                rows={WEIGHT_KEYS.map((k) => ({
                  label: k,
                  value: Number(value.weights[k] ?? "0").toFixed(2),
                  tone: Number(value.weights[k] ?? "0") > 0 ? "default" : "muted",
                }))}
              >
                <DataPill tone="accent" size="sm" className={cn("cursor-help tabular")}>
                  Σ {weightTotal.toFixed(2)}
                </DataPill>
              </DataTooltip>
            </div>
            <div className="grid gap-3 sm:grid-cols-4">
              {WEIGHT_KEYS.map((key) => (
                <TextInput
                  key={key}
                  id={`weight-${key}`}
                  label={key}
                  type="number"
                  value={value.weights[key] ?? "0"}
                  onChange={(next) => onChange({ ...value, weights: { ...value.weights, [key]: next } })}
                />
              ))}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button variant="primary" loading={pending} onClick={() => onSubmit(value)}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ReplayResult({ result }: { result: SchedulerReplayResult }) {
  const { t } = useLanguage();
  return (
    <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
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
  );
}

function SimulationResult({ result }: { result: SchedulerSimulationResult }) {
  const { t } = useLanguage();
  return (
    <div className="mt-4 grid gap-4 lg:grid-cols-3">
      <StatCard label={t("adminOps.currentWinner")} value={result.current.selected_account_id ?? "-"} />
      <StatCard label={t("adminOps.shadowWinner")} value={result.shadow.selected_account_id ?? "-"} />
      <StatCard label={t("adminOps.winnerChanged")} value={String(result.diff.winner_changed)} />
      <StatCard
        className="lg:col-span-3"
        label={t("adminOps.scoreDelta")}
        value={result.diff.final_score_delta.toFixed(3)}
        hint={
          <div className="flex flex-wrap gap-x-4 gap-y-1">
            <span>cost {result.diff.cost_score_delta.toFixed(3)}</span>
            <span>latency {result.diff.latency_score_delta.toFixed(3)}</span>
            <span>quality {result.diff.quality_score_delta.toFixed(3)}</span>
            <span>risk {result.diff.risk_penalty_delta.toFixed(3)}</span>
          </div>
        }
      />
    </div>
  );
}

function WeightSummary({ weights }: { weights: SchedulerStrategyWeights }) {
  return (
    <div className="flex max-w-[28rem] flex-wrap gap-1.5">
      {WEIGHT_KEYS.filter((key) => (weights[key] ?? 0) > 0).map((key) => (
        <DataPill key={key} tone="neutral" size="sm">
          {key}:{Number(weights[key] ?? 0).toFixed(2)}
        </DataPill>
      ))}
    </div>
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
          <span className="text-xs text-srapi-text-tertiary">-</span>
        ) : (
          entries.map(([key, value]) => (
            <div key={key} className="flex items-center justify-between gap-3 text-sm">
              <span className="truncate text-xs text-srapi-text-tertiary">{key}</span>
              <span className="tabular text-srapi-text-primary font-semibold">{String(value)}</span>
            </div>
          ))
        )}
      </CardContent>
    </Card>
  );
}

function StrategySelect({
  label,
  value,
  strategies,
  onValueChange,
}: {
  label: string;
  value: string;
  strategies: SchedulerStrategyName[];
  onValueChange: (value: string) => void;
}) {
  const { t } = useLanguage();
  return (
    <div>
      <Label>{label}</Label>
      <Select value={value} onValueChange={onValueChange}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {strategies.map((name) => (
            <SelectItem key={name} value={name}>
              {strategyNameLabel(t, name)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function EnumSelect({
  label,
  value,
  values,
  onValueChange,
}: {
  label: string;
  value: string;
  values: string[];
  onValueChange: (value: string) => void;
}) {
  return (
    <div>
      <Label>{label}</Label>
      <Select value={value} onValueChange={onValueChange}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {values.map((item) => (
            <SelectItem key={item} value={item}>
              {item}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function TextInput({
  id,
  label,
  value,
  onChange,
  type = "text",
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: string;
}) {
  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      <Input id={id} type={type} value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

function uniqueStrategyNames(items: SchedulerStrategy[]): SchedulerStrategyName[] {
  const names = new Set<SchedulerStrategyName>();
  for (const item of items) {
    names.add(item.name);
  }
  return Array.from(names);
}

function strategyTone(status: SchedulerStrategyStatus): QuietStatus {
  switch (status) {
    case "active":
      return "active";
    case "draft":
      return "limited";
    case "deprecated":
      return "disabled";
    default:
      return "disabled";
  }
}

function strategyNameLabel(t: (key: string) => string, name: SchedulerStrategyName) {
  return t(`adminOps.strategies.${name}`);
}

function nextVersion(version: string) {
  const trimmed = version.trim();
  if (!trimmed) {
    return "v1";
  }
  return `${trimmed}-copy`;
}
