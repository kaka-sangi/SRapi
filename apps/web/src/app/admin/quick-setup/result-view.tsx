"use client";

import { useState } from "react";
import { CheckCircle2, AlertTriangle, Copy, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { writeClipboard } from "@/components/ui/copy-button";
import { Label } from "@/components/ui/label";
import { useDiscoverAccountModels, useTestAccount } from "@/hooks/admin-queries";
import { useCreateApiKey } from "@/hooks/queries";
import { ApiKeyOnboarding } from "@/components/features/api-key-onboarding";
import { gatewayErrorHintKey } from "@/lib/gateway-error-hint";
import { useToast } from "@/context/ToastContext";
import { ADMIN_ROUTES } from "@/lib/routes";
import type { AdminQuickSetupResult } from "@/lib/sdk-types";

// ---------------------------------------------------------------------------
// Step 3: Result
// ---------------------------------------------------------------------------

export function ResultView({
  result,
  onReset,
  t,
}: {
  result: AdminQuickSetupResult;
  onReset: () => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}) {
  const { toast } = useToast();
  const discoverMut = useDiscoverAccountModels();
  const testMut = useTestAccount();
  const createKey = useCreateApiKey();
  const [keyPlaintext, setKeyPlaintext] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const accountId = String(
    (result.account as { id?: string | number })?.id ?? "",
  );
  const modelNames = result.model_names ?? [];
  // Only offer the one-click key when models actually route here — the key
  // create contract requires at least one allowed model.
  const canGenerateKey = modelNames.length > 0;

  // Finish the journey the wizard otherwise leaves half-done: mint a key bound
  // to NO group (group_ids empty) so the account just created is immediately
  // usable, with the models it just registered allow-listed.
  async function generateKey() {
    try {
      const created = await createKey.mutateAsync({
        name: t("adminQuickSetup.defaultKeyName"),
        allowedModels: modelNames.slice(0, 16),
        groupIds: [],
      });
      setKeyPlaintext(created.plaintextKey ?? null);
    } catch {
      toast({ title: t("adminQuickSetup.keyFailed"), tone: "error" });
    }
  }

  async function copyKey() {
    if (!keyPlaintext) return;
    const ok = await writeClipboard(keyPlaintext);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  }

  const providerName =
    (result.provider as { display_name?: string; name?: string })
      ?.display_name ||
    (result.provider as { name?: string })?.name ||
    "—";
  const accountName =
    (result.account as { name?: string })?.name || "—";

  return (
    <div className="space-y-6">
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

      {/* No models routed yet (e.g. a custom OpenAI-compatible provider): warn
          so the operator does not create a key against a model that nothing
          serves. */}
      {result.models_created + result.mappings_created === 0 ? (
        <div className="flex items-start gap-3 rounded-xl border border-srapi-warning/30 bg-srapi-warning/5 p-5">
          <AlertTriangle className="mt-0.5 size-5 shrink-0 text-srapi-warning" />
          <div>
            <div className="text-sm font-medium text-srapi-text-primary">
              {t("adminQuickSetup.noModelsTitle")}
            </div>
            <div className="mt-1 text-xs text-srapi-text-secondary">
              {t("adminQuickSetup.noModelsBody")}
            </div>
          </div>
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
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
              <dd className="text-right">
                <div className="font-medium text-srapi-text-primary">{accountName}</div>
                {accountId && (
                  <div className="font-mono text-2xs text-srapi-text-tertiary">ID: {accountId}</div>
                )}
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
          <div className="rounded-xl border border-srapi-border bg-srapi-card p-5">
            <Label className="mb-3">{t("adminQuickSetup.resultModels")}</Label>
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
          </div>
        )}
      </div>

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

      {/* One-click key: the wizard otherwise stops at provider+account+models.
          Mint a ready-to-use key bound to NO group so the account just created
          can be called immediately, with no further wiring. */}
      {keyPlaintext ? (
        <div className="space-y-3 rounded-xl border border-srapi-success/20 bg-srapi-success/5 p-5">
          <div>
            <div className="text-sm font-medium text-srapi-text-primary">
              {t("adminQuickSetup.keyReady")}
            </div>
            <div className="mt-1 text-xs text-srapi-text-secondary">
              {t("adminQuickSetup.keyUnbound")} {t("adminQuickSetup.keyRevealOnce")}
            </div>
          </div>
          <div className="flex items-center gap-2 rounded-xl border border-srapi-border bg-srapi-card px-3 py-2.5">
            <code className="flex-1 truncate font-mono text-sm">{keyPlaintext}</code>
            <Button
              variant="outline"
              size="icon"
              onClick={() => void copyKey()}
              aria-label={t("apiKeys.copyKey")}
            >
              {copied ? (
                <Check className="size-4 text-srapi-success" />
              ) : (
                <Copy className="size-4" />
              )}
            </Button>
          </div>
          <ApiKeyOnboarding apiKey={keyPlaintext} defaultModel={modelNames[0]} />
        </div>
      ) : null}

      {/* Actions */}
      <div className="flex flex-wrap gap-3">
        {canGenerateKey && !keyPlaintext ? (
          <Button
            variant="primary"
            size="md"
            loading={createKey.isPending}
            onClick={() => void generateKey()}
          >
            {t("adminQuickSetup.generateKey")}
          </Button>
        ) : null}
        <Button variant="outline" size="md" onClick={onReset}>
          {t("adminQuickSetup.backToSetup")}
        </Button>
        <Button variant="outline" size="md" asChild>
          <a href={ADMIN_ROUTES.accounts}>
            {t("adminQuickSetup.goToAccounts")}
          </a>
        </Button>
        <Button variant="outline" size="md" asChild>
          <a href={ADMIN_ROUTES.models}>{t("adminQuickSetup.goToModels")}</a>
        </Button>
        <Button variant="outline" size="md" asChild>
          <a href="/api-keys">{t("adminQuickSetup.createKey")}</a>
        </Button>
        {accountId && (
          <>
            <Button
              variant="outline"
              size="md"
              loading={testMut.isPending}
              onClick={() =>
                testMut.mutate(
                  { id: accountId, body: { mode: "live" } },
                  {
                    onSuccess: (res) => {
                      const hint = res?.ok ? null : gatewayErrorHintKey(res?.message);
                      toast({
                        title: res?.ok ? t("adminAccounts.testOk") : t("adminAccounts.testFailed"),
                        description:
                          [res?.message, hint ? t(`gatewayHints.${hint}`) : null]
                            .filter(Boolean)
                            .join(" — ") || undefined,
                        tone: res?.ok ? "success" : "error",
                      });
                    },
                    onError: () =>
                      toast({ title: t("adminAccounts.testFailed"), tone: "error" }),
                  },
                )
              }
            >
              {t("adminAccounts.test")}
            </Button>
            <Button
              variant="outline"
              size="md"
              loading={discoverMut.isPending}
              onClick={() =>
                discoverMut.mutate(
                  { id: accountId },
                  {
                    onSuccess: () =>
                      toast({
                        title: t("adminQuickSetup.discoverDone"),
                        tone: "success",
                      }),
                    onError: () =>
                      toast({
                        title: t("adminQuickSetup.discoverFailed"),
                        description: t("adminQuickSetup.discoverFailedHint"),
                        tone: "error",
                      }),
                  },
                )
              }
            >
              {t("adminQuickSetup.discoverModels")}
            </Button>
          </>
        )}
      </div>
    </div>
  );
}
