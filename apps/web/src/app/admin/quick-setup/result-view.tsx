"use client";

import { CheckCircle2, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useDiscoverAccountModels, useTestAccount } from "@/hooks/admin-queries";
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
  const accountId = String(
    (result.account as { id?: string | number })?.id ?? "",
  );

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

      {/* Actions */}
      <div className="flex flex-wrap gap-3">
        <Button variant="primary" size="md" onClick={onReset}>
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
                    onSuccess: (res) =>
                      toast({
                        title: res?.ok ? t("adminAccounts.testOk") : t("adminAccounts.testFailed"),
                        description: res?.message || undefined,
                        tone: res?.ok ? "success" : "error",
                      }),
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
