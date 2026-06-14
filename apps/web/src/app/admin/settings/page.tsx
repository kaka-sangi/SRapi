"use client";

import { useState } from "react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  useAdminSettings,
  useUpdateSettings,
  useAdminModels,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { FormSkeleton } from "@/components/charts/chart-skeleton";
import { type MultiSelectOption } from "@/components/ui/multi-select";
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
import { SPECIAL_FIELDS } from "./settings-fields";
import { SectionFields } from "./section-fields";
import { SpecialFieldRow } from "./special-field-row";
import { EmailTestPanel } from "./email-test-panel";
import { CopilotTab } from "./copilot-tab";
import { BackupTab } from "./backup-tab";

export default function AdminSettingsPage() {
  return (
    <AdminShell>
      <SettingsContent />
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
      <PageQueryState
        query={settings}
        skeleton={<FormSkeleton rows={6} className="p-5" />}
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
