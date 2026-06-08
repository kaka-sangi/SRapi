"use client";

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  CheckCircle2,
  AlertTriangle,
  ChevronLeft,
  ChevronDown,
  Settings2,
} from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAdminProxies } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { cn } from "@/lib/cn";
import { ADMIN_ROUTES } from "@/lib/routes";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface QuickSetupBody {
  platform: string;
  credential: Record<string, string>;
  name?: string;
  runtime_class?: string;
  model_catalog?: string[];
  proxy_id?: string;
  priority?: number;
  weight?: number;
}

interface QuickSetupResult {
  provider: Record<string, unknown>;
  account: Record<string, unknown>;
  models_created: number;
  mappings_created: number;
  model_names?: string[];
  warnings?: string[];
}

// ---------------------------------------------------------------------------
// Platform presets (mirrors backend provider/preset registry)
// ---------------------------------------------------------------------------

type AuthType = "api_key" | "oauth_refresh";

interface PlatformPreset {
  key: string;
  name: string;
  description: string;
  authTypes: AuthType[];
  defaultModels: string[];
}

const PLATFORMS: PlatformPreset[] = [
  {
    key: "codex-cli",
    name: "Codex CLI",
    description: "OpenAI Codex via ChatGPT backend",
    authTypes: ["oauth_refresh"],
    defaultModels: [
      "gpt-5.5",
      "gpt-5.4",
      "gpt-5.4-mini",
      "gpt-5.3-codex",
      "gpt-5.3-codex-spark",
      "gpt-5.2",
      "codex-mini-latest",
    ],
  },
  {
    key: "openai",
    name: "OpenAI",
    description: "GPT / o-series via API key or OAuth",
    authTypes: ["api_key", "oauth_refresh"],
    defaultModels: [
      "gpt-5.5",
      "gpt-5.4",
      "gpt-5.4-mini",
      "gpt-4.1",
      "gpt-4.1-mini",
      "gpt-4.1-nano",
      "o4-mini",
      "o3",
      "o3-pro",
    ],
  },
  {
    key: "anthropic",
    name: "Anthropic",
    description: "Claude Opus / Sonnet / Haiku",
    authTypes: ["api_key"],
    defaultModels: ["claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5"],
  },
  {
    key: "deepseek",
    name: "DeepSeek",
    description: "DeepSeek R1 / V3 / Coder",
    authTypes: ["api_key"],
    defaultModels: [],
  },
  {
    key: "groq",
    name: "Groq",
    description: "Ultra-fast inference via Groq Cloud",
    authTypes: ["api_key"],
    defaultModels: [],
  },
  {
    key: "mistral",
    name: "Mistral",
    description: "Mistral Large / Medium / Small",
    authTypes: ["api_key"],
    defaultModels: [],
  },
  {
    key: "openrouter",
    name: "OpenRouter",
    description: "Multi-provider aggregator",
    authTypes: ["api_key"],
    defaultModels: [],
  },
];

// ---------------------------------------------------------------------------
// Raw admin fetch (endpoint not yet in the generated SDK)
// ---------------------------------------------------------------------------

const CSRF_KEY = "srapi_csrf_token";

function baseUrl(): string {
  return (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || "").replace(/\/+$/, "");
}

function csrfToken(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem(CSRF_KEY) ?? "";
}

