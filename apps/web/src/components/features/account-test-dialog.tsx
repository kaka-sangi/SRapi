"use client";

import { useMemo, useState } from "react";
import { CheckCircle2, XCircle, Loader2, Play, ShieldCheck } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/admin-format";
import type { AdminAccountTestRequest, AdminTestResult, Model } from "@/lib/sdk-types";

const MODEL_AUTO = "__auto__";
const DEFAULT_MODE: NonNullable<AdminAccountTestRequest["mode"]> = "live";
const DEFAULT_PROMPT = "Reply with OK.";

type AccountTestMode = NonNullable<AdminAccountTestRequest["mode"]>;

function formatLatency(ms: number | undefined): string {
  if (ms == null) return "—";
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${Math.round(ms)}ms`;
}

function stringifyCheck(value: unknown): string {
  if (value == null) return "—";
  if (typeof value === "boolean") return value ? "ok" : "fail";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

// Flatten a result into copyable plain text so the whole report is one click away.
function resultToText(name: string, result: AdminTestResult | undefined, error: string | null): string {
  const lines = [`account: ${name}`];
  if (error) {
    lines.push(`status: error`, `message: ${error}`);
    return lines.join("\n");
  }
  if (!result) return lines.join("\n");
  lines.push(`status: ${result.status}`, `latency: ${formatLatency(result.latency_ms)}`);
  if (result.message) lines.push(`message: ${result.message}`);
  const checks = result.checks as Record<string, unknown> | undefined;
  if (checks) {
    for (const [k, v] of Object.entries(checks)) lines.push(`  ${k}: ${stringifyCheck(v)}`);
  }
  lines.push(`checked_at: ${result.checked_at}`);
  return lines.join("\n");
}

/**
 * AccountTestDialog — presents a provider-account connectivity/capability test
 * (status, latency, message, per-check breakdown) in a terminal-style panel,
 * instead of a bare ok/fail badge. Purely presentational: the parent owns the
 * mutation; this dialog only builds the selected mode/model/prompt request.
 */
export function AccountTestDialog({
  open,
  onOpenChange,
  accountName,
  onRun,
  models,
  result,
  errorMessage,
  isPending,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accountName: string;
  onRun: (body: AdminAccountTestRequest) => void;
  models: Model[];
  result?: AdminTestResult;
  errorMessage?: string | null;
  isPending: boolean;
}) {
  const { t } = useLanguage();
  const activeModels = useMemo(
    () =>
      models
        .filter((model) => model.status === "active")
        .sort((a, b) => a.canonical_name.localeCompare(b.canonical_name)),
    [models],
  );
  const [mode, setMode] = useState<AccountTestMode>(DEFAULT_MODE);
  const [model, setModel] = useState(MODEL_AUTO);
  const [prompt, setPrompt] = useState(DEFAULT_PROMPT);

  const error = errorMessage ?? null;
  const loading = isPending;
  const ok = !loading && !error && result?.ok === true;
  const failed = !loading && (error != null || result?.ok === false);
  const checks = (result?.checks as Record<string, unknown> | undefined) ?? undefined;
  const promptDisabled = mode === "default";

  function run() {
    const body: AdminAccountTestRequest = { mode };
    if (model !== MODEL_AUTO) body.model = model;
    const trimmedPrompt = prompt.trim();
    if (mode !== "default" && trimmedPrompt) body.prompt = trimmedPrompt;
    onRun(body);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheck className="size-4 text-srapi-text-tertiary" />
            {t("providers.testTitle")}
          </DialogTitle>
          <DialogDescription className="truncate font-mono text-2xs">
            {accountName}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-3 rounded-lg border border-srapi-border bg-srapi-card-muted p-3.5 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="account-test-mode">{t("providers.testMode")}</Label>
            <Select value={mode} onValueChange={(next) => setMode(next as AccountTestMode)}>
              <SelectTrigger id="account-test-mode" className="rounded-lg bg-srapi-card">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="live">{t("providers.testModeLive")}</SelectItem>
                <SelectItem value="responses_compact">{t("providers.testModeCompact")}</SelectItem>
                <SelectItem value="default">{t("providers.testModeDefault")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="account-test-model">{t("providers.testModel")}</Label>
            <Select value={model} onValueChange={setModel} disabled={mode === "default"}>
              <SelectTrigger id="account-test-model" className="rounded-lg bg-srapi-card">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={MODEL_AUTO}>{t("providers.testModelAuto")}</SelectItem>
                {activeModels.map((item) => (
                  <SelectItem key={item.id} value={item.canonical_name}>
                    {item.display_name || item.canonical_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="account-test-prompt">{t("providers.testPrompt")}</Label>
            <Textarea
              id="account-test-prompt"
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
              disabled={promptDisabled}
              maxLength={4000}
              className="min-h-24 rounded-lg bg-srapi-card"
              placeholder={DEFAULT_PROMPT}
            />
          </div>
        </div>

        {/* Result panel — mono, status-tinted */}
        <div className="overflow-hidden rounded-lg border border-srapi-border bg-srapi-card-muted p-3.5 font-mono text-xs">
          <div className="flex items-center gap-2">
            {loading ? (
              <>
                <Loader2 className="size-3.5 animate-spin text-srapi-text-tertiary" />
                <span className="text-srapi-text-secondary">{t("providers.testRunning")}</span>
              </>
            ) : failed ? (
              <>
                <XCircle className="size-3.5 text-srapi-error" />
                <span className="text-srapi-error">{t("providers.testFailed")}</span>
                {result?.latency_ms != null ? (
                  <span className="ml-auto tabular text-srapi-text-tertiary">
                    {formatLatency(result.latency_ms)}
                  </span>
                ) : null}
              </>
            ) : ok ? (
              <>
                <CheckCircle2 className="size-3.5 text-srapi-success" />
                <span className="text-srapi-success">{t("providers.testOk")}</span>
                <span className="ml-auto tabular text-srapi-text-tertiary">
                  {formatLatency(result?.latency_ms)}
                </span>
              </>
            ) : (
              <span className="text-srapi-text-tertiary">{t("providers.testReady")}</span>
            )}
          </div>

          {!loading && (error || result?.message) ? (
            <p className="mt-2 text-srapi-text-secondary [overflow-wrap:anywhere]">{error || result?.message}</p>
          ) : null}

          {!loading && checks && Object.keys(checks).length > 0 ? (
            <dl className="mt-2.5 space-y-1 border-t border-srapi-border pt-2.5">
              {Object.entries(checks).map(([k, v]) => (
                <div key={k} className="flex items-baseline justify-between gap-3">
                  <dt className="shrink-0 text-srapi-text-tertiary">{k}</dt>
                  <dd
                    className={cn(
                      "min-w-0 tabular text-right [overflow-wrap:anywhere]",
                      v === true
                        ? "text-srapi-success"
                        : v === false
                          ? "text-srapi-error"
                          : "text-srapi-text-primary",
                    )}
                  >
                    {stringifyCheck(v)}
                  </dd>
                </div>
              ))}
            </dl>
          ) : null}

          {!loading && result?.checked_at ? (
            <p className="mt-2.5 text-[10px] text-srapi-text-tertiary">
              {formatDateTime(result.checked_at)}
            </p>
          ) : null}
        </div>

        <div className="flex items-center justify-end gap-2">
          <CopyButton value={resultToText(accountName, result, error)} label={t("common.copy")} />
          <Button variant="primary" size="sm" onClick={run} loading={isPending}>
            <Play className="size-3.5" />
            {result || error ? t("providers.testRerun") : t("providers.test")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
