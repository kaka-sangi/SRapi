"use client";

import { useCallback, useMemo, useState } from "react";
import { ChevronDown, Eye, EyeOff, KeyRound, Sparkles, Zap } from "lucide-react";
import {
  AccountOAuthAuthorizeDialog,
  type AccountOAuthFlowMode,
  type ProvisionedTokens,
} from "@/components/admin/account-oauth-authorize-dialog";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { KeyValueEditor } from "@/components/ui/key-value-editor";
import { TagInput } from "@/components/ui/tag-input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useTlsProfiles, useTestAccount } from "@/hooks/admin-queries";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
} from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { cn } from "@/lib/cn";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatGatewayHintLine } from "@/lib/gateway-error-hint";
import {
  ACCOUNT_RUNTIME_CLASSES,
  ACCOUNT_RISK_LEVELS,
  ACCOUNT_STATUSES,
  emptyAccountForm,
  accountFormFromAccount,
  buildCreateAccountBody,
  buildUpdateAccountBody,
  type AdminAccountFormState,
} from "@/lib/admin-account-form";
import type { ProviderAccount } from "@/lib/sdk-types";
import {
  buildCredentialJson,
  buildDefaultAccountName,
  credentialNameSeed,
  credentialFieldsFromPaste,
  defaultCredInput,
  getProviderTemplate,
  groupProviders,
  hasCredential,
  metadataStringList,
  providerLabelFor,
  specFor,
  type AccountProviderOption,
  type RuntimeClass,
} from "@/components/admin/account-form-helpers";

// Re-exported so existing importers of the public type surface keep working.
export type { AccountProviderOption } from "@/components/admin/account-form-helpers";