async function postQuickSetup(body: QuickSetupBody): Promise<QuickSetupResult> {
  const res = await fetch(`${baseUrl()}/api/v1/admin/quick-setup`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrfToken(),
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const json = (await res.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    throw new Error(json?.error?.message || `Quick setup failed (${res.status})`);
  }
  const json = (await res.json()) as { data: QuickSetupResult };
  return json.data;
}

// ---------------------------------------------------------------------------
// Platform icon abbreviations
// ---------------------------------------------------------------------------

const PLATFORM_ICONS: Record<string, string> = {
  "codex-cli": "CX",
  openai: "OA",
  anthropic: "AN",
  deepseek: "DS",
  groq: "GQ",
  mistral: "MI",
  openrouter: "OR",
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AdminQuickSetupPage() {
  return (
    <AdminShell>
      <QuickSetupContent />
    </AdminShell>
  );
}

type Step = "platform" | "credentials" | "result";

const STEPS: Step[] = ["platform", "credentials", "result"];

function StepIndicator({
  current,
  t,
}: {
  current: Step;
  t: (key: string) => string;
}) {
  const labels: Record<Step, string> = {
    platform: t("adminQuickSetup.stepPlatform"),
    credentials: t("adminQuickSetup.stepCredentials"),
    result: t("adminQuickSetup.stepDone"),
  };
  const currentIdx = STEPS.indexOf(current);

  return (
    <div className="flex items-center gap-2">
      {STEPS.map((s, i) => {
        const done = i < currentIdx;
        const active = i === currentIdx;
        return (
          <div key={s} className="flex items-center gap-2">
            {i > 0 && (
              <div
                className={cn(
                  "h-px w-8 transition-colors",
                  done ? "bg-srapi-success" : "bg-srapi-border",
                )}
              />
            )}
            <div className="flex items-center gap-1.5">
              <div
                className={cn(
                  "flex size-6 items-center justify-center rounded-full text-2xs font-semibold transition-colors",
                  done && "bg-srapi-success/15 text-srapi-success",
                  active &&
                    "bg-srapi-text-primary text-srapi-bg",
                  !done && !active &&
                    "bg-srapi-bg-muted text-srapi-text-tertiary",
                )}
              >
                {done ? (
                  <CheckCircle2 className="size-3.5" />
                ) : (
                  i + 1
                )}
              </div>
              <span
                className={cn(
                  "text-xs transition-colors",
                  active
                    ? "font-medium text-srapi-text-primary"
                    : "text-srapi-text-tertiary",
                )}
              >
                {labels[s]}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function QuickSetupContent() {
  const { t } = useLanguage();
  const { toast } = useToast();

  const [step, setStep] = useState<Step>("platform");
  const [platform, setPlatform] = useState<PlatformPreset | null>(null);
  const [authType, setAuthType] = useState<AuthType>("api_key");
  const [result, setResult] = useState<QuickSetupResult | null>(null);

  // Credential fields
  const [apiKey, setApiKey] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [refreshToken, setRefreshToken] = useState("");
  const [accountName, setAccountName] = useState("");
  const [selectedModels, setSelectedModels] = useState<Set<string>>(new Set());

  // Advanced fields
  const [proxyId, setProxyId] = useState("");
  const [priority, setPriority] = useState("");
  const [weight, setWeight] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);

  const mutation = useMutation({
    mutationFn: postQuickSetup,
    onSuccess: (data) => {
      setResult(data);
      setStep("result");
      toast({ title: t("adminQuickSetup.success"), tone: "success" });
    },
    onError: (err: Error) => {
      toast({
        title: t("feedback.failed"),
        description: err.message,
        tone: "error",
      });
    },
  });

  function selectPlatform(p: PlatformPreset) {
    setPlatform(p);
    setAuthType(p.authTypes[0]);
    setSelectedModels(new Set());
    setApiKey("");
    setAccessToken("");
    setRefreshToken("");
    setAccountName("");
    setProxyId("");
    setPriority("");
    setWeight("");
    setShowAdvanced(false);
    setStep("credentials");
  }

  function toggleModel(model: string) {
    setSelectedModels((prev) => {
      const next = new Set(prev);
      if (next.has(model)) next.delete(model);
      else next.add(model);
      return next;
    });
  }

  function handleSubmit() {
    if (!platform) return;

    const credential: Record<string, string> =
      authType === "oauth_refresh"
        ? { access_token: accessToken, refresh_token: refreshToken }
        : { api_key: apiKey };

    const catalog =
      selectedModels.size > 0
        ? [...selectedModels]
        : platform.defaultModels.length > 0
          ? [...platform.defaultModels]
          : undefined;

    const body: QuickSetupBody = {
      platform: platform.key,
      credential,
      name: accountName || undefined,
      runtime_class: authType,
      model_catalog: catalog,
    };

    if (proxyId && proxyId !== "__none__") body.proxy_id = proxyId;
    if (priority) body.priority = parseInt(priority, 10);
    if (weight) body.weight = parseFloat(weight);

    mutation.mutate(body);
  }

  function reset() {
    setStep("platform");
    setPlatform(null);
    setResult(null);
    setApiKey("");
    setAccessToken("");
    setRefreshToken("");
    setAccountName("");
    setSelectedModels(new Set());
    setProxyId("");
    setPriority("");
    setWeight("");
    setShowAdvanced(false);
    mutation.reset();
  }

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminQuickSetup.title")}
        description={t("adminQuickSetup.subtitle")}
      />

      <div className="mb-6">
        <StepIndicator current={step} t={t} />
      </div>

      {step === "platform" && (
        <PlatformGrid platforms={PLATFORMS} onSelect={selectPlatform} />
      )}

      {step === "credentials" && platform && (
        <CredentialsForm
          platform={platform}
          authType={authType}
          onAuthTypeChange={setAuthType}
          apiKey={apiKey}
          onApiKeyChange={setApiKey}
          accessToken={accessToken}
          onAccessTokenChange={setAccessToken}
          refreshToken={refreshToken}
          onRefreshTokenChange={setRefreshToken}
          accountName={accountName}
          onAccountNameChange={setAccountName}
          selectedModels={selectedModels}
          onToggleModel={toggleModel}
          onSelectAll={() =>
            setSelectedModels(new Set(platform.defaultModels))
          }
          onClearModels={() => setSelectedModels(new Set())}
          proxyId={proxyId}
          onProxyIdChange={setProxyId}
          priority={priority}
          onPriorityChange={setPriority}
          weight={weight}
          onWeightChange={setWeight}
          showAdvanced={showAdvanced}
          onToggleAdvanced={() => setShowAdvanced((v) => !v)}
          isPending={mutation.isPending}
          onSubmit={handleSubmit}
          onBack={() => setStep("platform")}
          t={t}
        />
      )}

      {step === "result" && result && (
        <ResultView result={result} onReset={reset} t={t} />
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Step 1: Platform grid
// ---------------------------------------------------------------------------

function PlatformGrid({
  platforms,
  onSelect,
}: {
  platforms: PlatformPreset[];
  onSelect: (p: PlatformPreset) => void;
}) {
  const { t } = useLanguage();

  if (platforms.length === 0) {
    return (
      <p className="mt-6 text-sm text-srapi-text-tertiary">
        {t("adminQuickSetup.noPresets")}
      </p>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {platforms.map((p) => (
        <button
          key={p.key}
          type="button"
          onClick={() => onSelect(p)}
          className={cn(
            "group relative flex items-start gap-4 rounded-xl border border-srapi-border bg-srapi-card p-5 text-left transition-all",
            "hover:border-srapi-text-tertiary hover:shadow-sm",
            "active:scale-[0.985]",
          )}
        >
          <div
            className={cn(
              "flex size-11 shrink-0 items-center justify-center rounded-lg",
              "bg-srapi-bg-muted font-mono text-xs font-bold tracking-tight text-srapi-text-secondary",
              "transition-colors group-hover:bg-srapi-bg-sunken group-hover:text-srapi-text-primary",
            )}
          >
            {PLATFORM_ICONS[p.key] ?? p.key.slice(0, 2).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-srapi-text-primary">
              {p.name}
            </div>
            <div className="mt-0.5 text-xs leading-relaxed text-srapi-text-tertiary">
              {p.description}
            </div>
            <div className="mt-2.5 flex flex-wrap gap-1.5">
              {p.authTypes.map((a) => (
                <span
                  key={a}
                  className="rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary"
                >
                  {a}
                </span>
              ))}
              {p.defaultModels.length > 0 && (
                <span className="rounded-md bg-srapi-bg-muted px-1.5 py-0.5 text-2xs text-srapi-text-tertiary">
                  {p.defaultModels.length} models
                </span>
              )}
            </div>
          </div>
        </button>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Step 2: Credential form + Advanced settings
// ---------------------------------------------------------------------------

function CredentialsForm({
  platform,
  authType,
  onAuthTypeChange,
  apiKey,
  onApiKeyChange,
  accessToken,
  onAccessTokenChange,
  refreshToken,
  onRefreshTokenChange,
  accountName,
  onAccountNameChange,
  selectedModels,
  onToggleModel,
  onSelectAll,
  onClearModels,
  proxyId,
  onProxyIdChange,
  priority,
  onPriorityChange,
  weight,
  onWeightChange,
  showAdvanced,
  onToggleAdvanced,
  isPending,
  onSubmit,
  onBack,
  t,
}: {
  platform: PlatformPreset;
  authType: AuthType;
  onAuthTypeChange: (a: AuthType) => void;
  apiKey: string;
  onApiKeyChange: (v: string) => void;
  accessToken: string;
  onAccessTokenChange: (v: string) => void;
  refreshToken: string;
  onRefreshTokenChange: (v: string) => void;
  accountName: string;
  onAccountNameChange: (v: string) => void;
  selectedModels: Set<string>;
  onToggleModel: (m: string) => void;
  onSelectAll: () => void;
  onClearModels: () => void;
  proxyId: string;
  onProxyIdChange: (v: string) => void;
  priority: string;
  onPriorityChange: (v: string) => void;
  weight: string;
  onWeightChange: (v: string) => void;
  showAdvanced: boolean;
  onToggleAdvanced: () => void;
  isPending: boolean;
  onSubmit: () => void;
  onBack: () => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}) {
  const proxies = useAdminProxies();
  const activeProxies = (proxies.data?.data ?? []).filter(
    (p) => p.status === "active",
  );

  const hasMultipleAuth = platform.authTypes.length > 1;
  const isOAuth = authType === "oauth_refresh";

  const canSubmit = isOAuth
    ? accessToken.trim().length > 0 && refreshToken.trim().length > 0
    : apiKey.trim().length > 0;

  return (
    <div className="max-w-lg space-y-6">
      {/* Back link */}
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1 text-xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
      >
        <ChevronLeft className="size-3.5" />
        {platform.name}
      </button>

      {/* Auth type selector (only when multiple options) */}
      {hasMultipleAuth && (
        <div>
          <Label>{t("adminQuickSetup.credentials")}</Label>
          <div className="flex gap-2">
            {platform.authTypes.map((a) => (
              <button
                key={a}
                type="button"
                onClick={() => onAuthTypeChange(a)}
                className={cn(
                  "rounded-lg border px-3 py-1.5 font-mono text-xs transition-colors",
                  a === authType
                    ? "border-srapi-text-secondary bg-srapi-bg-muted text-srapi-text-primary"
                    : "border-srapi-border bg-srapi-card text-srapi-text-tertiary hover:border-srapi-text-tertiary",
                )}
              >
                {a}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Credential inputs */}
      {isOAuth ? (
        <div className="space-y-4">
          <div>
            <Label htmlFor="qs-access">access_token</Label>
            <Input
              id="qs-access"
              type="password"
              autoComplete="off"
              value={accessToken}
              onChange={(e) => onAccessTokenChange(e.target.value)}
              placeholder="eyJ..."
            />
          </div>
          <div>
            <Label htmlFor="qs-refresh">refresh_token</Label>
            <Input
              id="qs-refresh"
              type="password"
              autoComplete="off"
              value={refreshToken}
              onChange={(e) => onRefreshTokenChange(e.target.value)}
              placeholder="eyJ..."
            />
          </div>
        </div>
      ) : (
        <div>
          <Label htmlFor="qs-apikey">{t("adminQuickSetup.apiKeyLabel")}</Label>
          <Input
            id="qs-apikey"
            type="password"
            autoComplete="off"
            value={apiKey}
            onChange={(e) => onApiKeyChange(e.target.value)}
            placeholder="sk-..."
          />
        </div>
      )}

      {/* Account name */}
      <div>
        <Label htmlFor="qs-name">{t("adminQuickSetup.accountName")}</Label>
        <Input
          id="qs-name"
          value={accountName}
          onChange={(e) => onAccountNameChange(e.target.value)}
          placeholder={t("adminQuickSetup.accountNamePlaceholder")}
        />
      </div>

      {/* Model catalog */}
      {platform.defaultModels.length > 0 && (
        <div>
          <div className="mb-2 flex items-center justify-between">
            <Label className="mb-0">{t("adminQuickSetup.modelCatalog")}</Label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={onSelectAll}
                className="text-2xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
              >
                {t("adminQuickSetup.selectAll")}
              </button>
              <span className="text-2xs text-srapi-border">|</span>
              <button
                type="button"
                onClick={onClearModels}
                className="text-2xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
              >
                {t("adminQuickSetup.selectNone")}
              </button>
            </div>
          </div>
          <p className="mb-2 text-2xs text-srapi-text-tertiary">
            {t("adminQuickSetup.modelCatalogHint")}
          </p>
          <div className="flex flex-wrap gap-2">
            {platform.defaultModels.map((m) => {
              const selected = selectedModels.has(m);
              return (
                <button
                  key={m}
                  type="button"
                  onClick={() => onToggleModel(m)}
                  className={cn(
                    "rounded-lg border px-2.5 py-1 font-mono text-xs transition-colors",
                    selected
                      ? "border-srapi-text-secondary bg-srapi-bg-muted text-srapi-text-primary"
                      : "border-srapi-border bg-srapi-card text-srapi-text-tertiary hover:border-srapi-text-tertiary",
                  )}
                >
                  {m}
                </button>
              );
            })}
          </div>
        </div>
      )}

      {/* Advanced settings toggle */}
      <button
        type="button"
        onClick={onToggleAdvanced}
        className="flex w-full items-center gap-2 rounded-lg border border-srapi-border bg-srapi-card px-4 py-3 text-left transition-colors hover:bg-srapi-card-muted"
      >
        <Settings2 className="size-4 text-srapi-text-tertiary" />
        <span className="flex-1 text-sm text-srapi-text-secondary">
          {t("adminQuickSetup.advanced")}
        </span>
        <ChevronDown
          className={cn(
            "size-4 text-srapi-text-tertiary transition-transform",
            showAdvanced && "rotate-180",
          )}
        />
      </button>

      {/* Advanced fields */}
      {showAdvanced && (
        <div className="space-y-4 rounded-lg border border-srapi-border bg-srapi-card p-4">
          {/* Proxy selector */}
          <div>
            <Label>{t("adminQuickSetup.proxy")}</Label>
            <p className="mb-1.5 text-2xs text-srapi-text-tertiary">
              {t("adminQuickSetup.proxyHint")}
            </p>
            <Select value={proxyId} onValueChange={onProxyIdChange}>
              <SelectTrigger>
                <SelectValue
                  placeholder={t("adminQuickSetup.proxyNone")}
                />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">
                  {t("adminQuickSetup.proxyNone")}
                </SelectItem>
                {activeProxies.map((px) => (
                  <SelectItem key={px.id} value={String(px.id)}>
                    {px.name}{" "}
                    <span className="text-srapi-text-tertiary">
                      ({px.type})
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Priority + Weight on one row */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label htmlFor="qs-priority">
                {t("adminQuickSetup.priority")}
              </Label>
              <p className="mb-1.5 text-2xs text-srapi-text-tertiary">
                {t("adminQuickSetup.priorityHint")}
              </p>
              <Input
                id="qs-priority"
                type="number"
                min={0}
                value={priority}
                onChange={(e) => onPriorityChange(e.target.value)}
                placeholder="0"
              />
            </div>
            <div>
              <Label htmlFor="qs-weight">
                {t("adminQuickSetup.weight")}
              </Label>
              <p className="mb-1.5 text-2xs text-srapi-text-tertiary">
                {t("adminQuickSetup.weightHint")}
              </p>
              <Input
                id="qs-weight"
                type="number"
                min={0}
                step={0.1}
                value={weight}
                onChange={(e) => onWeightChange(e.target.value)}
                placeholder="1.0"
              />
            </div>
          </div>
        </div>
      )}

      {/* Submit */}
      <Button
        variant="primary"
        size="lg"
        className="w-full"
        disabled={!canSubmit}
        loading={isPending}
        onClick={onSubmit}
      >
        {isPending
          ? t("adminQuickSetup.submitting")
          : t("adminQuickSetup.submit")}
      </Button>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Step 3: Result
// ---------------------------------------------------------------------------

function ResultView({
  result,
  onReset,
  t,
}: {
  result: QuickSetupResult;
  onReset: () => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}) {
  const providerName =
    (result.provider as { display_name?: string; name?: string })
      ?.display_name ||
    (result.provider as { name?: string })?.name ||
    "—";
  const accountName =
    (result.account as { name?: string })?.name || "—";

  return (
    <div className="max-w-lg space-y-6">
      {/* Success header */}
      <div className="flex items-start gap-3 rounded-xl border border-srapi-success/20 bg-srapi-success/5 p-5">
        <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-srapi-success" />
        <div>
          <div className="text-sm font-medium text-srapi-text-primary">
            {t("adminQuickSetup.success")}
          </div>
          <div className="mt-1 text-xs text-srapi-text-secondary">
            {t("adminQuickSetup.successDetail", {
              models: String(result.models_created + result.mappings_created),
            })}
          </div>
        </div>
      </div>

      {/* Details */}
      <div className="rounded-xl border border-srapi-border bg-srapi-card p-5">
        <dl className="space-y-3 text-sm">
          <div className="flex justify-between">
            <dt className="text-srapi-text-tertiary">
              {t("adminQuickSetup.resultProvider")}
            </dt>
            <dd className="font-medium text-srapi-text-primary">
              {providerName}
            </dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-srapi-text-tertiary">
              {t("adminQuickSetup.resultAccount")}
            </dt>
            <dd className="font-medium text-srapi-text-primary">
              {accountName}
            </dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-srapi-text-tertiary">
              {t("adminQuickSetup.resultModels")}
            </dt>
            <dd className="font-medium text-srapi-text-primary">
              {result.models_created} / {result.mappings_created}
            </dd>
          </div>
        </dl>
      </div>

      {/* Model names */}
      {result.model_names && result.model_names.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {result.model_names.map((m) => (
            <span
              key={m}
              className="rounded-md bg-srapi-bg-muted px-2 py-0.5 font-mono text-2xs text-srapi-text-secondary"
            >
              {m}
            </span>
          ))}
        </div>
      )}

      {/* Warnings */}
      {result.warnings && result.warnings.length > 0 && (
        <div className="space-y-1.5 rounded-xl border border-srapi-warning/20 bg-srapi-warning/5 p-4">
          <div className="flex items-center gap-2 text-xs font-medium text-srapi-warning">
            <AlertTriangle className="size-3.5" />
            {t("adminQuickSetup.warnings")}
          </div>
          {result.warnings.map((w, i) => (
            <p key={i} className="text-xs text-srapi-text-secondary">
              {w}
            </p>
          ))}
        </div>
      )}

      {/* Actions */}
      <div className="flex gap-3">
        <Button variant="primary" size="md" onClick={onReset}>
          {t("adminQuickSetup.backToSetup")}
        </Button>
        <Button variant="outline" size="md" asChild>
          <a href={ADMIN_ROUTES.accounts}>
            {t("adminQuickSetup.goToAccounts")}
          </a>
        </Button>
      </div>
    </div>
  );
}
