"use client";

import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TagInput } from "@/components/ui/tag-input";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";

interface ModelSelectorProps {
  supportedModels: string[];
  excludedModels: string[];
  modelCatalog?: string[];
  onSupportedChange: (models: string[]) => void;
  onExcludedChange: (models: string[]) => void;
  onSyncUpstream?: () => Promise<string[]>;
  disabled?: boolean;
}

export function ModelSelector({
  supportedModels,
  excludedModels,
  modelCatalog,
  onSupportedChange,
  onExcludedChange,
  onSyncUpstream,
  disabled,
}: ModelSelectorProps) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const [syncing, setSyncing] = useState(false);

  async function handleSync() {
    if (!onSyncUpstream || syncing) return;
    setSyncing(true);
    try {
      const upstream = await onSyncUpstream();
      const existing = new Set(supportedModels);
      const added = upstream.filter((m) => !existing.has(m));
      if (added.length > 0) {
        onSupportedChange([...supportedModels, ...added]);
        toast({
          title: t("adminAccounts.modelSelector.syncDone", { count: added.length }),
          tone: "success",
        });
      } else {
        toast({ title: t("adminAccounts.modelSelector.syncNoNew"), tone: "info" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    } finally {
      setSyncing(false);
    }
  }

  return (
    <div className="space-y-4 rounded-lg border border-srapi-border p-3.5">
      <div className="flex items-center justify-between">
        <Label className="mb-0 text-sm font-medium">{t("adminAccounts.supportedModels")}</Label>
        {onSyncUpstream ? (
          <Button type="button" variant="outline" size="sm" disabled={disabled || syncing} onClick={handleSync}>
            <RefreshCw className={`size-3.5 ${syncing ? "animate-spin" : ""}`} />
            {t("adminAccounts.modelSelector.sync")}
          </Button>
        ) : null}
      </div>
      <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.supportedModelsHint")}</p>
      <TagInput
        value={supportedModels}
        onChange={onSupportedChange}
        disabled={disabled}
        placeholder={t("adminAccounts.supportedModelsPlaceholder")}
      />
      {modelCatalog?.length ? (
        <div className="flex flex-wrap gap-1">
          {modelCatalog
            .filter((m) => !supportedModels.includes(m))
            .map((m) => (
              <button
                key={m}
                type="button"
                disabled={disabled}
                className="rounded-full border border-srapi-border px-2.5 py-0.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:border-srapi-border-strong hover:bg-srapi-card-muted hover:text-srapi-text-primary"
                onClick={() => onSupportedChange([...supportedModels, m])}
              >
                + {m}
              </button>
            ))}
        </div>
      ) : null}

      <div className="border-t border-srapi-border pt-3">
        <Label className="text-sm font-medium">{t("adminAccounts.excludedModels")}</Label>
        <p className="mt-1 text-[11px] text-srapi-text-tertiary">{t("adminAccounts.excludedModelsHint")}</p>
        <TagInput
          value={excludedModels}
          onChange={onExcludedChange}
          disabled={disabled}
          placeholder={t("adminAccounts.excludedModelsPlaceholder")}
        />
      </div>
    </div>
  );
}