export function AccountFormDialog({
  open,
  onOpenChange,
  mode,
  target,
  providerOptions,
  defaultProviderId,
  submit,
  isPending,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: "create" | "edit";
  target?: ProviderAccount;
  providerOptions: AccountProviderOption[];
  defaultProviderId: string;
  submit: (
    body: ReturnType<typeof buildCreateAccountBody> | ReturnType<typeof buildUpdateAccountBody>,
  ) => Promise<unknown>;
  isPending?: boolean;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const tlsProfiles = useTlsProfiles();
  const testMut = useTestAccount();

  const initial =
    mode === "edit" && target ? accountFormFromAccount(target) : emptyAccountForm(defaultProviderId);

  // Map provider id to its accepted auth methods. If the backend omits the
  // allowlist, expose the canonical runtime set.
  const allowedFor = useCallback(
    (id: string): RuntimeClass[] => {
      const match = providerOptions.find((opt) => opt.value === id);
      const methods = match?.authMethods;
      return methods && methods.length > 0 ? methods : ACCOUNT_RUNTIME_CLASSES;
    },
    [providerOptions],
  );

  // Group providers by platform family for the dropdown (sub2api-style), in a
  // stable family order. Providers without a family fall into a trailing group.
  const groupedProviders = useMemo(() => groupProviders(providerOptions), [providerOptions]);

  // Start from a runtime class the selected provider actually accepts.
  const initialRuntimeClass: RuntimeClass =
    !allowedFor(initial.providerId).includes(initial.runtimeClass)
      ? (allowedFor(initial.providerId)[0] ?? initial.runtimeClass)
      : initial.runtimeClass;
  const initialTemplate =
    mode === "create" ? getProviderTemplate(providerOptions, initial.providerId) : null;
  const initialMetadata = { ...initial.metadata };
  if (mode === "create" && initialTemplate?.default_metadata) {
    for (const [key, value] of Object.entries(initialTemplate.default_metadata)) {
      if (!(key in initialMetadata)) initialMetadata[key] = value;
    }
  }

  const [providerId, setProviderId] = useState(initial.providerId);
  const [name, setName] = useState(initial.name);
  const [runtimeClass, setRuntimeClass] = useState<RuntimeClass>(initialRuntimeClass);
  const [credInput, setCredInput] = useState(defaultCredInput(initialRuntimeClass));
  const [credFields, setCredFields] = useState<Record<string, string>>({});
  const [status, setStatus] = useState(initial.status);
  const [riskLevel, setRiskLevel] = useState(initial.riskLevel);
  const [priority, setPriority] = useState(initial.priority);
  const [weight, setWeight] = useState(initial.weight);
  const [upstreamClient, setUpstreamClient] = useState(
    initial.upstreamClient || initialTemplate?.upstream_client || "",
  );
  const [baseUrl, setBaseUrl] = useState(
    typeof initialMetadata.base_url === "string" ? (initialMetadata.base_url as string) : "",
  );
  const [metadata, setMetadata] = useState<Record<string, unknown>>(() => {
    const m = { ...initialMetadata };
    delete m.base_url;
    return m;
  });
  const [createAnother, setCreateAnother] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [credVisible, setCredVisible] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [oauthWizardOpen, setOauthWizardOpen] = useState(false);
  const [quickOAuthToken, setQuickOAuthToken] = useState("");

  const template = useMemo(
    () => getProviderTemplate(providerOptions, providerId),
    [providerOptions, providerId],
  );
  const metadataHints = useMemo(
    () => ({ tls_profile: t("adminAccounts.tlsProfileMetadataHint"), ...(template?.metadata_hints ?? {}) }),
    [t, template],
  );
  const enabledTlsProfiles = useMemo(
    () => (tlsProfiles.data?.data ?? []).filter((profile) => profile.enabled),
    [tlsProfiles.data],
  );

  // Map the OAuth runtime class to its provisioning flow: refresh-token runtimes
  // run the redirect/PKCE authorization-code flow; device-code runtimes run the
  // RFC 8628 device flow.
  const oauthFlowMode: AccountOAuthFlowMode | null =
    runtimeClass === "oauth_refresh"
      ? "authorization_code"
      : runtimeClass === "oauth_device_code"
        ? "device_code"
        : null;

  function applyProvisionedTokens(tokens: ProvisionedTokens) {
    setCredFields((prev) => ({
      ...prev,
      access_token: tokens.accessToken || prev.access_token || "",
      refresh_token: tokens.refreshToken || prev.refresh_token || "",
    }));
  }

  const spec = specFor(runtimeClass);
  const busy = submitting || Boolean(isPending);

  // These keys live inside metadata but each gets a dedicated editor below; keep
  // them out of the generic metadata editor so the editors never overwrite each
  // other. model_mapping/compact_model_mapping are maps; supported_models/excluded_models are lists.
  const modelMappingKey = "model_mapping";
  const compactModelMappingKey = "compact_model_mapping";
  const supportedModelsKey = "supported_models";
  const excludedModelsKey = "excluded_models";
  const tlsProfileKey = "tls_profile";
  const rawModelMapping = metadata[modelMappingKey];
  const modelMapping =
    rawModelMapping && typeof rawModelMapping === "object" && !Array.isArray(rawModelMapping)
      ? (rawModelMapping as Record<string, unknown>)
      : {};
  const rawCompactModelMapping = metadata[compactModelMappingKey];
  const compactModelMapping =
    rawCompactModelMapping && typeof rawCompactModelMapping === "object" && !Array.isArray(rawCompactModelMapping)
      ? (rawCompactModelMapping as Record<string, unknown>)
      : {};
  const supportedModels = metadataStringList(metadata[supportedModelsKey]);
  const excludedModels = metadataStringList(metadata[excludedModelsKey]);
  const selectedTlsProfile =
    typeof metadata[tlsProfileKey] === "string" ? String(metadata[tlsProfileKey]).trim() : "";
  const tlsProfileEnabled = selectedTlsProfile.length > 0;
  // The generic metadata editor shows everything except the dedicated keys.
  const metadataWithoutDedicated: Record<string, unknown> = { ...metadata };
  delete metadataWithoutDedicated[modelMappingKey];
  delete metadataWithoutDedicated[compactModelMappingKey];
  delete metadataWithoutDedicated[supportedModelsKey];
  delete metadataWithoutDedicated[excludedModelsKey];
  delete metadataWithoutDedicated[tlsProfileKey];
  // Re-attach the dedicated keys (only when non-empty) onto a base object.
  const withDedicated = (base: Record<string, unknown>): Record<string, unknown> => {
    const next = { ...base };
    if (Object.keys(modelMapping).length > 0) next[modelMappingKey] = modelMapping;
    if (Object.keys(compactModelMapping).length > 0) next[compactModelMappingKey] = compactModelMapping;
    if (supportedModels.length > 0) next[supportedModelsKey] = supportedModels;
    if (excludedModels.length > 0) next[excludedModelsKey] = excludedModels;
    if (selectedTlsProfile) next[tlsProfileKey] = selectedTlsProfile;
    return next;
  };
  const updateMetadataFields = (next: Record<string, unknown>) => setMetadata(withDedicated(next));
  const updateModelMapping = (next: Record<string, unknown>) =>
    setMetadata(
      Object.keys(next).length > 0
        ? { ...withDedicated(metadataWithoutDedicated), [modelMappingKey]: next }
        : (() => {
            const base = withDedicated(metadataWithoutDedicated);
            delete base[modelMappingKey];
            return base;
          })(),
    );
  const updateCompactModelMapping = (next: Record<string, unknown>) =>
    setMetadata(
      Object.keys(next).length > 0
        ? { ...withDedicated(metadataWithoutDedicated), [compactModelMappingKey]: next }
        : (() => {
            const base = withDedicated(metadataWithoutDedicated);
            delete base[compactModelMappingKey];
            return base;
          })(),
    );
  const updateStringListKey = (key: string, next: string[]) => {
    const base = withDedicated(metadataWithoutDedicated);
    if (next.length > 0) base[key] = next;
    else delete base[key];
    setMetadata(base);
  };
  const updateTlsProfile = (profileName: string) => {
    const base = withDedicated(metadataWithoutDedicated);
    if (profileName.trim()) base[tlsProfileKey] = profileName.trim();
    else delete base[tlsProfileKey];
    setMetadata(base);
  };

  const runtimeClassOptions = allowedFor(providerId);
  const providerLabel = providerLabelFor(providerOptions, providerId);
  const defaultName = buildDefaultAccountName(
    providerLabel,
    credentialNameSeed(runtimeClass, credInput, credFields),
  );

  function changeRuntime(rc: RuntimeClass) {
    setRuntimeClass(rc);
    // Reset the credential inputs so they always match the selected auth type.
    setCredInput(defaultCredInput(rc));
    setCredFields({});
    setQuickOAuthToken("");
  }

  function changeProvider(id: string) {
    const previousTemplate = getProviderTemplate(providerOptions, providerId);
    const previousBaseUrl =
      typeof previousTemplate?.default_metadata?.base_url === "string"
        ? (previousTemplate.default_metadata.base_url as string)
        : "";
    setProviderId(id);
    // Clamp the auth method to one the newly-selected provider accepts.
    const methods = allowedFor(id);
    if (!methods.includes(runtimeClass)) {
      changeRuntime(methods[0] ?? "api_key");
    }
    // Auto-fill from provider template when creating a new account.
    const template = getProviderTemplate(providerOptions, id);
    if (template && mode === "create") {
      if (template.upstream_client) setUpstreamClient(template.upstream_client);
      const nextBaseUrl =
        typeof template.default_metadata?.base_url === "string"
          ? (template.default_metadata.base_url as string)
          : "";
      setBaseUrl((prev) => (!prev || prev === previousBaseUrl ? nextBaseUrl : prev));
      if (template.default_metadata) {
        const dm = { ...template.default_metadata };
        delete dm.base_url;
        setMetadata((prev) => {
          const next = { ...prev };
          for (const [k, v] of Object.entries(dm)) {
            if (!(k in next)) next[k] = v;
          }
          return next;
        });
      }
      setAdvancedOpen(true);
    }
  }

  function setCredField(key: string, value: string) {
    setCredFields((prev) => ({ ...prev, [key]: value }));
  }

  function applyPastedCredential(value: string, preferredPlainTextField = "access_token") {
    const parsed = credentialFieldsFromPaste(value, preferredPlainTextField);
    if (Object.keys(parsed.fields).length === 0 && Object.keys(parsed.metadata).length === 0) {
      return false;
    }
    if (Object.keys(parsed.fields).length > 0) {
      setCredFields((prev) => ({ ...prev, ...parsed.fields }));
    }
    applyPastedMetadata(parsed.metadata);
    if (mode === "create" && parsed.name && !name.trim()) {
      setName(parsed.name);
    }
    return true;
  }

  function applyQuickOAuthToken(value: string) {
    if (!applyPastedCredential(value, "refresh_token")) {
      return false;
    }
    setQuickOAuthToken("");
    return true;
  }

  function applyPastedSingleCredential(value: string) {
    if (!value.trim().startsWith("{")) {
      return false;
    }
    const parsed = credentialFieldsFromPaste(value);
    const key = spec.credKey;
    if (!key || !parsed.fields[key]) {
      return applyPastedCredential(value);
    }
    setCredInput(parsed.fields[key]);
    applyPastedMetadata(parsed.metadata);
    if (mode === "create" && parsed.name && !name.trim()) {
      setName(parsed.name);
    }
    return true;
  }

  function applyPastedMetadata(pasted: Record<string, unknown>) {
    if (Object.keys(pasted).length === 0) return;
    const nextMetadata = { ...pasted };
    const pastedBaseUrl =
      typeof nextMetadata.base_url === "string" ? String(nextMetadata.base_url).trim() : "";
    delete nextMetadata.base_url;
    if (pastedBaseUrl) {
      setBaseUrl((prev) => prev || pastedBaseUrl);
    }
    setMetadata((prev) => ({ ...nextMetadata, ...prev }));
  }

  function resetCredentialForNextAccount() {
    setName("");
    setCredInput(defaultCredInput(runtimeClass));
    setCredFields({});
    setQuickOAuthToken("");
    setCredVisible(false);
    setError(null);
  }

  function handleTest() {
    if (!target) return;
    testMut.mutate(
      { id: target.id, body: { mode: "live" } },
      {
        onSuccess: (result) => {
          const hint = result?.ok ? null : formatGatewayHintLine(result?.message, t);
          toast({
            title: result?.ok ? t("adminAccounts.testOk") : t("adminAccounts.testFailed"),
            description:
              [result?.message, hint]
                .filter(Boolean)
                .join(" — ") || undefined,
            tone: result?.ok ? "success" : "error",
          });
        },
        onError: () => toast({ title: t("adminAccounts.testFailed"), tone: "error" }),
      },
    );
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (mode === "create" && !hasCredential(runtimeClass, credInput, credFields)) {
      setError(t("adminAccounts.credentialRequired"));
      return;
    }
    const finalMetadata = { ...metadata };
    if (baseUrl.trim()) finalMetadata.base_url = baseUrl.trim();
    const credentialSeed = credentialNameSeed(runtimeClass, credInput, credFields);
    const finalName =
      name.trim() || (mode === "create" ? buildDefaultAccountName(providerLabel, credentialSeed) : "");
    const formState: AdminAccountFormState = {
      providerId,
      name: finalName,
      runtimeClass,
      upstreamClient,
      credential: buildCredentialJson(runtimeClass, credInput, credFields),
      proxyId: "",
      status,
      riskLevel,
      priority,
      weight,
      metadata: finalMetadata,
      groupIds: [],
    };
    let body;
    try {
      body =
        mode === "create" ? buildCreateAccountBody(formState) : buildUpdateAccountBody(formState);
    } catch (err) {
      setError(adminErrorMessage(err));
      return;
    }
    setSubmitting(true);
    try {
      await submit(body);
      toast({
        title: t(mode === "create" ? "feedback.created" : "feedback.updated"),
        tone: "success",
      });
      if (mode === "create" && createAnother) {
        resetCredentialForNextAccount();
        return;
      }
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setSubmitting(false);
    }
  }

  const credLabel = t(`adminAccounts.cred.${spec.cred}Label`);
  const credHint =
    mode === "edit" ? t("adminAccounts.cred.editBlankHint") : t(`adminAccounts.cred.${spec.cred}Hint`);
  const credId = "account-credential";

  return (
    <>
      {oauthFlowMode ? (
        <AccountOAuthAuthorizeDialog
          open={oauthWizardOpen}
          onOpenChange={setOauthWizardOpen}
          mode={oauthFlowMode}
          providerId={providerId}
          onProvisioned={applyProvisionedTokens}
        />
      ) : null}
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent>
          <form onSubmit={onSubmit}>
          <DialogHeader>
            <DialogTitle>
              {mode === "create" ? t("adminAccounts.create") : t("adminAccounts.edit")}
            </DialogTitle>
            {mode === "edit" && target ? (
              <DialogDescription>{target.name}</DialogDescription>
            ) : null}
          </DialogHeader>

          <div className="mt-4 max-h-[62vh] space-y-5 overflow-y-auto pr-1">
            {/* ── Section: Identity ── */}
            {mode === "create" ? (
              <div>
                <Label htmlFor="account-provider">{t("adminAccounts.provider")}</Label>
                <Select value={providerId} onValueChange={changeProvider} disabled={busy}>
                  <SelectTrigger id="account-provider">
                    <SelectValue placeholder={t("adminAccounts.provider")} />
                  </SelectTrigger>
                  <SelectContent>
                    {groupedProviders.map((group) => (
                      <SelectGroup key={group.family ?? "_ungrouped"}>
                        {group.family ? (
                          <SelectLabel>{t(`adminAccounts.platform.${group.family}`)}</SelectLabel>
                        ) : null}
                        {group.options.map((opt) => (
                          <SelectItem key={opt.value} value={opt.value}>
                            {opt.label}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    ))}
                  </SelectContent>
                </Select>
                {template ? (
                  <p className="mt-1.5 text-[11px] text-srapi-text-tertiary">
                    {t("adminAccounts.templateApplied")}
                  </p>
                ) : null}
              </div>
            ) : null}

            <div>
              <Label htmlFor="account-name">{t("adminAccounts.name")}</Label>
              <Input
                id="account-name"
                value={name}
                placeholder={mode === "create" ? defaultName : t("adminAccounts.namePlaceholder")}
                disabled={busy}
                onChange={(e) => setName(e.target.value)}
              />
            </div>

            {/* ── Section: Authentication ── */}
            <div className="space-y-4 border-t border-srapi-border pt-4">
              <div>
                <Label htmlFor="account-runtime">{t("adminAccounts.authType")}</Label>
                <Select
                  value={runtimeClass}
                  onValueChange={(v) => changeRuntime(v as RuntimeClass)}
                  disabled={busy}
                >
                  <SelectTrigger id="account-runtime">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {runtimeClassOptions.map((rc) => (
                      <SelectItem key={rc} value={rc}>
                        {t(`adminAccounts.runtime.${rc}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

            <div>
              {spec.kind === "fields" ? (
                <>
                  <div className="flex items-center justify-between gap-2">
                    <Label className="mb-0">{credLabel}</Label>
                    {oauthFlowMode ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={busy}
                        onClick={() => setOauthWizardOpen(true)}
                      >
                        <KeyRound className="size-3.5" />
                        {t("accountOAuth.authorizeAccount")}
                      </Button>
                    ) : null}
                  </div>
                  {oauthFlowMode ? (
                    <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                      {t("accountOAuth.authorizeAccountHint")}
                    </p>
                  ) : null}
                  {runtimeClass === "oauth_refresh" ? (
                    <div className="mt-3 rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3">
                      <Label htmlFor="oauth-token-quick-paste" className="text-xs">
                        {t("accountOAuth.quickPasteLabel")}
                      </Label>
                      <div className="mt-1.5 flex flex-col gap-2 sm:flex-row">
                        <Textarea
                          id="oauth-token-quick-paste"
                          spellCheck={false}
                          className="min-h-20 flex-1 font-mono text-xs"
                          value={quickOAuthToken}
                          disabled={busy}
                          autoComplete="off"
                          data-lpignore="true"
                          data-1p-ignore="true"
                          placeholder={t("accountOAuth.quickPastePlaceholder")}
                          onPaste={(e) => {
                            const text = e.clipboardData.getData("text");
                            if (applyQuickOAuthToken(text)) {
                              e.preventDefault();
                            }
                          }}
                          onChange={(e) => setQuickOAuthToken(e.target.value)}
                        />
                        <Button
                          type="button"
                          variant="outline"
                          disabled={busy || !quickOAuthToken.trim()}
                          onClick={() => applyQuickOAuthToken(quickOAuthToken)}
                          className="sm:self-end"
                        >
                          <Sparkles className="size-3.5" />
                          {t("accountOAuth.quickPasteApply")}
                        </Button>
                      </div>
                      <p className="mt-1.5 text-[11px] text-srapi-text-tertiary">
                        {t("accountOAuth.quickPasteHint")}
                      </p>
                    </div>
                  ) : null}
                  <div className="mt-1.5 space-y-3">
                    {(spec.fields ?? []).map((f) => (
                      <div key={f.key}>
                        <Label
                          htmlFor={`cred-${f.key}`}
                          className="mb-1 text-[11px] font-normal text-srapi-text-secondary"
                        >
                          {t(`adminAccounts.cred.${f.cred}Label`)}
                        </Label>
                        <div className="relative">
                          <Input
                            id={`cred-${f.key}`}
                            type={f.secret && !credVisible ? "password" : "text"}
                            autoComplete="new-password"
                            data-lpignore="true"
                            data-1p-ignore="true"
                            className={cn("font-mono", f.secret && "pr-9")}
                            value={credFields[f.key] ?? ""}
                            disabled={busy}
                            onPaste={(e) => {
                              const text = e.clipboardData.getData("text");
                              if (applyPastedCredential(text, f.key)) {
                                e.preventDefault();
                              }
                            }}
                            onChange={(e) => setCredField(f.key, e.target.value)}
                          />
                          {f.secret ? (
                            <button
                              type="button"
                              tabIndex={-1}
                              onClick={() => setCredVisible((v) => !v)}
                              className="absolute right-2 top-1/2 -translate-y-1/2 text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
                            >
                              {credVisible ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                            </button>
                          ) : null}
                        </div>
                      </div>
                    ))}
                  </div>
                </>
              ) : (
                <>
                  <div className="flex items-center justify-between gap-2">
                    <Label htmlFor={credId} className="mb-0">
                      {credLabel}
                    </Label>
                  </div>
                  {spec.kind === "password" ? (
                    <div className="relative mt-1.5">
                      <Input
                        id={credId}
                        type={credVisible ? "text" : "password"}
                        autoComplete="new-password"
                        data-lpignore="true"
                        data-1p-ignore="true"
                        className="pr-9 font-mono"
                        placeholder={
                          spec.cred === "apiKey" ? t("adminAccounts.cred.apiKeyPlaceholder") : undefined
                        }
                        value={credInput}
                        disabled={busy}
                        onPaste={(e) => {
                          const text = e.clipboardData.getData("text");
                          if (applyPastedSingleCredential(text)) {
                            e.preventDefault();
                          }
                        }}
                        onChange={(e) => setCredInput(e.target.value)}
                      />
                      <button
                        type="button"
                        tabIndex={-1}
                        onClick={() => setCredVisible((v) => !v)}
                        className="absolute right-2 top-1/2 -translate-y-1/2 text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
                      >
                        {credVisible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                      </button>
                    </div>
                  ) : (
                    <Textarea
                      id={credId}
                      spellCheck={false}
                      className={cn("mt-1.5", spec.kind === "json" && "min-h-28 font-mono text-xs")}
                      value={credInput}
                      disabled={busy}
                      autoComplete="off"
                      data-lpignore="true"
                      data-1p-ignore="true"
                      onChange={(e) => setCredInput(e.target.value)}
                    />
                  )}
                </>
              )}
              <p className="mt-1 text-[11px] text-srapi-text-tertiary">{credHint}</p>
            </div>
            </div>

            {/* ── Section: Endpoint ── */}
            <div>
              <Label htmlFor="account-base-url">{t("adminAccounts.baseUrl")}</Label>
              <Input
                id="account-base-url"
                type="url"
                className="mt-1.5 font-mono"
                placeholder={t("adminAccounts.baseUrlPlaceholder")}
                value={baseUrl}
                disabled={busy}
                onChange={(e) => setBaseUrl(e.target.value)}
              />
              <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                {t("adminAccounts.baseUrlHint")}
              </p>
            </div>

            {/* Advanced — everything an admin rarely touches, collapsed by default. */}
            <div className="rounded-lg border border-srapi-border">
              <button
                type="button"
                onClick={() => setAdvancedOpen((v) => !v)}
                className="flex w-full items-center justify-between px-3.5 py-2.5 text-left"
                aria-expanded={advancedOpen}
              >
                <span className="text-sm text-srapi-text-secondary">{t("adminAccounts.advanced")}</span>
                <ChevronDown
                  className={cn(
                    "size-4 text-srapi-text-tertiary transition-transform",
                    advancedOpen && "rotate-180",
                  )}
                />
              </button>
              {advancedOpen ? (
                <div className="space-y-4 border-t border-srapi-border px-3.5 py-4">
                  <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.advancedHint")}</p>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label htmlFor="account-priority">{t("adminAccounts.priority")}</Label>
                      <Input
                        id="account-priority"
                        type="number"
                        value={priority}
                        disabled={busy}
                        onChange={(e) => setPriority(e.target.value)}
                      />
                    </div>
                    <div>
                      <Label htmlFor="account-weight">{t("adminAccounts.weight")}</Label>
                      <Input
                        id="account-weight"
                        type="number"
                        value={weight}
                        disabled={busy}
                        onChange={(e) => setWeight(e.target.value)}
                      />
                    </div>
                  </div>
                  <div>
                    <Label htmlFor="account-status">{t("adminCommon.status")}</Label>
                    <Select
                      value={status}
                      onValueChange={(v) => setStatus(v as typeof status)}
                      disabled={busy}
                    >
                      <SelectTrigger id="account-status">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {ACCOUNT_STATUSES.map((s) => (
                          <SelectItem key={s} value={s}>
                            {s}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div>
                    <Label htmlFor="account-risk-level">{t("adminAccounts.riskLevel")}</Label>
                    <Select
                      value={riskLevel}
                      onValueChange={(v) => setRiskLevel(v as typeof riskLevel)}
                      disabled={busy}
                    >
                      <SelectTrigger id="account-risk-level">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {ACCOUNT_RISK_LEVELS.map((level) => (
                          <SelectItem key={level} value={level}>
                            {t(`adminAccounts.risk.${level}`)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div>
                    <Label htmlFor="account-upstream">{t("adminAccounts.upstreamClient")}</Label>
                    <Input
                      id="account-upstream"
                      value={upstreamClient}
                      disabled={busy}
                      onChange={(e) => setUpstreamClient(e.target.value)}
                    />
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <Switch
                        id="account-tls-profile-enabled"
                        checked={tlsProfileEnabled}
                        disabled={busy || enabledTlsProfiles.length === 0}
                        onCheckedChange={(checked) =>
                          updateTlsProfile(
                            checked ? (selectedTlsProfile || enabledTlsProfiles[0]?.name || "") : "",
                          )
                        }
                      />
                      <Label htmlFor="account-tls-profile-enabled" className="mb-0">
                        {t("adminAccounts.tlsProfile")}
                      </Label>
                    </div>
                    <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                      {t("adminAccounts.tlsProfileHint")}
                    </p>
                    {tlsProfileEnabled ? (
                      <Select
                        value={selectedTlsProfile || enabledTlsProfiles[0]?.name}
                        onValueChange={updateTlsProfile}
                        disabled={busy || enabledTlsProfiles.length === 0}
                      >
                        <SelectTrigger id="account-tls-profile" className="mt-1.5">
                          <SelectValue placeholder={t("adminAccounts.tlsProfilePlaceholder")} />
                        </SelectTrigger>
                        <SelectContent>
                          {enabledTlsProfiles.map((profile) => (
                            <SelectItem key={profile.id} value={profile.name}>
                              {profile.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    ) : null}
                  </div>
                  {Object.keys(metadataHints).length > 0 ? (
                    <div>
                      <Label>{t("adminAccounts.metadataHints")}</Label>
                      <div className="mt-1.5 space-y-1 text-[11px] text-srapi-text-tertiary">
                        {Object.entries(metadataHints).map(([key, hint]) => (
                          <p key={key}>
                            <span className="font-mono text-srapi-text-secondary">{key}</span>: {hint}
                          </p>
                        ))}
                      </div>
                    </div>
                  ) : null}
                  <div>
                    <Label>{t("adminCommon.metadata")}</Label>
                    <div className="mt-1.5">
                      <KeyValueEditor
                        value={metadataWithoutDedicated}
                        onChange={updateMetadataFields}
                        disabled={busy}
                        addLabel={t("adminCommon.addField")}
                      />
                    </div>
                  </div>
                  <div>
                    <Label>{t("adminAccounts.modelMapping")}</Label>
                    <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                      {t("adminAccounts.modelMappingHint")}
                    </p>
                    <div className="mt-1.5">
                      <KeyValueEditor
                        value={modelMapping}
                        onChange={updateModelMapping}
                        disabled={busy}
                        keyPlaceholder={t("adminAccounts.modelMappingKeyPlaceholder")}
                        valuePlaceholder={t("adminAccounts.modelMappingValuePlaceholder")}
                        addLabel={t("adminAccounts.addModelMapping")}
                      />
                    </div>
                  </div>
                  <div>
                    <Label>{t("adminAccounts.compactModelMapping")}</Label>
                    <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                      {t("adminAccounts.compactModelMappingHint")}
                    </p>
                    <div className="mt-1.5">
                      <KeyValueEditor
                        value={compactModelMapping}
                        onChange={updateCompactModelMapping}
                        disabled={busy}
                        keyPlaceholder={t("adminAccounts.modelMappingKeyPlaceholder")}
                        valuePlaceholder={t("adminAccounts.modelMappingValuePlaceholder")}
                        addLabel={t("adminAccounts.addCompactModelMapping")}
                      />
                    </div>
                  </div>
                  <div>
                    <Label>{t("adminAccounts.supportedModels")}</Label>
                    <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                      {t("adminAccounts.supportedModelsHint")}
                    </p>
                    <div className="mt-1.5">
                      <TagInput
                        value={supportedModels}
                        onChange={(next) => updateStringListKey(supportedModelsKey, next)}
                        disabled={busy}
                        placeholder={t("adminAccounts.supportedModelsPlaceholder")}
                      />
                    </div>
                    {template?.model_catalog?.length ? (
                      <div className="mt-1.5 flex flex-wrap gap-1">
                        {template.model_catalog
                          .filter((m) => !supportedModels.includes(m))
                          .map((m) => (
                            <button
                              key={m}
                              type="button"
                              className="rounded-full border border-srapi-border px-2.5 py-0.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:border-srapi-border-strong hover:bg-srapi-card-muted hover:text-srapi-text-primary"
                              disabled={busy}
                              onClick={() => updateStringListKey(supportedModelsKey, [...supportedModels, m])}
                            >
                              + {m}
                            </button>
                          ))}
                      </div>
                    ) : null}
                  </div>
                  <div>
                    <Label>{t("adminAccounts.excludedModels")}</Label>
                    <p className="mt-1 text-[11px] text-srapi-text-tertiary">
                      {t("adminAccounts.excludedModelsHint")}
                    </p>
                    <div className="mt-1.5">
                      <TagInput
                        value={excludedModels}
                        onChange={(next) => updateStringListKey(excludedModelsKey, next)}
                        disabled={busy}
                        placeholder={t("adminAccounts.excludedModelsPlaceholder")}
                      />
                    </div>
                  </div>
                </div>
              ) : null}
            </div>

            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>

          <DialogFooter className="mt-6">
            {mode === "create" ? (
              <label className="mr-auto flex items-center gap-2 text-xs text-srapi-text-secondary">
                <Switch
                  checked={createAnother}
                  onCheckedChange={setCreateAnother}
                  disabled={busy}
                />
                {t("adminAccounts.createAnother")}
              </label>
            ) : null}
            <Button type="button" variant="ghost" disabled={busy} onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            {mode === "edit" && target ? (
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                loading={testMut.isPending}
                onClick={handleTest}
              >
                <Zap className="size-3.5" />
                {t("adminAccounts.test")}
              </Button>
            ) : null}
            <Button type="submit" variant="primary" loading={busy}>
              {t("common.save")}
            </Button>
          </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
