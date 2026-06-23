"use client";

import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { CheckCircle2, CircleAlert, Loader2 } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { SectionHero } from "@/components/visual/section-hero";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminSettings, useUpdateSettings, useAdminModels } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { FormSkeleton } from "@/components/charts/chart-skeleton";
import { type MultiSelectOption } from "@/components/ui/multi-select";
import { KbdShortcut } from "@/components/ui/kbd";
import { DataPill } from "@/components/ui/data-pill";
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
import { MaintenanceTab } from "./maintenance-tab";
import { GeneralTab } from "./general-tab";
import { AgreementTab } from "./agreement-tab";
import { FeaturesTab } from "./features-tab";
import { SecurityTab } from "./security-tab";
import { UsersTab } from "./users-tab";
import { GatewayTab } from "./gateway-tab";
import { PaymentTab } from "./payment-tab";
import { EmailTab } from "./email-tab";
import type { AdminSettingsMaintenance } from "../../../../../../packages/sdk/typescript/src/types.gen";

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
      <SectionHero
        eyebrow="System · Settings"
        title={t("adminSettings.title")}
        description="站点 / 安全 / 限速 / 集成"
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

  // Count changed top-level keys within the active section so we can surface
  // a precise «N changed» badge on the sticky save bar.
  const dirtyFieldCount = useMemo(() => {
    if (!isDirty) return 0;
    const cur = (draft.value[activeTab] as Record<string, unknown>) ?? {};
    const saved = (savedDraft.value[activeTab] as Record<string, unknown>) ?? {};
    const keys = new Set([...Object.keys(cur), ...Object.keys(saved)]);
    let n = 0;
    keys.forEach((k) => {
      if (JSON.stringify(cur[k]) !== JSON.stringify(saved[k])) n++;
    });
    // Also factor in any special / top-level draft fields that diverged.
    (Object.keys(draft) as (keyof AdminSettingsDraft)[]).forEach((k) => {
      if (k === "value") return;
      if (JSON.stringify(draft[k]) !== JSON.stringify(savedDraft[k])) n++;
    });
    return n;
  }, [draft, savedDraft, activeTab, isDirty]);

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

  // ⌘S / Ctrl+S → trigger the same save flow as the section button, so the
  // sticky save bar's Kbd hint is wired to a real shortcut. We intentionally
  // do not intercept when no diff exists (avoids fighting browser defaults).
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      const isSave =
        (event.metaKey || event.ctrlKey) && (event.key === "s" || event.key === "S");
      if (!isSave) return;
      if (!isDirty || updateMut.isPending) return;
      event.preventDefault();
      requestSave(activeTab);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // requestSave is stable enough — re-bind only when dirty/active changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isDirty, updateMut.isPending, activeTab]);

  return (
    <Tabs value={activeTab} onValueChange={setTab}>
      <TabsList className="flex-wrap">
        {SETTINGS_TABS.map((tab) => (
          <TabsTrigger key={tab.id} value={tab.id}>
            {t(`adminSettings.tabs.${tab.id}`)}
          </TabsTrigger>
        ))}
      </TabsList>
      <SettingsSaveState
        dirty={isDirty}
        pending={updateMut.isPending}
        dirtyCount={dirtyFieldCount}
        onSave={isDirty && !updateMut.isPending ? () => requestSave(activeTab) : undefined}
      />

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
          ) : tab.id === "maintenance" ? (
            <MaintenanceTab
              value={draft.value.maintenance as AdminSettingsMaintenance}
              onField={(key, v) => setSectionField("maintenance", key, v)}
              onSave={() => requestSave("maintenance")}
              pending={updateMut.isPending}
            />
          ) : tab.id === "security" ? (
            <SecurityTab
              value={draft.value.security as Record<string, unknown>}
              draft={draft}
              onField={(key, v) => setSectionField("security", key, v)}
              onSpecial={setSpecial}
              onSave={() => requestSave("security")}
              pending={updateMut.isPending}
              modelOptions={modelOptions}
            />
          ) : tab.id === "payment" ? (
            <PaymentTab
              paymentValue={draft.value.payment as Record<string, unknown>}
              featuresValue={draft.value.features as Record<string, unknown>}
              draft={draft}
              onPaymentField={(key, v) => setSectionField("payment", key, v)}
              onFeaturesField={(key, v) => setSectionField("features", key, v)}
              onSpecial={setSpecial}
              onSave={() => requestSave("payment")}
              pending={updateMut.isPending}
              modelOptions={modelOptions}
            />
          ) : tab.id === "general" ? (
            <GeneralTab
              value={draft.value.general as Record<string, unknown>}
              draft={draft}
              onField={(key, v) => setSectionField("general", key, v)}
              onSpecial={setSpecial}
              onSave={() => requestSave("general")}
              pending={updateMut.isPending}
              modelOptions={modelOptions}
            />
          ) : tab.id === "agreement" ? (
            <AgreementTab
              value={draft.value.agreement as Record<string, unknown>}
              onField={(key, v) => setSectionField("agreement", key, v)}
              onSave={() => requestSave("agreement")}
              pending={updateMut.isPending}
            />
          ) : tab.id === "features" ? (
            <FeaturesTab
              value={draft.value.features as Record<string, unknown>}
              draft={draft}
              onField={(key, v) => setSectionField("features", key, v)}
              onSpecial={setSpecial}
              onSave={() => requestSave("features")}
              pending={updateMut.isPending}
              modelOptions={modelOptions}
            />
          ) : tab.id === "users" ? (
            <UsersTab
              value={draft.value.users as Record<string, unknown>}
              onField={(key, v) => setSectionField("users", key, v)}
              onSave={() => requestSave("users")}
              pending={updateMut.isPending}
            />
          ) : tab.id === "gateway" ? (
            <GatewayTab
              value={draft.value.gateway as Record<string, unknown>}
              draft={draft}
              onField={(key, v) => setSectionField("gateway", key, v)}
              onSpecial={setSpecial}
              onSave={() => requestSave("gateway")}
              pending={updateMut.isPending}
              modelOptions={modelOptions}
            />
          ) : tab.id === "email" ? (
            <EmailTab
              value={draft.value.email as Record<string, unknown>}
              draft={draft}
              onField={(key, v) => setSectionField("email", key, v)}
              onSpecial={setSpecial}
              onSave={() => requestSave("email")}
              pending={updateMut.isPending}
              modelOptions={modelOptions}
            />
          ) : null}
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

