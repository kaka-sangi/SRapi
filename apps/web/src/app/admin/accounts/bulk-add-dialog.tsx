"use client";

/**
 * BulkAddAccountsDialog — operator paste-many shortcut for provider accounts.
 *
 * Replaces the "click 'New account' 1000 times" / "script 1000 POSTs"
 * workflow with a single POST /api/v1/admin/accounts/batch call. The server
 * dedupes by name (within the batch + against existing accounts), surfaces
 * per-row failures in result.results[i].error, and never aborts the batch on
 * a single bad row.
 *
 * UX shape: left column = defaults (provider/runtime_class/group/proxy/…);
 * right column = textarea with "name,api_key" OR bare "api_key" per line.
 * After submit, the textarea is replaced with a result panel — failed rows
 * can be retried in-place via "Retry failed rows" without re-typing the
 * defaults.
 */

import { useMemo, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import {
  useAdminGroups,
  useAdminProxies,
  useBatchCreateAccounts,
} from "@/hooks/admin-queries";
import { adminErrorMessage } from "@/lib/admin-api";
import { ACCOUNT_RUNTIME_CLASSES } from "@/lib/admin-account-form";
import {
  buildDefaultAccountName,
  getProviderTemplate,
  providerLabelFor,
  type AccountProviderOption,
} from "@/components/admin/account-form-helpers";
import type {
  BatchCreateProviderAccountsResult,
  Id,
  RuntimeClass,
} from "@/lib/sdk-types";

// Server-side cap (matches OpenAPI maxItems + service.BatchCreateAccountsMaxItems).
// We surface it client-side so the operator gets immediate feedback before they
// hit submit on a 5000-line paste.
const MAX_ITEMS = 1000;

// "no group" sentinel for the Radix Select — Radix forbids "" as an item
// value, so we use a non-empty token and map it to undefined on submit.
const NO_GROUP_VALUE = "__none__";
const NO_PROXY_VALUE = "__none__";

type RuntimeChoice =
  | "api_key"
  | "cli_client_token"
  | "web_session_cookie"
  | "custom_reverse_proxy";

const BULK_RUNTIME_CHOICES: RuntimeChoice[] = [
  "api_key",
  "cli_client_token",
  "web_session_cookie",
  "custom_reverse_proxy",
];

interface ParsedItem {
  index: number;        // index in the parsed (valid) array
  rawLine: number;      // 1-based line number in the textarea — for error display
  name: string;
  credential: string;
}

interface ParsedRow {
  rawLine: number;
  ok: boolean;
  parsed?: ParsedItem;
  error?: string;
}

// Parse "name,api_key" OR bare "api_key" OR "name\tapi_key" — accept any of
// comma, tab, or pipe so an operator pasting from a spreadsheet, terminal, or
// note doesn't get a confusing "format" error.
function parseLines(text: string, stripComments: boolean, providerLabel: string): ParsedRow[] {
  const lines = text.split("\n");
  const rows: ParsedRow[] = [];
  let validCount = 0;
  for (let i = 0; i < lines.length; i++) {
    const rawLine = i + 1;
    let line = lines[i];
    if (stripComments) {
      line = line.trim();
      if (!line || line.startsWith("#")) continue;
    } else if (line.trim() === "") {
      continue;
    }
    // Permissive separator: comma > tab > pipe. Whitespace is NOT a separator
    // because real names + tokens commonly contain underscores/dashes but
    // never raw spaces.
    const sep = line.includes(",") ? "," : line.includes("\t") ? "\t" : line.includes("|") ? "|" : null;
    let name = "";
    let apiKey = "";
    if (sep) {
      const parts = line.split(sep).map((p) => p.trim()).filter((p) => p.length > 0);
      if (parts.length >= 2) {
        name = parts[0];
        apiKey = parts.slice(1).join(sep);
      } else if (parts.length === 1) {
        apiKey = parts[0];
      }
    } else {
      apiKey = line.trim();
    }
    if (!apiKey) {
      rows.push({ rawLine, ok: false, error: "empty" });
      continue;
    }
    if (!name) {
      name = buildDefaultAccountName(providerLabel, apiKey, validCount + 1);
    }
    rows.push({
      rawLine,
      ok: true,
      parsed: { index: validCount, rawLine, name, credential: apiKey },
    });
    validCount++;
  }
  return rows;
}

export function BulkAddAccountsDialog({
  open,
  onOpenChange,
  defaultProviderId,
  providerOptions,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultProviderId?: string;
  providerOptions: AccountProviderOption[];
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const groupsQuery = useAdminGroups();
  const proxiesQuery = useAdminProxies();
  const batchCreate = useBatchCreateAccounts();

  const groupOptions = useMemo(
    () =>
      (groupsQuery.data?.data ?? []).map((g) => ({ value: String(g.id), label: g.name })),
    [groupsQuery.data],
  );
  const proxyOptions = useMemo(
    () => (proxiesQuery.data?.data ?? []).map((p) => ({ value: p.id, label: p.name })),
    [proxiesQuery.data],
  );

  const [providerId, setProviderId] = useState<string>(defaultProviderId ?? "");
  const [runtimeClass, setRuntimeClass] = useState<RuntimeChoice>("api_key");
  const [groupId, setGroupId] = useState<string>(NO_GROUP_VALUE);
  const [proxyId, setProxyId] = useState<string>(NO_PROXY_VALUE);
  const [priority, setPriority] = useState<string>("");
  const [weight, setWeight] = useState<string>("");
  const [riskLevel, setRiskLevel] = useState<"normal" | "medium" | "high">("normal");
  const [baseUrl, setBaseUrl] = useState("");
  const [stripComments, setStripComments] = useState(true);
  const [text, setText] = useState("");
  const [result, setResult] = useState<BatchCreateProviderAccountsResult | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);

  const effectiveProviderId = providerId || defaultProviderId || providerOptions[0]?.value || "";

  const selectedProvider = useMemo(
    () => providerOptions.find((opt) => opt.value === effectiveProviderId),
    [effectiveProviderId, providerOptions],
  );
  const providerLabel = providerLabelFor(providerOptions, effectiveProviderId);
  const template = useMemo(
    () => getProviderTemplate(providerOptions, effectiveProviderId),
    [providerOptions, effectiveProviderId],
  );
  const templateBaseUrl =
    typeof template?.default_metadata?.base_url === "string"
      ? (template.default_metadata.base_url as string)
      : "";
  const runtimeOptions = useMemo(() => {
    const allowed = selectedProvider?.authMethods?.length
      ? selectedProvider.authMethods
      : ACCOUNT_RUNTIME_CLASSES;
    const scoped = BULK_RUNTIME_CHOICES.filter((rc) => allowed.includes(rc as RuntimeClass));
    return scoped.length > 0 ? scoped : BULK_RUNTIME_CHOICES;
  }, [selectedProvider]);
  const effectiveRuntimeClass = runtimeOptions.includes(runtimeClass)
    ? runtimeClass
    : (runtimeOptions[0] ?? "api_key");

  const parsedRows = useMemo(
    () => parseLines(text, stripComments, providerLabel),
    [text, stripComments, providerLabel],
  );
  const validRows = parsedRows.filter((r) => r.ok && r.parsed);
  const invalidRows = parsedRows.filter((r) => !r.ok);
  const overLimit = validRows.length > MAX_ITEMS;
  const canSubmit =
    !!effectiveProviderId && validRows.length > 0 && !overLimit && !batchCreate.isPending;

  function credentialKeyFor(rc: RuntimeChoice): string {
    if (rc === "web_session_cookie") return "cookie";
    if (rc === "cli_client_token" || rc === "custom_reverse_proxy") return "access_token";
    return "api_key";
  }

  function reset() {
    setText("");
    setResult(null);
    setSubmitError(null);
    batchCreate.reset();
  }

  async function submit(itemsOverride?: ParsedItem[]) {
    setSubmitError(null);
    const items = itemsOverride ?? validRows.map((r) => r.parsed!);
    if (items.length === 0) {
      setSubmitError(t("adminAccounts.bulkAddNoItems"));
      return;
    }
    if (items.length > MAX_ITEMS) {
      setSubmitError(t("adminAccounts.bulkAddMaxItemsExceeded", { max: MAX_ITEMS }));
      return;
    }
    const credKey = credentialKeyFor(effectiveRuntimeClass);
    const priorityNum = priority.trim() === "" ? undefined : Number.parseInt(priority, 10);
    const weightNum = weight.trim() === "" ? undefined : Number.parseFloat(weight);
    const groupNum =
      groupId === NO_GROUP_VALUE ? undefined : Number.parseInt(groupId, 10);
    const resolvedProxyId = proxyId === NO_PROXY_VALUE ? null : proxyId;
    const metadata: Record<string, unknown> = {};
    if (baseUrl.trim()) metadata.base_url = baseUrl.trim();
    try {
      const res = await batchCreate.mutateAsync({
        defaults: {
          provider_id: effectiveProviderId as Id,
          runtime_class: effectiveRuntimeClass as RuntimeClass,
          upstream_client: template?.upstream_client,
          group_id: groupNum,
          proxy_id: resolvedProxyId,
          priority: Number.isFinite(priorityNum) ? priorityNum : undefined,
          weight: Number.isFinite(weightNum) ? weightNum : undefined,
          risk_level: riskLevel,
          metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
        },
        items: items.map((item) => ({
          name: item.name,
          credential: { [credKey]: item.credential },
        })),
      });
      setResult(res);
      const toneSummary =
        res.failed > 0 && res.succeeded > 0
          ? "warning"
          : res.failed > 0
            ? "error"
            : "success";
      toast({
        title: `${t("adminAccounts.bulkAddSucceeded", { count: res.succeeded })} · ${t("adminAccounts.bulkAddFailed", { count: res.failed })}`,
        tone: toneSummary,
      });
    } catch (err) {
      setSubmitError(adminErrorMessage(err));
    }
  }

  function retryFailed() {
    if (!result) return;
    const failedNames = new Set(
      result.results.filter((row) => row.error && !row.account_id).map((row) => row.name),
    );
    // Filter the LAST submitted items by name — we still have them in the
    // parsed view, which is the source of truth pre-submit.
    const retryItems = validRows
      .map((r) => r.parsed!)
      .filter((item) => failedNames.has(item.name));
    if (retryItems.length === 0) {
      toast({ title: t("adminAccounts.bulkAddSucceeded", { count: 0 }), tone: "info" });
      return;
    }
    setResult(null);
    void submit(retryItems);
  }

  function handleOpenChange(next: boolean) {
    if (!next && result && result.succeeded > 0) {
      // Soft-confirm so the operator does not lose the per-row audit when
      // some accounts succeeded — but skip the prompt when nothing landed.
      const ok = window.confirm(
        t("adminAccounts.bulkAddCloseConfirm", { count: result.succeeded }),
      );
      if (!ok) return;
    }
    onOpenChange(next);
    if (!next) reset();
  }

  const validCount = validRows.length;
  const invalidCount = invalidRows.length;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("adminAccounts.bulkAddTitle")}</DialogTitle>
          <DialogDescription>{t("adminAccounts.bulkAddDescription")}</DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {/* LEFT — defaults form */}
          <div className="space-y-3" data-testid="bulk-defaults">
            <h3 className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("adminAccounts.bulkAddDefaultsHeading")}
            </h3>
            <div>
              <Label htmlFor="bulk-provider">{t("adminAccounts.provider")}</Label>
              <Select
                value={effectiveProviderId}
                onValueChange={(value) => {
                  setProviderId(value);
                  setBaseUrl("");
                }}
                disabled={batchCreate.isPending}
              >
                <SelectTrigger id="bulk-provider">
                  <SelectValue placeholder={t("adminAccounts.provider")} />
                </SelectTrigger>
                <SelectContent>
                  {providerOptions.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="bulk-runtime">{t("adminAccounts.authType")}</Label>
              <Select
                value={effectiveRuntimeClass}
                onValueChange={(v) => setRuntimeClass(v as RuntimeChoice)}
                disabled={batchCreate.isPending}
              >
                <SelectTrigger id="bulk-runtime">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {runtimeOptions.map((rc) => (
                    <SelectItem key={rc} value={rc}>
                      {t(`adminAccounts.runtime.${rc}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="bulk-base-url">{t("adminAccounts.baseUrl")}</Label>
              <Input
                id="bulk-base-url"
                type="url"
                className="font-mono"
                value={baseUrl}
                placeholder={templateBaseUrl || t("adminAccounts.baseUrlPlaceholder")}
                onChange={(e) => setBaseUrl(e.target.value)}
                disabled={batchCreate.isPending}
              />
            </div>
            <div>
              <Label htmlFor="bulk-group">{t("adminAccounts.bulkAddGroupOptional")}</Label>
              <Select value={groupId} onValueChange={setGroupId} disabled={batchCreate.isPending}>
                <SelectTrigger id="bulk-group">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_GROUP_VALUE}>{t("adminAccounts.bulkAddNoGroup")}</SelectItem>
                  {groupOptions.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="bulk-proxy">{t("adminAccounts.proxy")}</Label>
              <Select value={proxyId} onValueChange={setProxyId} disabled={batchCreate.isPending}>
                <SelectTrigger id="bulk-proxy">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_PROXY_VALUE}>{t("adminAccounts.noProxy")}</SelectItem>
                  {proxyOptions.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <Label htmlFor="bulk-priority">{t("adminAccounts.priority")}</Label>
                <Input
                  id="bulk-priority"
                  value={priority}
                  onChange={(e) => setPriority(e.target.value)}
                  placeholder="0"
                  inputMode="numeric"
                  disabled={batchCreate.isPending}
                />
              </div>
              <div>
                <Label htmlFor="bulk-weight">{t("adminAccounts.weight")}</Label>
                <Input
                  id="bulk-weight"
                  value={weight}
                  onChange={(e) => setWeight(e.target.value)}
                  placeholder="1"
                  inputMode="decimal"
                  disabled={batchCreate.isPending}
                />
              </div>
            </div>
            <div>
              <Label htmlFor="bulk-risk">{t("adminAccounts.riskLevel")}</Label>
              <Select
                value={riskLevel}
                onValueChange={(v) => setRiskLevel(v as typeof riskLevel)}
                disabled={batchCreate.isPending}
              >
                <SelectTrigger id="bulk-risk">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="normal">normal</SelectItem>
                  <SelectItem value="medium">medium</SelectItem>
                  <SelectItem value="high">high</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {/* RIGHT — items textarea + result panel */}
          <div className="space-y-3" data-testid="bulk-items">
            <h3 className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("adminAccounts.bulkAddItemsHeading")}
            </h3>
            {result ? (
              <div className="space-y-3">
                <h4 className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminAccounts.bulkAddResultsHeading")}
                </h4>
                <div className="grid grid-cols-2 gap-2">
                  <div className="rounded-xl border border-srapi-border bg-srapi-card p-3">
                    <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                      {t("adminAccounts.bulkAddSucceeded", { count: result.succeeded })}
                    </div>
                    <div className="mt-1 text-2xl font-semibold tracking-tight tabular text-srapi-success">
                      {result.succeeded}
                    </div>
                  </div>
                  <div className="rounded-xl border border-srapi-border bg-srapi-card p-3">
                    <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                      {t("adminAccounts.bulkAddFailed", { count: result.failed })}
                    </div>
                    <div className="mt-1 text-2xl font-semibold tracking-tight tabular text-srapi-error">
                      {result.failed}
                    </div>
                  </div>
                </div>
                <ul
                  className="max-h-64 space-y-1 overflow-y-auto rounded-xl border border-srapi-border bg-srapi-card-muted p-2"
                  data-testid="bulk-result-list"
                >
                  {result.results.map((row) => (
                    <li
                      key={`${row.index}-${row.name}`}
                      className="flex items-center justify-between gap-2 rounded-md px-1.5 py-1 text-[11px]"
                    >
                      <span className="truncate font-medium text-srapi-text-secondary">{row.name}</span>
                      {row.error && !row.account_id ? (
                        <span className="shrink-0 font-medium text-srapi-error" title={row.error}>
                          ✗ {row.error}
                        </span>
                      ) : (
                        <span className="shrink-0 font-medium text-srapi-success">
                          ✓ {t("adminAccounts.bulkAddRowOk")}{row.account_id ? ` (#${row.account_id})` : ""}
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
                {result.failed > 0 ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => retryFailed()}
                    loading={batchCreate.isPending}
                    className="btn-raise"
                  >
                    {t("adminAccounts.bulkAddRetryFailed")}
                  </Button>
                ) : null}
              </div>
            ) : (
              <>
                <Textarea
                  id="bulk-items-textarea"
                  rows={14}
                  spellCheck={false}
                  className="font-mono text-xs"
                  placeholder={t("adminAccounts.bulkAddItemsPlaceholder")}
                  value={text}
                  onChange={(e) => setText(e.target.value)}
                  disabled={batchCreate.isPending}
                  data-testid="bulk-items-textarea"
                />
                <div className="flex items-center justify-between">
                  <label className="flex items-center gap-2 text-xs text-srapi-text-tertiary">
                    <Switch
                      checked={stripComments}
                      onCheckedChange={setStripComments}
                      disabled={batchCreate.isPending}
                    />
                    {t("adminAccounts.bulkAddStripComments")}
                  </label>
                  <span className="text-xs tabular text-srapi-text-tertiary" data-testid="bulk-counts">
                    {t("adminAccounts.bulkAddValidCount", { count: validCount })}
                    {invalidCount > 0
                      ? ` · ${t("adminAccounts.bulkAddInvalidCount", { count: invalidCount })}`
                      : ""}
                  </span>
                </div>
                {overLimit ? (
                  <p className="text-xs text-srapi-error">
                    {t("adminAccounts.bulkAddMaxItemsExceeded", { max: MAX_ITEMS })}
                  </p>
                ) : null}
                {submitError ? (
                  <p className="text-xs text-srapi-error">{submitError}</p>
                ) : null}
              </>
            )}
          </div>
        </div>

        <DialogFooter className="mt-4">
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            {result ? t("common.close") : t("common.cancel")}
          </Button>
          {!result ? (
            <Button
              variant="primary"
              onClick={() => void submit()}
              disabled={!canSubmit}
              loading={batchCreate.isPending}
              data-testid="bulk-submit"
            >
              {t("adminAccounts.bulkAddSubmit", { count: validCount })}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
