"use client";

import { useMemo, useState } from "react";
import { Copy, Check, CheckCircle2, XCircle, Loader2, Mail } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  useAdminSettings,
  useUpdateSettings,
  useSendTestEmail,
  useConfigSnapshot,
  useImportConfigSnapshot,
  useAdminModels,
  useAdminAccounts,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { TagInput } from "@/components/ui/tag-input";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { KeyValueEditor } from "@/components/ui/key-value-editor";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  SETTINGS_TABS,
  type SettingsTab,
  type AdminSettingsDraft,
  type AdminSettingsCopilot,
  createSettingsDraft,
  materializeSettingsDraft,
  settingsTabRequiresConfirmation,
  settingsSaveConfirmationPhrase,
} from "@/lib/admin-settings-form";
import { cn } from "@/lib/cn";
import { formatDateTime } from "@/lib/admin-format";

export default function AdminSettingsPage() {
  return (
    <AdminShell>
      <SettingsContent />
    </AdminShell>
  );
}

/** Graphical controls for the list/map fields the draft tracks outside `value`. */
type SpecialKind = "tags" | "models" | "templates" | "json";
interface SpecialField {
  key: keyof AdminSettingsDraft;
  kind: SpecialKind;
  skip: string;
}
const SPECIAL_FIELDS: Partial<Record<SettingsTab, SpecialField[]>> = {
  general: [{ key: "customMenusJson", kind: "json", skip: "custom_menus" }],
  features: [{ key: "enabledChannels", kind: "tags", skip: "enabled_channels" }],
  security: [{ key: "oauthProviders", kind: "tags", skip: "oauth_providers" }],
  gateway: [
    { key: "schedulerRolloutModels", kind: "models", skip: "scheduler_strategy_rollout_models" },
    {
      key: "schedulerRolloutApiKeyHashes",
      kind: "tags",
      skip: "scheduler_strategy_rollout_api_key_hashes",
    },
    {
      key: "passthroughHeaderAllowlist",
      kind: "tags",
      skip: "passthrough_header_allowlist",
    },
  ],
  payment: [{ key: "paymentProviders", kind: "tags", skip: "providers" }],
  email: [{ key: "emailTemplates", kind: "templates", skip: "templates" }],
};

/**
 * Gateway numeric settings that must stay non-negative integers (the
 * operator-tunable retry/failover knobs and cooldown/timeout values). The Go
 * side clamps too, but clamping in the input keeps the control honest.
 */
const GATEWAY_NON_NEGATIVE_INT_FIELDS = new Set<string>([
  "overload_cooldown_seconds",
  "rate_limit_cooldown_seconds",
  "stream_timeout_seconds",
  "retry_count",
  "max_retry_credentials",
  "max_retry_interval_ms",
]);

function humanize(key: string): string {
  return key.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

/** Localized settings-field label; falls back to humanized English for any unmapped key. */
function fieldLabel(key: string, t: (k: string) => string): string {
  const id = `adminSettings.fields.${key}`;
  const label = t(id);
  return label === id ? humanize(key) : label;
}

function SettingsContent() {
  const { t } = useLanguage();
  const settings = useAdminSettings();

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminSettings.title")}
        description={t("adminSettings.subtitle")}
      />
      <PageQueryState
        query={settings}
        skeleton={<Skeleton className="h-96 rounded-xl" />}
      >
        {(data) => <SettingsEditor initial={data} />}
      </PageQueryState>
    </>
  );
}