function SettingsSaveState({
  dirty,
  pending,
  dirtyCount,
  onSave,
}: {
  dirty: boolean;
  pending: boolean;
  dirtyCount: number;
  onSave?: () => void;
}) {
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
    // Sticky so it never scrolls out of view inside long settings tabs. The
    // pill stays right-aligned; the save shortcut chip floats next to it
    // whenever there's a real diff to flush.
    <div className="sticky top-2 z-20 mt-3 flex items-center justify-end gap-2 backdrop-blur-sm">
      {dirty ? (
        <DataPill tone="warning" size="sm" className="tabular">
          {dirtyCount} {dirtyCount === 1 ? "field" : "fields"}
        </DataPill>
      ) : null}
      <span
        className={
          "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium shadow-sm ring-1 " +
          (dirty
            ? "bg-srapi-warning/10 text-srapi-warning ring-srapi-warning/30"
            : "bg-srapi-card-muted text-srapi-text-secondary ring-srapi-border/60")
        }
      >
        {icon}
        {label}
      </span>
      {onSave ? (
        <button
          type="button"
          onClick={onSave}
          className="inline-flex items-center gap-2 rounded-full bg-srapi-primary px-3 py-1 text-[11px] font-semibold text-white shadow-sm transition-colors hover:bg-srapi-primary-hover"
        >
          {t("common.save")}
          <KbdShortcut keys={["⌘", "S"]} />
        </button>
      ) : null}
    </div>
  );
}
