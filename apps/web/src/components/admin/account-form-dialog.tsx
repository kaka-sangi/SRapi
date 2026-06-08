"use client";

import { useCallback, useMemo, useState } from "react";
import { ChevronDown, KeyRound, Upload } from "lucide-react";
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
import {
  ACCOUNT_RUNTIME_CLASSES,
  ACCOUNT_STATUSES,
  emptyAccountForm,
  accountFormFromAccount,
  buildCreateAccountBody,
  buildUpdateAccountBody,
  type AdminAccountFormState,
} from "@/lib/admin-account-form";
import type { PlatformFamily, ProviderAccount } from "@/lib/sdk-types";

/** A provider entry enriched with the per-provider auth-method scoping data. */
export interface AccountProviderOption {
  value: string;
  label: string;
  platformFamily?: PlatformFamily | null;
  authMethods?: RuntimeClass[] | null;
  adapterType?: string;
  accountTemplate?: AccountTemplate | null;
}

interface AccountTemplate {
  upstream_client?: string;
  default_metadata?: Record<string, unknown>;
  model_catalog?: string[];
  metadata_hints?: Record<string, string>;
}

/**
 * Stable display order for the grouped provider dropdown — first-party families
 * first, then reverse-proxy and rerank. Families not listed here (or providers
 * carrying no platform_family) fall through to an unlabeled trailing group.
 */
const PLATFORM_FAMILY_ORDER: PlatformFamily[] = [
  "anthropic_compatible",
  "openai_compatible",
  "bedrock_anthropic",
  "reverse_proxy_antigravity",
  "rerank_compatible",
];

const CODEX_FALLBACK_TEMPLATE: AccountTemplate = {
  upstream_client: "codex_cli",
  default_metadata: { base_url: "https://chatgpt.com/backend-api/codex" },
  model_catalog: ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2", "codex-mini-latest"],
  metadata_hints: { base_url: "Codex upstream (adapter appends /responses)", chatgpt_account_id: "From session JWT (optional)" },
};

function getProviderTemplate(
  providerOptions: AccountProviderOption[],
  providerId: string,
): AccountTemplate | null {
  const p = providerOptions.find((o) => o.value === providerId);
  if (p?.accountTemplate) return p.accountTemplate;
  if (p?.adapterType === "reverse-proxy-codex-cli") return CODEX_FALLBACK_TEMPLATE;
  return null;
}

type RuntimeClass = AdminAccountFormState["runtimeClass"];

/**
 * Per-runtime credential UX. The admin enters friendly values — an API key, a
 * token, a cookie, or a couple of labeled token fields — never raw JSON, and we
 * assemble the credential object with the exact keys the backend's `injectAuth`
 * switch reads. OAuth runtimes get one labeled input per token; the
 * service-account runtime keeps a JSON box but adds a file-upload button so the
 * admin can drop in the downloaded `.json` rather than hand-type it.
 */
type CredKind = "password" | "textarea" | "json" | "fields";
interface CredFieldSpec {
  key: string; // credential object key the backend reads
  cred: string; // i18n suffix under adminAccounts.cred.* (…Label / …Hint)
  secret?: boolean; // render as a password input
}
interface CredSpec {
  kind: CredKind;
  credKey?: string;
  cred: string; // i18n suffix under adminAccounts.cred.*
  template?: string;
  /** kind "fields": one labeled input per credential key */
  fields?: CredFieldSpec[];
  /** kind "json": offer a ".json" file upload that fills the box */
  upload?: boolean;
}
const OAUTH_FIELDS: CredFieldSpec[] = [
  { key: "access_token", cred: "accessToken", secret: true },
  { key: "refresh_token", cred: "refreshToken", secret: true },
];
const CREDENTIAL_SPECS: Record<RuntimeClass, CredSpec> = {
  api_key: { kind: "password", credKey: "api_key", cred: "apiKey" },
  cli_client_token: { kind: "password", credKey: "access_token", cred: "accessToken" },
  desktop_client_token: { kind: "password", credKey: "access_token", cred: "accessToken" },
  ide_plugin_token: { kind: "password", credKey: "access_token", cred: "accessToken" },
  custom_reverse_proxy: { kind: "password", credKey: "access_token", cred: "accessToken" },
  web_session_cookie: { kind: "textarea", credKey: "cookie", cred: "cookie" },
  oauth_refresh: { kind: "fields", cred: "oauth", fields: OAUTH_FIELDS },
  oauth_device_code: { kind: "fields", cred: "oauth", fields: OAUTH_FIELDS },
  service_account_json: { kind: "json", cred: "serviceAccount", template: "{\n  \n}", upload: true },
};

