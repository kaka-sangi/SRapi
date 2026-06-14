"use client";

import { useCallback, useRef, useState } from "react";
import { KeyRound } from "lucide-react";
import {
  AccountOAuthAuthorizeDialog,
  type AccountOAuthFlowMode,
  type ProvisionedTokens,
} from "@/components/admin/account-oauth-authorize-dialog";
import { CodexImportResultPanel } from "@/components/admin/codex-session-import-dialog";
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
import { FileDropZone } from "@/components/ui/file-drop-zone";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useCreateAccount, useImportAccounts, useImportCodexSession } from "@/hooks/admin-queries";
import { buildImportAccountsBody } from "@/lib/admin-account-form";
import { adminApi, adminErrorMessage } from "@/lib/admin-api";
import type { CRSPreviewResult, CRSSyncResult } from "@/lib/admin-api";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import type {
  CodexSessionImportResult,
  Id,
  ProviderAccountImportResult,
  RuntimeClass,
} from "@/lib/sdk-types";

export function AccountImportDialog({
  open,
  onOpenChange,
  providerOptions,
  codexProviderOptions,
  defaultProviderId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerOptions: { value: string; label: string }[];
  // Codex/ChatGPT sessions can only be imported into a codex-cli reverse-proxy
  // provider. When provided, the "Import Codex session" tab is restricted to
  // these (vs. listing every provider, which would let an operator pick an
  // incompatible one and fail confusingly). Falls back to all providers.
  codexProviderOptions?: { value: string; label: string }[];
  defaultProviderId: string;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const codexOptions =
    codexProviderOptions && codexProviderOptions.length > 0 ? codexProviderOptions : providerOptions;
  const defaultCodexProviderId = codexOptions[0]?.value ?? defaultProviderId;
  const importMut = useImportAccounts();
  const codexImportMut = useImportCodexSession();
  const createAccountMut = useCreateAccount();
  const [tab, setTab] = useState<"json" | "codex" | "oauth" | "batch" | "crs">("batch");
  const [json, setJson] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ProviderAccountImportResult | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [codexProviderId, setCodexProviderId] = useState<string>(defaultCodexProviderId);
  const [codexContent, setCodexContent] = useState("");
  const [codexName, setCodexName] = useState("");
  const [codexUpdateExisting, setCodexUpdateExisting] = useState(true);
  const [codexResult, setCodexResult] = useState<CodexSessionImportResult | null>(null);
  const [codexFileNames, setCodexFileNames] = useState<string[]>([]);
  const [oauthProviderId, setOAuthProviderId] = useState<string>(defaultProviderId);
  const [oauthName, setOAuthName] = useState("");
  const [oauthMode, setOAuthMode] = useState<AccountOAuthFlowMode>("authorization_code");
  const [oauthWizardOpen, setOAuthWizardOpen] = useState(false);
  const [batchProviderId, setBatchProviderId] = useState<string>(defaultProviderId);
  const [batchAuthType, setBatchAuthType] = useState<"api_key" | "cli_client_token" | "web_session_cookie">("api_key");
  const [batchPrefix, setBatchPrefix] = useState("");
  const [batchLines, setBatchLines] = useState("");
  const [batchResult, setBatchResult] = useState<{ created: number; failed: number; errors: string[] } | null>(null);
  const [crsUrl, setCrsUrl] = useState("");
  const [crsUser, setCrsUser] = useState("");
  const [crsPass, setCrsPass] = useState("");
  const [crsStep, setCrsStep] = useState<"input" | "preview" | "result">("input");
  const [crsPreview, setCrsPreview] = useState<CRSPreviewResult | null>(null);
  const [crsSelected, setCrsSelected] = useState<Set<string>>(new Set());
  const [crsSyncing, setCrsSyncing] = useState(false);
  const [crsResult, setCrsResult] = useState<CRSSyncResult | null>(null);
  const [batchProgress, setBatchProgress] = useState<{ current: number; total: number } | null>(null);
  const batchProgressRef = useRef<HTMLDivElement>(null);

  const handleFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return;
    const text = await files[0].text();
    setJson(text);
    setFileName(files[0].name);
  }, []);

  const handleCodexFiles = useCallback(async (files: File[]) => {
    const texts = await Promise.all(files.map((f) => f.text()));
    // Replace on each drop (combining only the files in this drop) rather than
    // appending across drops — dropping a corrected file should not silently
    // concatenate it onto the previous (wrong) one.
    setCodexContent(texts.join("\n"));
    setCodexFileNames(files.map((f) => f.name));
  }, []);

  function reset() {
    setTab("batch");
    setJson("");
    setError(null);
    setResult(null);
    setFileName(null);
    setCodexProviderId(defaultProviderId);
    setCodexContent("");
    setCodexName("");
    setCodexUpdateExisting(true);
    setCodexResult(null);
    setCodexFileNames([]);
    setOAuthProviderId(defaultProviderId);
    setOAuthName("");
    setOAuthMode("authorization_code");
    setOAuthWizardOpen(false);
    setBatchProviderId(defaultProviderId);
    setBatchAuthType("api_key");
    setBatchPrefix("");
    setBatchLines("");
    setBatchResult(null);
    setBatchProgress(null);
    setCrsUrl("");
    setCrsUser("");
    setCrsPass("");
    setCrsStep("input");
    setCrsPreview(null);
    setCrsSelected(new Set());
    setCrsSyncing(false);
    setCrsResult(null);
  }

  async function submitJson() {
    setError(null);
    let body: ReturnType<typeof buildImportAccountsBody>;
    try {
      body = buildImportAccountsBody(json);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Invalid import JSON.");
      return;
    }
    try {
      const res = await importMut.mutateAsync(body);
      setResult(res);
      toast({
        title: t("adminAccounts.importDone", {
          created: res.created_count,
          skipped: res.skipped_count,
        }),
        tone: res.errors.length > 0 ? "default" : "success",
      });
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  async function submitCodex() {
    setError(null);
    if (!codexProviderId) {
      setError(t("codexImport.providerRequired"));
      return;
    }
    if (!codexContent.trim()) {
      setError(t("codexImport.contentRequired"));
      return;
    }
    try {
      const data = await codexImportMut.mutateAsync({
        provider_id: codexProviderId as Id,
        content: codexContent,
        name: codexName.trim() ? codexName.trim() : undefined,
        update_existing: codexUpdateExisting,
      });
      setCodexResult(data);
      toast({
        title: t("codexImport.done"),
        description: t("codexImport.doneSummary", {
          created: data.created,
          updated: data.updated,
          skipped: data.skipped,
          failed: data.failed,
        }),
        tone: data.failed > 0 ? "error" : "success",
      });
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  async function applyProvisionedTokens(tokens: ProvisionedTokens) {
    setError(null);
    if (!oauthProviderId) {
      setError(t("codexImport.providerRequired"));
      return;
    }
    const credential: Record<string, string> = {};
    if (tokens.accessToken) credential.access_token = tokens.accessToken;
    if (tokens.refreshToken) credential.refresh_token = tokens.refreshToken;
    try {
      await createAccountMut.mutateAsync({
        provider_id: oauthProviderId as Id,
        name: oauthName.trim() || `OAuth account ${new Date().toISOString()}`,
        runtime_class: (oauthMode === "authorization_code"
          ? "oauth_refresh"
          : "oauth_device_code") as RuntimeClass,
        credential,
        status: "active",
      });
      toast({ title: t("feedback.created"), tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  async function submitBatch() {
    setError(null);
    setBatchResult(null);
    if (!batchProviderId) {
      setError(t("codexImport.providerRequired"));
      return;
    }
    const rawLines = batchLines
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith("#"));
    if (rawLines.length === 0) {
      setError(t("batchAdd.linesRequired"));
      return;
    }
    const entries: { baseUrl: string; apiKey: string }[] = [];
    for (const line of rawLines) {
      const sep = line.includes("|") ? "|" : line.includes(",") ? "," : /\s+/;
      const parts = line.split(sep).map((p) => p.trim()).filter(Boolean);
      if (parts.length >= 2) {
        entries.push({ baseUrl: parts[0], apiKey: parts[1] });
      } else if (parts.length === 1) {
        entries.push({ baseUrl: "", apiKey: parts[0] });
      }
    }
    if (entries.length === 0) {
      setError(t("batchAdd.linesRequired"));
      return;
    }
    let created = 0;
    let failed = 0;
    const errors: string[] = [];
    setBatchProgress({ current: 0, total: entries.length });
    requestAnimationFrame(() => batchProgressRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" }));
    for (let i = 0; i < entries.length; i++) {
      const entry = entries[i];
      const accountName = batchPrefix.trim()
        ? `${batchPrefix.trim()}-${i + 1}`
        : `account-${i + 1}`;
      const metadata: Record<string, unknown> = {};
      if (entry.baseUrl) metadata.base_url = entry.baseUrl;
      const credKey = batchAuthType === "web_session_cookie" ? "cookie" : batchAuthType === "cli_client_token" ? "access_token" : "api_key";
      try {
        await createAccountMut.mutateAsync({
          provider_id: batchProviderId as Id,
          name: accountName,
          runtime_class: batchAuthType as RuntimeClass,
          credential: { [credKey]: entry.apiKey },
          metadata,
          status: "active",
        });
        created++;
      } catch (err) {
        failed++;
        errors.push(`#${i + 1}: ${adminErrorMessage(err)}`);
      }
      setBatchProgress({ current: i + 1, total: entries.length });
    }
    setBatchProgress(null);
    setBatchResult({ created, failed, errors });
    toast({
      title: t("batchAdd.done", { created, failed }),
      tone: failed > 0 ? (created > 0 ? "warning" : "error") : "success",
    });
    if (failed === 0) {
      onOpenChange(false);
    }
  }

  const busy =
    importMut.isPending || codexImportMut.isPending || createAccountMut.isPending;

  return (
    <>
      <AccountOAuthAuthorizeDialog
        open={oauthWizardOpen}
        onOpenChange={setOAuthWizardOpen}
        mode={oauthMode}
        providerId={oauthProviderId}
        onProvisioned={(tokens) => void applyProvisionedTokens(tokens)}
      />
      <Dialog
        open={open}
        onOpenChange={(next) => {
          onOpenChange(next);
          if (!next) reset();
        }}
      >
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("adminAccounts.importTitle")}</DialogTitle>
          <DialogDescription>
            {tab === "codex" ? t("codexImport.dialogHint") : t("adminAccounts.importHint")}
          </DialogDescription>
        </DialogHeader>

        <Tabs value={tab} onValueChange={(v) => setTab(v as typeof tab)}>
          <TabsList className="mt-4 flex-wrap">
            <TabsTrigger value="batch">{t("batchAdd.tab")}</TabsTrigger>
            <TabsTrigger value="crs">{t("crsSync.tab")}</TabsTrigger>
            <TabsTrigger value="json">{t("adminAccounts.importJson")}</TabsTrigger>
            <TabsTrigger value="codex">{t("codexImport.action")}</TabsTrigger>
            <TabsTrigger value="oauth">{t("accountOAuth.authorizeAccount")}</TabsTrigger>
          </TabsList>

          <TabsContent value="batch">
            <div className="space-y-4">
              <div>
                <Label htmlFor="batch-provider">{t("adminAccounts.provider")}</Label>
                <Select value={batchProviderId} onValueChange={setBatchProviderId} disabled={busy}>
                  <SelectTrigger id="batch-provider">
                    <SelectValue placeholder={t("codexImport.providerPlaceholder")} />
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
                <Label htmlFor="batch-auth-type">{t("adminAccounts.authType")}</Label>
                <Select value={batchAuthType} onValueChange={(v) => setBatchAuthType(v as typeof batchAuthType)} disabled={busy}>
                  <SelectTrigger id="batch-auth-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="api_key">{t("adminAccounts.runtime.api_key")}</SelectItem>
                    <SelectItem value="cli_client_token">{t("adminAccounts.runtime.cli_client_token")}</SelectItem>
                    <SelectItem value="web_session_cookie">{t("adminAccounts.runtime.web_session_cookie")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="batch-prefix">{t("batchAdd.namePrefix")}</Label>
                <Input
                  id="batch-prefix"
                  placeholder={t("batchAdd.namePrefixPlaceholder")}
                  value={batchPrefix}
                  onChange={(e) => setBatchPrefix(e.target.value)}
                  disabled={busy}
                />
              </div>
              <div>
                <Label htmlFor="batch-lines">{t("batchAdd.lines")}</Label>
                <Textarea
                  id="batch-lines"
                  rows={10}
                  spellCheck={false}
                  className="font-mono text-xs"
                  placeholder={
                    batchAuthType === "web_session_cookie"
                      ? "https://api.example.com/v1|cookie-value-here\ncookie-only-value"
                      : batchAuthType === "cli_client_token"
                        ? "https://api.example.com/v1|eyJ...\neyJ..."
                        : t("batchAdd.linesPlaceholder")
                  }
                  value={batchLines}
                  onChange={(e) => setBatchLines(e.target.value)}
                  disabled={busy}
                />
                <div className="mt-1 flex items-center justify-between">
                  <p className="text-2xs text-srapi-text-tertiary">
                    {t("batchAdd.linesHint")}
                  </p>
                  {batchLines.trim() ? (
                    <span className="font-mono text-2xs text-srapi-text-tertiary">
                      {t("batchAdd.lineCount", { count: batchLines.split("\n").filter((l) => l.trim() && !l.trim().startsWith("#")).length })}
                    </span>
                  ) : null}
                </div>
              </div>
              {batchProgress ? (
                <div ref={batchProgressRef} className="space-y-1.5">
                  <div className="flex items-center justify-between text-2xs text-srapi-text-secondary">
                    <span>{t("batchAdd.progress", { current: batchProgress.current, total: batchProgress.total })}</span>
                    <span className="font-mono tabular">{Math.round((batchProgress.current / batchProgress.total) * 100)}%</span>
                  </div>
                  <div className="relative h-2 overflow-hidden rounded-full bg-srapi-border">
                    <div
                      className="h-full rounded-full bg-srapi-primary transition-all"
                      style={{ width: `${(batchProgress.current / batchProgress.total) * 100}%` }}
                    />
                  </div>
                </div>
              ) : null}
              {batchResult ? (
                <div className="space-y-2 rounded-md border border-srapi-border bg-srapi-card-muted p-3">
                  <div className="grid grid-cols-2 gap-2 text-center">
                    <ImportStat label={t("codexImport.created")} value={batchResult.created} tone="success" />
                    <ImportStat label={t("codexImport.failed")} value={batchResult.failed} tone="error" />
                  </div>
                  {batchResult.errors.length > 0 ? (
                    <ul className="list-disc space-y-0.5 pl-4 text-2xs text-srapi-error">
                      {batchResult.errors.map((msg, idx) => (
                        <li key={idx}>{msg}</li>
                      ))}
                    </ul>
                  ) : null}
                </div>
              ) : null}
            </div>
          </TabsContent>

          <TabsContent value="crs">
            <div className="space-y-4">
              {crsStep === "input" && (
                <>
                  <div>
                    <Label htmlFor="crs-url">{t("crsSync.baseUrl")}</Label>
                    <Input id="crs-url" placeholder="https://crs.example.com" value={crsUrl} onChange={(e) => setCrsUrl(e.target.value)} disabled={busy} />
                  </div>
                  <div>
                    <Label htmlFor="crs-user">{t("crsSync.username")}</Label>
                    <Input id="crs-user" value={crsUser} onChange={(e) => setCrsUser(e.target.value)} disabled={busy} />
                  </div>
                  <div>
                    <Label htmlFor="crs-pass">{t("crsSync.password")}</Label>
                    <Input id="crs-pass" type="password" value={crsPass} onChange={(e) => setCrsPass(e.target.value)} disabled={busy} />
                  </div>
                </>
              )}
              {crsStep === "preview" && crsPreview && (
                <>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium text-srapi-text-primary">{t("crsSync.newAccounts")} ({crsPreview.new_accounts.length})</span>
                      <div className="flex gap-2 text-2xs">
                        <button type="button" className="text-srapi-text-tertiary hover:text-srapi-text-secondary" onClick={() => setCrsSelected(new Set(crsPreview.new_accounts.map((a) => a.crs_account_id)))}>{t("adminQuickSetup.selectAll")}</button>
                        <button type="button" className="text-srapi-text-tertiary hover:text-srapi-text-secondary" onClick={() => setCrsSelected(new Set())}>{t("adminQuickSetup.selectNone")}</button>
                      </div>
                    </div>
                    {crsPreview.new_accounts.map((a) => (
                      <label key={a.crs_account_id} className="flex items-center gap-2 rounded-md border border-srapi-border px-3 py-2 text-sm cursor-pointer hover:bg-srapi-card-muted">
                        <input type="checkbox" checked={crsSelected.has(a.crs_account_id)} onChange={() => {
                          setCrsSelected(prev => {
                            const next = new Set(prev);
                            if (next.has(a.crs_account_id)) next.delete(a.crs_account_id); else next.add(a.crs_account_id);
                            return next;
                          });
                        }} />
                        <span className="text-srapi-text-primary">{a.name}</span>
                        <span className="ml-auto font-mono text-2xs text-srapi-text-tertiary">{a.platform} · {a.type}</span>
                      </label>
                    ))}
                    {crsPreview.existing_accounts.length > 0 && (
                      <div className="mt-3">
                        <span className="text-sm text-srapi-text-secondary">{t("crsSync.existingAccounts")} ({crsPreview.existing_accounts.length})</span>
                        <div className="mt-1 space-y-1">
                          {crsPreview.existing_accounts.map((a) => (
                            <div key={a.crs_account_id} className="flex items-center gap-2 rounded-md border border-srapi-border/50 bg-srapi-card-muted px-3 py-2 text-sm opacity-60">
                              <span>{a.name}</span>
                              <span className="ml-auto font-mono text-2xs text-srapi-text-tertiary">{a.platform}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                </>
              )}
              {crsStep === "result" && crsResult && (
                <div className="space-y-2 rounded-md border border-srapi-border bg-srapi-card-muted p-3">
                  <div className="grid grid-cols-4 gap-2 text-center">
                    <ImportStat label={t("codexImport.created")} value={crsResult.created} tone="success" />
                    <ImportStat label={t("codexImport.updated")} value={crsResult.updated} />
                    <ImportStat label={t("codexImport.skipped")} value={crsResult.skipped} />
                    <ImportStat label={t("codexImport.failed")} value={crsResult.failed} tone="error" />
                  </div>
                </div>
              )}
            </div>
          </TabsContent>

          <TabsContent value="json">
            <div className="space-y-3">
              <div>
                <Label htmlFor="import-json">{t("adminAccounts.importJson")}</Label>
                <FileDropZone
                  accept=".json"
                  disabled={busy}
                  hint={t("adminAccounts.importDropHint")}
                  onFiles={(files) => void handleFiles(files)}
                  fileNames={fileName ? [fileName] : undefined}
                  onClearFiles={() => {
                    setFileName(null);
                    setJson("");
                  }}
                  className="mb-2"
                />
                <Textarea
                  id="import-json"
                  rows={10}
                  spellCheck={false}
                  className="font-mono text-xs"
                  value={json}
                  onChange={(e) => setJson(e.target.value)}
                  placeholder={'{ "accounts": [ { "name": "...", "provider_id": "...", "runtime_class": "...", "credential": { } } ] }'}
                  disabled={busy}
                />
              </div>
              {result ? <ProviderImportResultPanel result={result} /> : null}
            </div>
          </TabsContent>

          <TabsContent value="codex">
            <div className="space-y-4">
              <div>
                <Label htmlFor="codex-import-provider">{t("codexImport.provider")}</Label>
                <Select value={codexProviderId} onValueChange={setCodexProviderId} disabled={busy}>
                  <SelectTrigger id="codex-import-provider">
                    <SelectValue placeholder={t("codexImport.providerPlaceholder")} />
                  </SelectTrigger>
                  <SelectContent>
                    {codexOptions.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="codex-import-content">{t("codexImport.content")}</Label>
                <FileDropZone
                  accept=".json,.txt,.ndjson"
                  multiple
                  disabled={busy}
                  hint={t("codexImport.dropHint")}
                  onFiles={(files) => void handleCodexFiles(files)}
                  fileNames={codexFileNames}
                  onClearFiles={() => {
                    setCodexFileNames([]);
                    setCodexContent("");
                  }}
                  className="mb-2"
                />
                <Textarea
                  id="codex-import-content"
                  rows={8}
                  spellCheck={false}
                  className="font-mono text-xs"
                  placeholder={t("codexImport.contentPlaceholder")}
                  value={codexContent}
                  onChange={(e) => setCodexContent(e.target.value)}
                  disabled={busy}
                />
                <p className="mt-1 text-xs text-srapi-text-tertiary">{t("codexImport.contentHint")}</p>
              </div>
              <div>
                <Label htmlFor="codex-import-name">{t("codexImport.name")}</Label>
                <Input
                  id="codex-import-name"
                  placeholder={t("codexImport.namePlaceholder")}
                  value={codexName}
                  onChange={(e) => setCodexName(e.target.value)}
                  disabled={busy}
                />
              </div>
              <div className="flex items-center justify-between rounded-md border border-srapi-border px-3 py-2">
                <div>
                  <Label htmlFor="codex-import-update" className="cursor-pointer">
                    {t("codexImport.updateExisting")}
                  </Label>
                  <p className="text-xs text-srapi-text-tertiary">{t("codexImport.updateExistingHint")}</p>
                </div>
                <Switch
                  id="codex-import-update"
                  checked={codexUpdateExisting}
                  onCheckedChange={setCodexUpdateExisting}
                  disabled={busy}
                />
              </div>
              {codexResult ? <CodexImportResultPanel result={codexResult} /> : null}
            </div>
          </TabsContent>

          <TabsContent value="oauth">
            <div className="space-y-4">
              <div>
                <Label htmlFor="oauth-import-provider">{t("codexImport.provider")}</Label>
                <Select value={oauthProviderId} onValueChange={setOAuthProviderId} disabled={busy}>
                  <SelectTrigger id="oauth-import-provider">
                    <SelectValue placeholder={t("codexImport.providerPlaceholder")} />
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
                <Label htmlFor="oauth-import-name">{t("codexImport.name")}</Label>
                <Input
                  id="oauth-import-name"
                  placeholder={t("codexImport.namePlaceholder")}
                  value={oauthName}
                  onChange={(e) => setOAuthName(e.target.value)}
                  disabled={busy}
                />
              </div>
              <div>
                <Label htmlFor="oauth-import-mode">{t("adminAccounts.authType")}</Label>
                <Select
                  value={oauthMode}
                  onValueChange={(value) => setOAuthMode(value as AccountOAuthFlowMode)}
                  disabled={busy}
                >
                  <SelectTrigger id="oauth-import-mode">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="authorization_code">
                      {t("adminAccounts.runtime.oauth_refresh")}
                    </SelectItem>
                    <SelectItem value="device_code">
                      {t("adminAccounts.runtime.oauth_device_code")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button
                type="button"
                variant="outline"
                disabled={!oauthProviderId || busy}
                onClick={() => setOAuthWizardOpen(true)}
              >
                <KeyRound className="size-3.5" />
                {t("accountOAuth.authorizeAccount")}
              </Button>
            </div>
          </TabsContent>
        </Tabs>

        {error ? (
          <p role="alert" className="mt-3 text-sm text-srapi-error">
            {error}
          </p>
        ) : null}

        <DialogFooter className="mt-6">
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
          {tab === "batch" ? (
            <Button
              type="button"
              variant="primary"
              loading={createAccountMut.isPending}
              disabled={!batchLines.trim() || !batchProviderId || busy}
              onClick={() => void submitBatch()}
            >
              {t("batchAdd.submit")}
            </Button>
          ) : tab === "json" ? (
            <Button
              type="button"
              variant="primary"
              loading={importMut.isPending}
              disabled={!json.trim() || busy}
              onClick={() => void submitJson()}
            >
              {t("adminAccounts.importSubmit")}
            </Button>
          ) : tab === "codex" ? (
            <Button
              type="button"
              variant="primary"
              loading={codexImportMut.isPending}
              disabled={!codexContent.trim() || !codexProviderId || busy}
              onClick={() => void submitCodex()}
            >
              {t("codexImport.submit")}
            </Button>
          ) : tab === "crs" ? (
            crsStep === "input" ? (
              <Button
                type="button"
                variant="primary"
                disabled={!crsUrl.trim() || !crsUser.trim() || !crsPass.trim() || busy}
                loading={crsSyncing}
                onClick={async () => {
                  setError(null);
                  setCrsSyncing(true);
                  try {
                    const preview = await adminApi.crsPreview({ base_url: crsUrl, username: crsUser, password: crsPass });
                    setCrsPreview(preview);
                    setCrsSelected(new Set(preview.new_accounts.map((a) => a.crs_account_id)));
                    setCrsStep("preview");
                  } catch (err) {
                    setError(adminErrorMessage(err));
                  } finally {
                    setCrsSyncing(false);
                  }
                }}
              >
                {t("crsSync.preview")}
              </Button>
            ) : crsStep === "preview" ? (
              <Button
                type="button"
                variant="primary"
                disabled={crsSelected.size === 0 || busy}
                loading={crsSyncing}
                onClick={async () => {
                  setError(null);
                  setCrsSyncing(true);
                  try {
                    const result = await adminApi.crsSync({
                      base_url: crsUrl,
                      username: crsUser,
                      password: crsPass,
                      selected_account_ids: [...crsSelected],
                    });
                    setCrsResult(result);
                    setCrsStep("result");
                    toast({
                      title: t("crsSync.done", { created: result.created, failed: result.failed }),
                      tone: result.failed > 0 ? "warning" : "success",
                    });
                  } catch (err) {
                    setError(adminErrorMessage(err));
                  } finally {
                    setCrsSyncing(false);
                  }
                }}
              >
                {t("crsSync.sync")} ({crsSelected.size})
              </Button>
            ) : null
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
    </>
  );
}

function ProviderImportResultPanel({ result }: { result: ProviderAccountImportResult }) {
  const { t } = useLanguage();
  return (
    <div className="space-y-3 rounded-md border border-srapi-border bg-srapi-card-muted p-3">
      <div className="grid grid-cols-4 gap-2 text-center">
        <ImportStat label={t("codexImport.created")} value={result.created_count} tone="success" />
        <ImportStat label={t("codexImport.updated")} value={result.updated_count} />
        <ImportStat label={t("codexImport.skipped")} value={result.skipped_count} />
        <ImportStat label={t("codexImport.failed")} value={result.failed_count} tone="error" />
      </div>
      {result.errors.length > 0 ? (
        <div>
          <p className="text-2xs font-medium text-srapi-text-secondary">
            {t("adminAccounts.importErrorsTitle")}
          </p>
          <ul className="mt-1 list-disc space-y-0.5 pl-4 text-2xs text-srapi-error">
            {result.errors.map((message, idx) => (
              <li key={idx}>{message}</li>
            ))}
          </ul>
        </div>
      ) : null}
      {result.warnings.length > 0 ? (
        <ul className="space-y-1 text-xs text-srapi-text-tertiary">
          {result.warnings.map((msg, idx) => (
            <li key={`warn-${idx}`}>
              #{msg.index}
              {msg.name ? ` ${msg.name}` : ""}: {msg.message}
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function ImportStat({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: "success" | "error";
}) {
  const toneClass =
    tone === "success"
      ? "text-srapi-success"
      : tone === "error"
        ? "text-srapi-error"
        : "text-srapi-text-primary";
  return (
    <div>
      <div className={`text-lg font-semibold ${toneClass}`}>{value}</div>
      <div className="text-xs text-srapi-text-tertiary">{label}</div>
    </div>
  );
}
