"use client";

import { useMemo, useState } from "react";
import { Copy, Check } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  useAdminSettings,
  useUpdateSettings,
  useConfigSnapshot,
  useImportConfigSnapshot,
  useAdminModels,
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
import { Skeleton } from "@/components/ui/skeleton";
import { TagInput } from "@/components/ui/tag-input";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { KeyValueEditor } from "@/components/ui/key-value-editor";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  SETTINGS_TABS,
  type SettingsTab,
  type AdminSettingsDraft,
  createSettingsDraft,
  materializeSettingsDraft,
  settingsTabRequiresConfirmation,
  settingsSaveConfirmationPhrase,
} from "@/lib/admin-settings-form";

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
  ],
  payment: [{ key: "paymentProviders", kind: "tags", skip: "providers" }],
  email: [{ key: "emailTemplates", kind: "templates", skip: "templates" }],
};

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
    return (
      <div>
        <Label htmlFor={id}>{label}</Label>
        <div className="mt-1.5">
          <TagInput id={id} value={tags} onChange={(next) => onChange(field.key, next)} />
        </div>
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
          return (
            <div key={key}>
              <Label htmlFor={id}>{fieldLabel(key, t)}</Label>
              <Input
                id={id}
                type="number"
                value={String(v)}
                onChange={(e) => onChange(key, e.target.value === "" ? 0 : Number(e.target.value))}
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