function SettingsEditor({ initial }: { initial: Parameters<typeof createSettingsDraft>[0] }) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const updateMut = useUpdateSettings();
  const models = useAdminModels();
  const modelOptions: MultiSelectOption[] = (models.data?.data ?? []).map((m) => ({
    value: m.canonical_name ?? m.id,
    label: m.canonical_name ?? m.id,
  }));
  const [draft, setDraft] = useState<AdminSettingsDraft>(() => createSettingsDraft(initial));
  const [confirmTab, setConfirmTab] = useState<SettingsTab | null>(null);

  function setSectionField(section: SettingsTab, key: string, value: unknown) {
    setDraft((d) => ({
      ...d,
      value: {
        ...d.value,
        [section]: { ...(d.value[section] as Record<string, unknown>), [key]: value },
      },
    }));
  }

  function setSpecial(key: keyof AdminSettingsDraft, value: unknown) {
    setDraft((d) => ({ ...d, [key]: value }) as AdminSettingsDraft);
  }

  async function save() {
    try {
      const body = materializeSettingsDraft(draft);
      await updateMut.mutateAsync(body);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  function requestSave(tab: SettingsTab) {
    if (settingsTabRequiresConfirmation(tab)) {
      setConfirmTab(tab);
    } else {
      void save();
    }
  }

  return (
    <Tabs defaultValue="general">
      <TabsList className="flex-wrap">
        {SETTINGS_TABS.map((tab) => (
          <TabsTrigger key={tab.id} value={tab.id}>
            {t(`adminSettings.tabs.${tab.id}`)}
          </TabsTrigger>
        ))}
      </TabsList>

      {SETTINGS_TABS.map((tab) => (
        <TabsContent key={tab.id} value={tab.id}>
          {tab.id === "backup" ? (
            <BackupTab />
          ) : tab.id === "copilot" ? (
            <CopilotTab
              value={draft.value.copilot as AdminSettingsCopilot}
              onField={(key, v) => setSectionField("copilot", key, v)}
              onSave={() => requestSave("copilot")}
              pending={updateMut.isPending}
              modelOptions={modelOptions}
            />
          ) : (
            <Card>
              <CardContent className="space-y-5">
                <SectionFields
                  section={tab.id}
                  value={draft.value[tab.id] as Record<string, unknown>}
                  onChange={(key, value) => setSectionField(tab.id, key, value)}
                />
                {(SPECIAL_FIELDS[tab.id] ?? []).map((field) => (
                  <SpecialFieldRow
                    key={String(field.key)}
                    field={field}
                    draft={draft}
                    onChange={setSpecial}
                    modelOptions={modelOptions}
                  />
                ))}
                {tab.id === "email" ? <EmailTestPanel /> : null}
                <div className="flex justify-end border-t border-srapi-border pt-4">
                  <Button variant="primary" loading={updateMut.isPending} onClick={() => requestSave(tab.id)}>
                    {t("adminSettings.saveSection")}
                  </Button>
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>
      ))}

      {confirmTab ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setConfirmTab(null);
          }}
          title={t("adminSettings.saveSection")}
          tone="default"
          confirmLabel={t("common.save")}
          confirmPhrase={settingsSaveConfirmationPhrase(confirmTab)}
          onConfirm={save}
          successMessage={t("feedback.saved")}
          isPending={updateMut.isPending}
        />
      ) : null}
    </Tabs>
  );
}

/**
 * "Send test email" control for the email tab. The SMTP password is write-only,
 * so a live delivery is the only way to confirm the saved credentials work. Save
 * the SMTP fields first, then send to the admin's own mailbox (or an override)
 * and read the per-step checks below.
 */
