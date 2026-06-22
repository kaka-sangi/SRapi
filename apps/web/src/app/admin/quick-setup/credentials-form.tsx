"use client";

import { useState } from "react";
import { ChevronLeft, ChevronDown, Settings2, Eye, EyeOff } from "lucide-react";
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
import { cn } from "@/lib/cn";
import {
  PLATFORM_ICON_COLORS,
  PLATFORM_ICONS,
  type AuthType,
  type PlatformPreset,
} from "./presets";

// ---------------------------------------------------------------------------
// Step 2: Credential form + Advanced settings
// ---------------------------------------------------------------------------

/**
 * Masked credential input with a reveal toggle, so the operator can verify a
 * pasted key/token instead of staring at dots. Mirrors the reveal pattern used
 * in the account form dialog.
 */
function SecretInput({
  id,
  value,
  onChange,
  placeholder,
}: {
  id: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  const { t } = useLanguage();
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative">
      <Input
        id={id}
        type={visible ? "text" : "password"}
        autoComplete="off"
        spellCheck={false}
        className="pr-9 font-mono"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
      />
      <button
        type="button"
        tabIndex={-1}
        aria-label={visible ? t("common.hide") : t("common.show")}
        onClick={() => setVisible((v) => !v)}
        className="absolute right-2 top-1/2 -translate-y-1/2 text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}

export function CredentialsForm({
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
  baseUrl,
  onBaseUrlChange,
  customModels,
  onCustomModelsChange,
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
  baseUrl: string;
  onBaseUrlChange: (v: string) => void;
  customModels: string;
  onCustomModelsChange: (v: string) => void;
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
    : apiKey.trim().length > 0 && (!platform.custom || baseUrl.trim().length > 0);

  // Tell the operator *why* the submit button is disabled instead of leaving a
  // dead grey button — the most common "I'm stuck" moment in the wizard.
  const submitHint = isOAuth
    ? !accessToken.trim() || !refreshToken.trim()
      ? t("adminQuickSetup.needTokens")
      : ""
    : !apiKey.trim()
      ? t("adminQuickSetup.needApiKey")
      : platform.custom && !baseUrl.trim()
        ? t("adminQuickSetup.needBaseUrl")
        : "";

  return (
    <div className="space-y-6">
      {/* Back link */}
      <button
        type="button"
        onClick={onBack}
        className="inline-flex items-center gap-1 text-xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
      >
        <ChevronLeft className="size-3.5" />
        {platform.name}
      </button>

      <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
        {/* ── Left column: credentials ── */}
        <div className="space-y-5">
          <div className="flex items-center gap-3">
            <div className={cn(
              "flex size-10 shrink-0 items-center justify-center rounded-lg font-mono text-xs font-bold tracking-tight",
              PLATFORM_ICON_COLORS[platform.key] ?? "bg-srapi-card-muted text-srapi-text-secondary",
            )}>
              {PLATFORM_ICONS[platform.key] ?? platform.key.slice(0, 2).toUpperCase()}
            </div>
            <div>
              <div className="text-sm font-medium text-srapi-text-primary">{platform.name}</div>
              <div className="text-xs text-srapi-text-tertiary">{platform.description}</div>
            </div>
          </div>

          {/* Auth type selector */}
          {hasMultipleAuth && (
            <div>
              <Label>{t("adminQuickSetup.credentials")}</Label>
              <div className="inline-flex items-center gap-0.5 rounded-xl border border-srapi-border bg-srapi-card/80 p-1">
                {platform.authTypes.map((a) => (
                  <button
                    key={a}
                    type="button"
                    onClick={() => onAuthTypeChange(a)}
                    className={cn(
                      "rounded-lg px-3 py-1.5 text-xs font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary/40",
                      a === authType
                        ? "bg-srapi-accent-soft text-srapi-primary shadow-[0_1px_2px_rgba(26,24,20,0.04)]"
                        : "text-srapi-text-tertiary hover:text-srapi-text-secondary",
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
                <SecretInput
                  id="qs-access"
                  value={accessToken}
                  onChange={onAccessTokenChange}
                  placeholder="eyJ..."
                />
              </div>
              <div>
                <Label htmlFor="qs-refresh">refresh_token</Label>
                <SecretInput
                  id="qs-refresh"
                  value={refreshToken}
                  onChange={onRefreshTokenChange}
                  placeholder="eyJ..."
                />
              </div>
            </div>
          ) : (
            <div>
              <Label htmlFor="qs-apikey">{t("adminQuickSetup.apiKeyLabel")}</Label>
              <SecretInput
                id="qs-apikey"
                value={apiKey}
                onChange={onApiKeyChange}
                placeholder="sk-..."
              />
            </div>
          )}

          {/* Base URL — always visible for custom, in advanced for presets */}
          {platform.custom && (
            <div>
              <Label htmlFor="qs-baseurl">{t("adminAccounts.baseUrl")}</Label>
              <Input
                id="qs-baseurl"
                type="url"
                className="font-mono"
                value={baseUrl}
                onChange={(e) => onBaseUrlChange(e.target.value)}
                placeholder="https://api.example.com/v1"
              />
              <p className="mt-1 text-xs text-srapi-text-tertiary">
                {t("adminAccounts.baseUrlHint")}
              </p>
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

          {showAdvanced && (
            <div className="space-y-4 rounded-lg border border-srapi-border bg-srapi-card p-4">
              {!platform.custom && (
                <div>
                  <Label htmlFor="qs-baseurl-adv">{t("adminAccounts.baseUrl")}</Label>
                  <Input
                    id="qs-baseurl-adv"
                    type="url"
                    className="font-mono"
                    value={baseUrl}
                    onChange={(e) => onBaseUrlChange(e.target.value)}
                    placeholder={t("adminAccounts.baseUrlPlaceholder")}
                  />
                  <p className="mt-1 text-xs text-srapi-text-tertiary">
                    {t("adminAccounts.baseUrlHint")}
                  </p>
                </div>
              )}
              <div>
                <Label>{t("adminQuickSetup.proxy")}</Label>
                <p className="mb-1.5 text-xs text-srapi-text-tertiary">
                  {t("adminQuickSetup.proxyHint")}
                </p>
                <Select value={proxyId} onValueChange={onProxyIdChange}>
                  <SelectTrigger>
                    <SelectValue placeholder={t("adminQuickSetup.proxyNone")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">
                      {t("adminQuickSetup.proxyNone")}
                    </SelectItem>
                    {activeProxies.map((px) => (
                      <SelectItem key={px.id} value={String(px.id)}>
                        {px.name}{" "}
                        <span className="text-srapi-text-tertiary">({px.type})</span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Label htmlFor="qs-priority">{t("adminQuickSetup.priority")}</Label>
                  <p className="mb-1.5 text-xs text-srapi-text-tertiary">
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
                  <Label htmlFor="qs-weight">{t("adminQuickSetup.weight")}</Label>
                  <p className="mb-1.5 text-xs text-srapi-text-tertiary">
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
        </div>

        {/* ── Right column: model catalog ── */}
        <div className="space-y-5">
          {platform.defaultModels.length > 0 && (
            <div className="rounded-xl border border-srapi-border bg-srapi-card p-5">
              <div className="mb-3 flex items-center justify-between">
                <Label className="mb-0">{t("adminQuickSetup.modelCatalog")}</Label>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={onSelectAll}
                    className="text-xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
                  >
                    {t("adminQuickSetup.selectAll")}
                  </button>
                  <span className="text-xs text-srapi-border">|</span>
                  <button
                    type="button"
                    onClick={onClearModels}
                    className="text-xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
                  >
                    {t("adminQuickSetup.selectNone")}
                  </button>
                </div>
              </div>
              <p className="mb-3 text-xs text-srapi-text-tertiary">
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
                        "rounded-full px-3 py-1 font-mono text-xs font-medium transition-colors",
                        selected
                          ? "bg-srapi-accent-soft text-srapi-primary"
                          : "bg-srapi-card-muted text-srapi-text-tertiary hover:text-srapi-text-secondary",
                      )}
                    >
                      {m}
                    </button>
                  );
                })}
              </div>
              {selectedModels.size > 0 && (
                <p className="mt-3 text-xs text-srapi-text-tertiary tabular">
                  {selectedModels.size} / {platform.defaultModels.length}
                </p>
              )}
            </div>
          )}
          {platform.custom && (
            <div className="rounded-2xl border border-srapi-border bg-srapi-card p-5">
              <Label className="mb-2 block">{t("adminQuickSetup.modelCatalog")}</Label>
              <p className="mb-3 text-xs text-srapi-text-tertiary">
                {t("adminQuickSetup.modelCatalogHint")}
              </p>
              <textarea
                value={customModels}
                onChange={(e) => onCustomModelsChange(e.target.value)}
                rows={6}
                placeholder={"gpt-4o\ndeepseek-chat\nllama-3.3-70b"}
                className="w-full rounded-xl border border-srapi-border bg-srapi-card px-3 py-2 font-mono text-xs text-srapi-text-primary placeholder:text-srapi-text-tertiary focus:border-srapi-primary focus:outline-none"
              />
            </div>
          )}
        </div>
      </div>

      {/* Submit — full width below both columns */}
      <div className="max-w-sm">
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
        {submitHint && !isPending ? (
          <p className="mt-1.5 text-xs text-srapi-text-tertiary">{submitHint}</p>
        ) : null}
      </div>
    </div>
  );
}
