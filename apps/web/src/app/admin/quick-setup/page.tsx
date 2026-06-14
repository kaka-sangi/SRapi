"use client";

import { useState } from "react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import {
  useAdminProviders,
  useCreateAccount,
  useRunQuickSetup,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import type { AdminQuickSetupRequestWritable, AdminQuickSetupResult } from "@/lib/sdk-types";
import {
  PLATFORMS,
  type AuthType,
  type PlatformPreset,
  type Step,
} from "./presets";
import { StepIndicator } from "./step-indicator";
import { PlatformGrid } from "./platform-grid";
import { CredentialsForm } from "./credentials-form";
import { ResultView } from "./result-view";

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

function QuickSetupContent() {
  const { t } = useLanguage();
  const { toast } = useToast();

  const [step, setStep] = useState<Step>("platform");
  const [platform, setPlatform] = useState<PlatformPreset | null>(null);
  const [authType, setAuthType] = useState<AuthType>("api_key");
  const [result, setResult] = useState<AdminQuickSetupResult | null>(null);

  // Credential fields
  const [apiKey, setApiKey] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [refreshToken, setRefreshToken] = useState("");
  const [accountName, setAccountName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [selectedModels, setSelectedModels] = useState<Set<string>>(new Set());

  // Advanced fields
  const [proxyId, setProxyId] = useState("");
  const [priority, setPriority] = useState("");
  const [weight, setWeight] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);

  const mutation = useRunQuickSetup();
  const createMut = useCreateAccount();
  const providers = useAdminProviders();

  function handleSuccess(data: AdminQuickSetupResult) {
    setResult(data);
    setStep("result");
    toast({ title: t("adminQuickSetup.success"), tone: "success" });
  }

  function handleError(err: Error) {
    toast({
      title: t("feedback.failed"),
      description: err.message,
      tone: "error",
    });
  }

  function submitQuickSetup(body: AdminQuickSetupRequestWritable) {
    mutation.mutate(body, {
      onSuccess: handleSuccess,
      onError: handleError,
    });
  }

  function resetMutation() {
    mutation.reset();
  }

  const isSubmitting = mutation.isPending || createMut.isPending;

  function selectPlatform(p: PlatformPreset) {
    setPlatform(p);
    setAuthType(p.authTypes[0]);
    setSelectedModels(new Set());
    setApiKey("");
    setAccessToken("");
    setRefreshToken("");
    setAccountName("");
    setBaseUrl("");
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

  async function handleSubmit() {
    if (!platform) return;

    if (platform.custom) {
      const providerList = providers.data?.data ?? [];
      const oaiProvider = providerList.find(
        (p) => p.platform_family === "openai_compatible",
      ) ?? providerList[0];
      if (!oaiProvider) {
        toast({ title: t("adminQuickSetup.noProvider"), tone: "error" });
        return;
      }
      const metadata: Record<string, unknown> = {};
      if (baseUrl.trim()) metadata.base_url = baseUrl.trim();
      try {
        const account = await createMut.mutateAsync({
          provider_id: oaiProvider.id,
          name: accountName || "Custom Account",
          runtime_class: "api_key",
          credential: { api_key: apiKey },
          metadata,
          status: "active",
          priority: priority ? parseInt(priority, 10) : undefined,
          weight: weight ? parseFloat(weight) : undefined,
          proxy_id: proxyId && proxyId !== "__none__" ? proxyId : undefined,
        });
        const fakeResult: AdminQuickSetupResult = {
          provider: oaiProvider,
          account,
          models_created: 0,
          mappings_created: 0,
          model_names: [],
          warnings: [],
        };
        handleSuccess(fakeResult);
      } catch (err) {
        handleError(err instanceof Error ? err : new Error(String(err)));
      }
      return;
    }

    const credential: AdminQuickSetupRequestWritable["credential"] =
      authType === "oauth_refresh"
        ? { access_token: accessToken, refresh_token: refreshToken }
        : { api_key: apiKey };

    const catalog =
      selectedModels.size > 0
        ? [...selectedModels]
        : platform.defaultModels.length > 0
          ? [...platform.defaultModels]
          : undefined;

    const body: AdminQuickSetupRequestWritable & { metadata?: Record<string, unknown> } = {
      platform: platform.key,
      credential,
      name: accountName || undefined,
      runtime_class: authType,
      model_catalog: catalog,
    };

    if (proxyId && proxyId !== "__none__") body.proxy_id = proxyId;
    if (priority) body.priority = parseInt(priority, 10);
    if (weight) body.weight = parseFloat(weight);
    if (baseUrl.trim()) body.metadata = { base_url: baseUrl.trim() };

    submitQuickSetup(body as AdminQuickSetupRequestWritable);
  }

  function reset() {
    setStep("platform");
    setPlatform(null);
    setResult(null);
    setApiKey("");
    setAccessToken("");
    setRefreshToken("");
    setAccountName("");
    setBaseUrl("");
    setSelectedModels(new Set());
    setProxyId("");
    setPriority("");
    setWeight("");
    setShowAdvanced(false);
    resetMutation();
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
          baseUrl={baseUrl}
          onBaseUrlChange={setBaseUrl}
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
          isPending={isSubmitting}
          onSubmit={() => void handleSubmit()}
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