function specFor(rc: RuntimeClass): CredSpec {
  return CREDENTIAL_SPECS[rc] ?? CREDENTIAL_SPECS.api_key;
}

// metadataStringList reads a metadata field as a list of trimmed, non-empty
// strings. Accepts an array (the canonical shape) or a comma-separated string
// (a legacy/typo-friendly shape the gateway also tolerates).
function metadataStringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value === "string") {
    return value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

function defaultCredInput(rc: RuntimeClass): string {
  const spec = specFor(rc);
  return spec.kind === "json" ? (spec.template ?? "{}") : "";
}

/** True when the admin has supplied a credential for the selected runtime. */
function hasCredential(rc: RuntimeClass, value: string, fields: Record<string, string>): boolean {
  const spec = specFor(rc);
  if (spec.kind === "fields") return (spec.fields ?? []).some((f) => fields[f.key]?.trim());
  return value.trim() !== "";
}

/** Assemble the credential JSON string consumed by buildCreate/UpdateAccountBody. */
function buildCredentialJson(
  rc: RuntimeClass,
  value: string,
  fields: Record<string, string>,
): string {
  const spec = specFor(rc);
  if (spec.kind === "json") return value; // raw JSON, validated downstream
  if (spec.kind === "fields") {
    const object: Record<string, string> = {};
    for (const f of spec.fields ?? []) {
      const v = fields[f.key]?.trim();
      if (v) object[f.key] = v;
    }
    return Object.keys(object).length ? JSON.stringify(object) : "";
  }
  const v = value.trim();
  return v ? JSON.stringify({ [spec.credKey as string]: v }) : "";
}

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

  const initial =
    mode === "edit" && target ? accountFormFromAccount(target) : emptyAccountForm(defaultProviderId);

  // Map provider id → its allowed auth methods. A provider with no allowlist
  // (legacy / manually-created) maps to the full set: no restriction.
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
  const groupedProviders = useMemo(() => {
    const byFamily = new Map<PlatformFamily, AccountProviderOption[]>();
    const ungrouped: AccountProviderOption[] = [];
    for (const opt of providerOptions) {
      if (opt.platformFamily) {
        const list = byFamily.get(opt.platformFamily) ?? [];
        list.push(opt);
        byFamily.set(opt.platformFamily, list);
      } else {
        ungrouped.push(opt);
      }
    }
    const groups: { family: PlatformFamily | null; options: AccountProviderOption[] }[] = [];
    for (const family of PLATFORM_FAMILY_ORDER) {
      const list = byFamily.get(family);
      if (list && list.length > 0) groups.push({ family, options: list });
    }
    for (const [family, list] of byFamily) {
      if (!PLATFORM_FAMILY_ORDER.includes(family) && list.length > 0) {
        groups.push({ family, options: list });
      }
    }
    if (ungrouped.length > 0) groups.push({ family: null, options: ungrouped });
    return groups;
  }, [providerOptions]);

  // On create, start from a runtime class the default provider actually accepts
  // (e.g. Antigravity has no api_key). On edit, keep the account's real class.
  const initialRuntimeClass: RuntimeClass =
    mode === "create" && !allowedFor(initial.providerId).includes(initial.runtimeClass)
      ? (allowedFor(initial.providerId)[0] ?? initial.runtimeClass)
      : initial.runtimeClass;

  const [providerId, setProviderId] = useState(initial.providerId);
  const [name, setName] = useState(initial.name);
  const [runtimeClass, setRuntimeClass] = useState<RuntimeClass>(initialRuntimeClass);
  const [credInput, setCredInput] = useState(defaultCredInput(initialRuntimeClass));
  const [credFields, setCredFields] = useState<Record<string, string>>({});
  const [status, setStatus] = useState(initial.status);
  const [priority, setPriority] = useState(initial.priority);
  const [weight, setWeight] = useState(initial.weight);
  const [upstreamClient, setUpstreamClient] = useState(initial.upstreamClient);
  const [metadata, setMetadata] = useState<Record<string, unknown>>(initial.metadata);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [oauthWizardOpen, setOauthWizardOpen] = useState(false);

  const template = useMemo(
    () => getProviderTemplate(providerOptions, providerId),
    [providerOptions, providerId],
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
  // other. model_mapping is a map; supported_models/excluded_models are lists.
  const modelMappingKey = "model_mapping";
  const supportedModelsKey = "supported_models";
  const excludedModelsKey = "excluded_models";
  const rawModelMapping = metadata[modelMappingKey];
  const modelMapping =
    rawModelMapping && typeof rawModelMapping === "object" && !Array.isArray(rawModelMapping)
      ? (rawModelMapping as Record<string, unknown>)
      : {};
  const supportedModels = metadataStringList(metadata[supportedModelsKey]);
  const excludedModels = metadataStringList(metadata[excludedModelsKey]);
  // The generic metadata editor shows everything except the dedicated keys.
  const metadataWithoutDedicated: Record<string, unknown> = { ...metadata };
  delete metadataWithoutDedicated[modelMappingKey];
  delete metadataWithoutDedicated[supportedModelsKey];
  delete metadataWithoutDedicated[excludedModelsKey];
  // Re-attach the dedicated keys (only when non-empty) onto a base object.
  const withDedicated = (base: Record<string, unknown>): Record<string, unknown> => {
    const next = { ...base };
    if (Object.keys(modelMapping).length > 0) next[modelMappingKey] = modelMapping;
    if (supportedModels.length > 0) next[supportedModelsKey] = supportedModels;
    if (excludedModels.length > 0) next[excludedModelsKey] = excludedModels;
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
  const updateStringListKey = (key: string, next: string[]) => {
    const base = withDedicated(metadataWithoutDedicated);
    if (next.length > 0) base[key] = next;
    else delete base[key];
    setMetadata(base);
  };

  // Auth methods the selected provider accepts. Always keep the current
  // selection visible so editing a legacy account never hides its real value.
  const availableRuntimeClasses = allowedFor(providerId);
  const runtimeClassOptions = availableRuntimeClasses.includes(runtimeClass)
    ? availableRuntimeClasses
    : [runtimeClass, ...availableRuntimeClasses];

  function changeRuntime(rc: RuntimeClass) {
    setRuntimeClass(rc);
    // Reset the credential inputs so they always match the selected auth type.
    setCredInput(defaultCredInput(rc));
    setCredFields({});
  }

  function changeProvider(id: string) {
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
      if (template.default_metadata) {
        setMetadata((prev) => {
          const next = { ...prev };
          for (const [k, v] of Object.entries(template.default_metadata!)) {
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

  async function onUploadCredential(file: File | null) {
    if (!file) return;
    setCredInput(await file.text());
  }

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (mode === "create" && !hasCredential(runtimeClass, credInput, credFields)) {
      setError(t("adminAccounts.credentialRequired"));
      return;
    }
    const formState: AdminAccountFormState = {
      providerId,
      name,
      runtimeClass,
      upstreamClient,
      credential: buildCredentialJson(runtimeClass, credInput, credFields),
      proxyId: "",
      status,
      priority,
      weight,
      metadata,
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

          <div className="mt-4 max-h-[62vh] space-y-4 overflow-y-auto pr-1">
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
                  <p className="mt-1.5 text-2xs text-srapi-text-tertiary">
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
                placeholder={t("adminAccounts.namePlaceholder")}
                disabled={busy}
                onChange={(e) => setName(e.target.value)}
              />
            </div>

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
                    <p className="mt-1 text-2xs text-srapi-text-tertiary">
                      {t("accountOAuth.authorizeAccountHint")}
                    </p>
                  ) : null}
                  <div className="mt-1.5 space-y-3">
                    {(spec.fields ?? []).map((f) => (
                      <div key={f.key}>
                        <Label
                          htmlFor={`cred-${f.key}`}
                          className="mb-1 text-2xs font-normal text-srapi-text-secondary"
                        >
                          {t(`adminAccounts.cred.${f.cred}Label`)}
                        </Label>
                        <Input
                          id={`cred-${f.key}`}
                          type={f.secret ? "password" : "text"}
                          autoComplete="off"
                          className="font-mono"
                          value={credFields[f.key] ?? ""}
                          disabled={busy}
                          onChange={(e) => setCredField(f.key, e.target.value)}
                        />
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
                    {spec.upload ? (
                      <label
                        htmlFor="cred-upload"
                        className={cn(
                          "inline-flex cursor-pointer items-center gap-1.5 rounded-lg border border-srapi-border px-2.5 py-1 text-2xs text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:text-srapi-text-primary",
                          busy && "pointer-events-none opacity-50",
                        )}
                      >
                        <Upload className="size-3.5" />
                        {t("adminAccounts.cred.uploadFile")}
                        <input
                          id="cred-upload"
                          type="file"
                          accept="application/json,.json"
                          className="sr-only"
                          disabled={busy}
                          onChange={(e) => {
                            void onUploadCredential(e.target.files?.[0] ?? null);
                            e.target.value = "";
                          }}
                        />
                      </label>
                    ) : null}
                  </div>
                  {spec.kind === "password" ? (
                    <Input
                      id={credId}
                      type="password"
                      autoComplete="off"
                      className="mt-1.5 font-mono"
                      placeholder={
                        spec.cred === "apiKey" ? t("adminAccounts.cred.apiKeyPlaceholder") : undefined
                      }
                      value={credInput}
                      disabled={busy}
                      onChange={(e) => setCredInput(e.target.value)}
                    />
                  ) : (
                    <Textarea
                      id={credId}
                      spellCheck={false}
                      className={cn("mt-1.5", spec.kind === "json" && "min-h-28 font-mono text-xs")}
                      value={credInput}
                      disabled={busy}
                      onChange={(e) => setCredInput(e.target.value)}
                    />
                  )}
                </>
              )}
              <p className="mt-1 text-2xs text-srapi-text-tertiary">{credHint}</p>
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
                  <p className="text-2xs text-srapi-text-tertiary">{t("adminAccounts.advancedHint")}</p>
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
                    <Label htmlFor="account-upstream">{t("adminAccounts.upstreamClient")}</Label>
                    <Input
                      id="account-upstream"
                      value={upstreamClient}
                      disabled={busy}
                      onChange={(e) => setUpstreamClient(e.target.value)}
                    />
                  </div>
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
                    <p className="mt-1 text-2xs text-srapi-text-tertiary">
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
                    <Label>{t("adminAccounts.supportedModels")}</Label>
                    <p className="mt-1 text-2xs text-srapi-text-tertiary">
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
                              className="rounded-md border border-srapi-border px-2 py-0.5 text-2xs text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:text-srapi-text-primary"
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
                    <p className="mt-1 text-2xs text-srapi-text-tertiary">
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
            <Button type="button" variant="ghost" disabled={busy} onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
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