function EmailTestPanel() {
  const { t } = useLanguage();
  const sendMut = useSendTestEmail();
  const [recipient, setRecipient] = useState("");
  const result = sendMut.data;
  const loading = sendMut.isPending;
  const failed = !loading && (sendMut.isError || result?.ok === false);
  const ok = !loading && !sendMut.isError && result?.ok === true;
  const checks = (result?.checks as Record<string, unknown> | undefined) ?? undefined;

  function send() {
    const trimmed = recipient.trim();
    sendMut.mutate(trimmed ? { recipient: trimmed } : undefined);
  }

  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex items-center gap-2">
        <Mail className="size-4 text-srapi-text-tertiary" />
        <span className="text-sm font-medium text-srapi-text-primary">
          {t("adminSettings.testEmail.title")}
        </span>
      </div>
      <p className="mt-1 text-2xs text-srapi-text-tertiary">
        {t("adminSettings.testEmail.hint")}
      </p>
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-end">
        <div className="flex-1">
          <Label htmlFor="s-test-email-recipient">
            {t("adminSettings.testEmail.recipient")}
          </Label>
          <Input
            id="s-test-email-recipient"
            type="email"
            className="mt-1.5"
            value={recipient}
            placeholder={t("adminSettings.testEmail.recipientPlaceholder")}
            onChange={(e) => setRecipient(e.target.value)}
          />
        </div>
        <Button variant="outline" loading={loading} onClick={send}>
          {t("adminSettings.testEmail.send")}
        </Button>
      </div>

      {loading || result || sendMut.isError ? (
        <div className="mt-3 rounded-lg border border-srapi-border bg-srapi-card p-3.5 font-mono text-xs">
          <div className="flex items-center gap-2">
            {loading ? (
              <>
                <Loader2 className="size-3.5 animate-spin text-srapi-text-tertiary" />
                <span className="text-srapi-text-secondary">
                  {t("adminSettings.testEmail.running")}
                </span>
              </>
            ) : failed ? (
              <>
                <XCircle className="size-3.5 text-srapi-error" />
                <span className="text-srapi-error">{t("adminSettings.testEmail.failed")}</span>
              </>
            ) : ok ? (
              <>
                <CheckCircle2 className="size-3.5 text-srapi-success" />
                <span className="text-srapi-success">{t("adminSettings.testEmail.ok")}</span>
              </>
            ) : null}
          </div>

          {!loading && (sendMut.isError || result?.message) ? (
            <p className="mt-2 break-words text-srapi-text-secondary">
              {sendMut.isError ? adminErrorMessage(sendMut.error) : result?.message}
            </p>
          ) : null}

          {!loading && checks && Object.keys(checks).length > 0 ? (
            <dl className="mt-2.5 space-y-1 border-t border-srapi-border pt-2.5">
              {Object.entries(checks).map(([k, v]) => (
                <div key={k} className="flex items-baseline justify-between gap-3">
                  <dt className="text-srapi-text-tertiary">{k}</dt>
                  <dd
                    className={cn(
                      "tabular text-right",
                      v === true
                        ? "text-srapi-success"
                        : v === false
                          ? "text-srapi-error"
                          : "text-srapi-text-primary",
                    )}
                  >
                    {stringifyEmailCheck(v)}
                  </dd>
                </div>
              ))}
            </dl>
          ) : null}

          {!loading && result?.checked_at ? (
            <p className="mt-2.5 text-[10px] text-srapi-text-tertiary">
              {formatDateTime(result.checked_at)}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function stringifyEmailCheck(value: unknown): string {
  if (typeof value === "boolean") return value ? "✓" : "✗";
  if (value == null) return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/**
 * Render one "special" settings field with a graphical control: chips for plain
 * string lists, a searchable model picker for the scheduler rollout scope, a
 * key→value editor for the email-template map, and a JSON box only for the
 * freeform custom-menus array (which has no fixed shape).
 */
function SpecialFieldRow({
  field,
  draft,
  onChange,
  modelOptions,
}: {
  field: SpecialField;
  draft: AdminSettingsDraft;
  onChange: (key: keyof AdminSettingsDraft, value: unknown) => void;
  modelOptions: MultiSelectOption[];
}) {
  const { t } = useLanguage();
  const id = `s-${String(field.key)}`;
  const label = fieldLabel(field.skip, t);
  const value = draft[field.key];

  if (field.kind === "tags") {
    const tags = Array.isArray(value) ? (value as string[]) : [];
    const hintId = `adminSettings.fields.${field.skip}_hint`;
    const hint = t(hintId);
    return (
      <div>
        <Label htmlFor={id}>{label}</Label>
        <div className="mt-1.5">
          <TagInput id={id} value={tags} onChange={(next) => onChange(field.key, next)} />
        </div>
        {hint !== hintId ? (
          <p className="mt-1 text-2xs text-srapi-text-tertiary">{hint}</p>
        ) : null}
      </div>
    );
  }

  if (field.kind === "models") {
    const selected = Array.isArray(value) ? (value as string[]) : [];
    return (
      <div>
        <Label htmlFor={id}>{label}</Label>
        <div className="mt-1.5">
          <MultiSelect
            id={id}
            value={selected}
            onChange={(next) => onChange(field.key, next)}
            options={modelOptions}
            allowCustom
            placeholder={t("adminSettings.allModels")}
            searchPlaceholder={t("adminCommon.search")}
            emptyText={t("adminCommon.noResults")}
            addCustomLabel={(q) => t("adminCommon.addValue", { value: q })}
          />
        </div>
        <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("adminSettings.rolloutModelsHint")}</p>
      </div>
    );
  }

  if (field.kind === "templates") {
    const map =
      value && typeof value === "object" && !Array.isArray(value)
        ? (value as Record<string, string>)
        : {};
    return (
      <div>
        <Label>{label}</Label>
        <div className="mt-1.5">
          <KeyValueEditor
            value={map}
            onChange={(next) => onChange(field.key, next)}
            addLabel={t("adminSettings.addTemplate")}
            keyPlaceholder={t("adminSettings.templateKeyPlaceholder")}
            valuePlaceholder={t("adminSettings.templateValuePlaceholder")}
          />
        </div>
      </div>
    );
  }

  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      <Textarea
        id={id}
        className="min-h-28 font-mono text-xs"
        spellCheck={false}
        value={String(value ?? "")}
        onChange={(e) => onChange(field.key, e.target.value)}
      />
      <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("adminSettings.customMenusHint")}</p>
    </div>
  );
}

/** Auto-render the primitive fields of a settings section as typed inputs. */
function SectionFields({
  section,
  value,
  onChange,
}: {
  section: SettingsTab;
  value: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
}) {
  const { t } = useLanguage();
  const skip = useMemo(
    () => new Set((SPECIAL_FIELDS[section] ?? []).map((f) => f.skip)),
    [section],
  );

  const entries = Object.entries(value).filter(([key, v]) => {
    if (skip.has(key)) return false;
    const type = typeof v;
    return v === null || type === "boolean" || type === "number" || type === "string";
  });

  if (entries.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-5 sm:grid-cols-2">
      {entries.map(([key, v]) => {
        const id = `f-${section}-${key}`;
        if (typeof v === "boolean") {
          return (
            <div key={key} className="flex items-center justify-between gap-4 sm:col-span-2">
              <Label htmlFor={id} className="mb-0">
                {fieldLabel(key, t)}
              </Label>
              <Switch id={id} checked={v} onCheckedChange={(checked) => onChange(key, checked)} />
            </div>
          );
        }
        if (typeof v === "number") {
          const clamp = GATEWAY_NON_NEGATIVE_INT_FIELDS.has(key)
            ? (n: number) => (Number.isFinite(n) ? Math.max(0, Math.trunc(n)) : 0)
            : (n: number) => n;
          return (
            <div key={key}>
              <Label htmlFor={id}>{fieldLabel(key, t)}</Label>
              <Input
                id={id}
                type="number"
                min={GATEWAY_NON_NEGATIVE_INT_FIELDS.has(key) ? 0 : undefined}
                value={String(v)}
                onChange={(e) =>
                  onChange(key, e.target.value === "" ? 0 : clamp(Number(e.target.value)))
                }
              />
            </div>
          );
        }
        return (
          <div key={key}>
            <Label htmlFor={id}>{fieldLabel(key, t)}</Label>
            <Input
              id={id}
              value={v == null ? "" : String(v)}
              onChange={(e) => onChange(key, e.target.value)}
            />
          </div>
        );
      })}
    </div>
  );
}

/**
 * Copilot tab: configure the admin AI copilot — enable it, choose which LLM
 * powers it (an existing provider account, or a standalone dedicated key), and
 * tune its autonomy. The dedicated API key is write-only.
 */
function CopilotTab({
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
            <Select value={value.source} onValueChange={(v) => onField("source", v)}>
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
              onValueChange={(v) => onField("provider_account_id", Number(v))}
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
                  <SelectItem value="openai-compatible">OpenAI-compatible</SelectItem>
                  <SelectItem value="anthropic-compatible">Anthropic-compatible</SelectItem>
                  <SelectItem value="gemini-compatible">Gemini-compatible</SelectItem>
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

        <div className="grid gap-5 sm:grid-cols-2">
          <div>
            <Label htmlFor="copilot-max-steps">{t("copilot.fieldMaxSteps")}</Label>
            <Input
              id="copilot-max-steps"
              type="number"
              min={1}
              max={20}
              value={String(value.max_steps)}
              onChange={(e) => onField("max_steps", e.target.value === "" ? 1 : Number(e.target.value))}
            />
          </div>
        </div>

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

/** Backup tab: export the full config snapshot as JSON, or import one (dry-run first). */
function BackupTab() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const snapshot = useConfigSnapshot();
  const importMut = useImportConfigSnapshot();
  const [importText, setImportText] = useState("");
  const [dryRun, setDryRun] = useState(true);
  const [result, setResult] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const snapshotJson = snapshot.data ? JSON.stringify(snapshot.data, null, 2) : "";

  async function copySnapshot() {
    if (!snapshotJson) return;
    await navigator.clipboard.writeText(snapshotJson);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  async function runImport() {
    setResult(null);
    let body;
    try {
      body = JSON.parse(importText || "{}");
    } catch {
      toast({ title: t("feedback.failed"), description: "Invalid JSON", tone: "error" });
      return;
    }
    try {
      const res = await importMut.mutateAsync({ body, dryRun });
      setResult(JSON.stringify(res, null, 2));
      toast({ title: dryRun ? t("adminSettings.dryRun") : t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="font-serif text-lg text-srapi-text-primary">{t("adminSettings.export")}</h3>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => snapshot.refetch()} loading={snapshot.isFetching}>
                {t("adminSettings.fetchSnapshot")}
              </Button>
              {snapshotJson ? (
                <Button variant="outline" size="sm" onClick={copySnapshot}>
                  {copied ? <Check className="size-4 text-srapi-success" /> : <Copy className="size-4" />}
                  {t("common.copy")}
                </Button>
              ) : null}
            </div>
          </div>
          <p className="text-2xs text-srapi-text-tertiary">{t("adminSettings.exportHint")}</p>
          <Textarea
            readOnly
            className="min-h-64 font-mono text-xs"
            value={snapshotJson}
            placeholder={t("adminSettings.fetchSnapshot")}
          />
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-3">
          <h3 className="font-serif text-lg text-srapi-text-primary">{t("adminSettings.import")}</h3>
          <p className="text-2xs text-srapi-text-tertiary">{t("adminSettings.importHint")}</p>
          <Textarea
            className="min-h-48 font-mono text-xs"
            spellCheck={false}
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            placeholder='{ "providers": [], "models": [] }'
          />
          <div className="flex items-center justify-between gap-4">
            <label className="flex items-center gap-2 text-sm text-srapi-text-secondary">
              <Switch checked={dryRun} onCheckedChange={setDryRun} />
              {t("adminSettings.dryRun")}
            </label>
            <Button
              variant={dryRun ? "outline" : "primary"}
              size="sm"
              loading={importMut.isPending}
              disabled={!importText.trim()}
              onClick={runImport}
            >
              {dryRun ? t("adminSettings.dryRun") : t("adminSettings.applyImport")}
            </Button>
          </div>
          {result ? (
            <div>
              <Label>{t("adminSettings.importResult")}</Label>
              <Textarea readOnly className="min-h-32 font-mono text-xs" value={result} />
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
