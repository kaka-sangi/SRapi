import { useAdminAccounts } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { type AdminSettingsCopilot } from "@/lib/admin-settings-form";

/**
 * Copilot tab: configure the admin AI copilot — enable it, choose which LLM
 * powers it (an existing provider account, or a standalone dedicated key), and
 * tune its autonomy. The dedicated API key is write-only.
 */
export function CopilotTab({
  value,
  onField,
  onSave,
  pending,
  modelOptions,
}: {
  value: AdminSettingsCopilot;
  onField: (key: keyof AdminSettingsCopilot, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
  modelOptions: MultiSelectOption[];
}) {
  const { t } = useLanguage();
  const accounts = useAdminAccounts();
  const accountOptions = (accounts.data?.data ?? []).map((a) => ({
    value: String(a.id),
    label: a.name,
  }));

  return (
    <Card>
      <CardContent className="space-y-5">
        <div className="flex items-start gap-2 rounded-lg border border-srapi-warning/30 bg-srapi-warning/5 px-3 py-2 text-xs text-srapi-text-secondary">
          <span>{t("copilot.settingsEgressWarning")}</span>
        </div>

        <div className="flex items-center justify-between gap-4">
          <div>
            <Label htmlFor="copilot-enabled" className="mb-0">
              {t("copilot.fieldEnabled")}
            </Label>
            <p className="mt-0.5 text-2xs text-srapi-text-tertiary">{t("copilot.fieldEnabledHint")}</p>
          </div>
          <Switch
            id="copilot-enabled"
            checked={value.enabled}
            onCheckedChange={(checked) => onField("enabled", checked)}
          />
        </div>

        <div className="grid gap-5 sm:grid-cols-2">
          <div>
            <Label>{t("copilot.fieldSource")}</Label>
            <Select
              value={value.source}
              onValueChange={(v) => {
                onField("source", v);
                // Clear the other mode's fields so switching source can't save
                // stale/conflicting credentials from the mode you left.
                if (v === "account") {
                  onField("dedicated_api_key", "");
                  onField("dedicated_base_url", "");
                  onField("dedicated_protocol", "");
                } else {
                  onField("provider_account_id", 0);
                }
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="account">{t("copilot.sourceAccount")}</SelectItem>
                <SelectItem value="dedicated">{t("copilot.sourceDedicated")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label htmlFor="copilot-model">{t("copilot.fieldModel")}</Label>
            <Input
              id="copilot-model"
              value={value.model}
              placeholder="claude-3-5-sonnet / gpt-4o"
              onChange={(e) => onField("model", e.target.value)}
            />
            <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("copilot.fieldModelHint")}</p>
          </div>
        </div>

        <div>
          <Label>{t("copilot.fieldModels")}</Label>
          <div className="mt-1.5">
            <MultiSelect
              value={value.models ?? []}
              onChange={(next) => onField("models", next)}
              options={modelOptions}
              allowCustom
              placeholder={t("copilot.fieldModelsPlaceholder")}
              searchPlaceholder={t("adminCommon.search")}
              emptyText={t("adminCommon.noResults")}
              addCustomLabel={(q) => t("adminCommon.addValue", { value: q })}
            />
          </div>
          <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("copilot.fieldModelsHint")}</p>
        </div>

        {value.source === "account" ? (
          <div>
            <Label>{t("copilot.fieldAccount")}</Label>
            <Select
              value={value.provider_account_id ? String(value.provider_account_id) : ""}
              onValueChange={(v) => v && onField("provider_account_id", Number(v))}
            >
              <SelectTrigger>
                <SelectValue placeholder={t("copilot.selectAccount")} />
              </SelectTrigger>
              <SelectContent>
                {accountOptions.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("copilot.fieldAccountHint")}</p>
          </div>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2">
            <div>
              <Label>{t("copilot.fieldProtocol")}</Label>
              <Select
                value={value.dedicated_protocol}
                onValueChange={(v) => onField("dedicated_protocol", v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="openai-compatible">{t("copilot.protocolOpenAI")}</SelectItem>
                  <SelectItem value="anthropic-compatible">{t("copilot.protocolAnthropic")}</SelectItem>
                  <SelectItem value="gemini-compatible">{t("copilot.protocolGemini")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="copilot-base-url">{t("copilot.fieldBaseUrl")}</Label>
              <Input
                id="copilot-base-url"
                value={value.dedicated_base_url}
                placeholder="https://api.openai.com/v1"
                onChange={(e) => onField("dedicated_base_url", e.target.value)}
              />
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="copilot-key">{t("copilot.fieldApiKey")}</Label>
              <Input
                id="copilot-key"
                type="password"
                autoComplete="off"
                value={value.dedicated_api_key ?? ""}
                placeholder={
                  value.dedicated_api_key_configured ? t("copilot.keyConfigured") : t("copilot.keyPlaceholder")
                }
                onChange={(e) => onField("dedicated_api_key", e.target.value)}
              />
            </div>
          </div>
        )}


        <div className="space-y-4 border-t border-srapi-border pt-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="copilot-websearch" className="mb-0">
                {t("copilot.fieldWebSearch")}
              </Label>
              <p className="mt-0.5 text-2xs text-srapi-text-tertiary">{t("copilot.fieldWebSearchHint")}</p>
            </div>
            <Switch
              id="copilot-websearch"
              checked={value.web_search_enabled}
              onCheckedChange={(checked) => onField("web_search_enabled", checked)}
            />
          </div>
          {value.web_search_enabled ? (
            <div className="grid gap-5 sm:grid-cols-2">
              <div>
                <Label>{t("copilot.fieldWebSearchProvider")}</Label>
                <Select
                  value={value.web_search_provider || "tavily"}
                  onValueChange={(v) => onField("web_search_provider", v)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="tavily">Tavily</SelectItem>
                    <SelectItem value="brave">Brave</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="copilot-websearch-url">{t("copilot.fieldWebSearchBaseUrl")}</Label>
                <Input
                  id="copilot-websearch-url"
                  value={value.web_search_base_url}
                  placeholder="https://api.tavily.com"
                  onChange={(e) => onField("web_search_base_url", e.target.value)}
                />
              </div>
              <div className="sm:col-span-2">
                <Label htmlFor="copilot-websearch-key">{t("copilot.fieldWebSearchKey")}</Label>
                <Input
                  id="copilot-websearch-key"
                  type="password"
                  autoComplete="off"
                  value={value.web_search_api_key ?? ""}
                  placeholder={
                    value.web_search_api_key_configured
                      ? t("copilot.keyConfigured")
                      : t("copilot.keyPlaceholder")
                  }
                  onChange={(e) => onField("web_search_api_key", e.target.value)}
                />
              </div>
            </div>
          ) : null}
        </div>

        <div className="space-y-4 border-t border-srapi-border pt-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="copilot-autorun" className="mb-0">
                {t("copilot.fieldAutoRunReads")}
              </Label>
              <p className="mt-0.5 text-2xs text-srapi-text-tertiary">{t("copilot.fieldAutoRunReadsHint")}</p>
            </div>
            <Switch
              id="copilot-autorun"
              checked={value.auto_run_reads}
              onCheckedChange={(checked) => onField("auto_run_reads", checked)}
            />
          </div>
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="copilot-owner" className="mb-0">
                {t("copilot.fieldOwnerOnly")}
              </Label>
              <p className="mt-0.5 text-2xs text-srapi-text-tertiary">{t("copilot.fieldOwnerOnlyHint")}</p>
            </div>
            <Switch
              id="copilot-owner"
              checked={value.owner_only}
              onCheckedChange={(checked) => onField("owner_only", checked)}
            />
          </div>
        </div>

        <div className="flex justify-end border-t border-srapi-border pt-4">
          <Button variant="primary" loading={pending} onClick={onSave}>
            {t("adminSettings.saveSection")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
