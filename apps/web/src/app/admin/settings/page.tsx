"use client";

import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { CheckCircle2, CircleAlert, Loader2 } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminSettings, useUpdateSettings, useAdminModels } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { FormSkeleton } from "@/components/charts/chart-skeleton";
import { type MultiSelectOption } from "@/components/ui/multi-select";
import { adminErrorMessage } from "@/lib/admin-api";
import { ADMIN_ROUTES } from "@/lib/routes";
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
import { SPECIAL_FIELDS } from "./settings-fields";
import { SectionFields } from "./section-fields";
import { SpecialFieldRow } from "./special-field-row";
import { EmailTestPanel } from "./email-test-panel";
import { CopilotTab } from "./copilot-tab";
import { BackupTab } from "./backup-tab";
import { CaptchaSettingsPanel } from "./captcha-settings-panel";

export default function AdminSettingsPage() {
  return (
    <AdminShell>
      <Suspense>
        <SettingsContent />
      </Suspense>
    </AdminShell>
  );
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
      <PageQueryState query={settings} skeleton={<FormSkeleton rows={6} className="p-5" />}>
        {(data) => <SettingsEditor initial={data} />}
      </PageQueryState>
    </>
  );
}

function isSettingsTab(value: string | null): value is SettingsTab {
  return value !== null && SETTINGS_TABS.some((tab) => tab.id === value);
}

function SettingsEditor({ initial }: { initial: Parameters<typeof createSettingsDraft>[0] }) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const router = useRouter();
  const params = useSearchParams();
  const rawTab = params.get("tab");
  const activeTab: SettingsTab = isSettingsTab(rawTab) ? rawTab : "general";
  const updateMut = useUpdateSettings();
  const models = useAdminModels();
  const modelOptions: MultiSelectOption[] = (models.data?.data ?? []).map((m) => ({
    value: m.canonical_name ?? m.id,
    label: m.canonical_name ?? m.id,
  }));
  const initialSignature = JSON.stringify(initial);
  const initialSignatureRef = useRef(initialSignature);
  const [draft, setDraft] = useState<AdminSettingsDraft>(() => createSettingsDraft(initial));
  const [savedDraft, setSavedDraft] = useState<AdminSettingsDraft>(() => createSettingsDraft(initial));
  const [confirmTab, setConfirmTab] = useState<SettingsTab | null>(null);

  useEffect(() => {
    if (initialSignatureRef.current === initialSignature) return;
    initialSignatureRef.current = initialSignature;
    const next = createSettingsDraft(initial);
    setDraft(next);
    setSavedDraft(next);
  }, [initial, initialSignature]);

  const isDirty = useMemo(
    () => JSON.stringify(draft) !== JSON.stringify(savedDraft),
    [draft, savedDraft],
  );

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
      const saved = await updateMut.mutateAsync(body);
      const normalizedDraft = createSettingsDraft(saved);
      setDraft(normalizedDraft);
      setSavedDraft(normalizedDraft);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
      throw err;
    }
  }

  function requestSave(tab: SettingsTab) {
    if (settingsTabRequiresConfirmation(tab)) {
      setConfirmTab(tab);
    } else {
      // save() now re-throws so the confirm dialog can detect failure; the error
      // is already toasted, so swallow the rejection at this fire-and-forget site.
      void save().catch(() => {});
    }
  }

  function setTab(next: string) {
    const q = new URLSearchParams();
    if (next !== "general") q.set("tab", next);
    const qs = q.toString();
    router.replace(`${ADMIN_ROUTES.settings}${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  return (
    <Tabs value={activeTab} onValueChange={setTab}>
      <TabsList className="flex-wrap">
        {SETTINGS_TABS.map((tab) => (
          <TabsTrigger key={tab.id} value={tab.id}>
            {t(`adminSettings.tabs.${tab.id}`)}
          </TabsTrigger>
        ))}
      </TabsList>
      <SettingsSaveState dirty={isDirty} pending={updateMut.isPending} />

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
                {tab.id === "security" ? <CaptchaSettingsPanel /> : null}
                <div className="border-srapi-border flex justify-end border-t pt-4">
                  <Button
                    variant="primary"
                    loading={updateMut.isPending}
                    disabled={!isDirty}
                    onClick={() => requestSave(tab.id)}
                  >
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
          isPending={updateMut.isPending}
        />
      ) : null}
    </Tabs>
  );
}

function SettingsSaveState({ dirty, pending }: { dirty: boolean; pending: boolean }) {
  const { t } = useLanguage();
  const icon = pending ? (
    <Loader2 className="size-3.5 animate-spin" aria-hidden />
  ) : dirty ? (
    <CircleAlert className="size-3.5 text-srapi-warning" aria-hidden />
  ) : (
    <CheckCircle2 className="size-3.5 text-srapi-success" aria-hidden />
  );
  const label = pending
    ? t("adminSettings.saveState.saving")
    : dirty
      ? t("adminSettings.saveState.unsaved")
      : t("adminSettings.saveState.saved");

  return (
    <div className="mt-3 flex items-center justify-end font-mono text-2xs text-srapi-text-tertiary">
      <span className="inline-flex items-center gap-1.5 rounded-md border border-srapi-border px-2 py-1">
        {icon}
        {label}
      </span>
    </div>
  );
}
