"use client";

import { useState } from "react";
import { CreditCard } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminSubscriptionPlans,
  useAdminSubscriptions,
  useAdminUsers,
  useCreateUserSubscription,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney, formatDate } from "@/lib/admin-format";
import {
  USER_SUBSCRIPTION_STATUSES,
  emptyUserSubscriptionForm,
  buildCreateUserSubscriptionBody,
  type UserSubscriptionFormState,
} from "@/lib/admin-subscription-form";
import type {
  SubscriptionPlan,
  UserSubscription,
} from "@/lib/sdk-types";

export default function AdminSubscriptionsPage() {
  return (
    <AdminShell>
      <SubscriptionsContent />
    </AdminShell>
  );
}

function SubscriptionsContent() {
  const { t } = useLanguage();
  const plans = useAdminSubscriptionPlans();
  const subs = useAdminSubscriptions();
  const users = useAdminUsers();
  const createSub = useCreateUserSubscription();
  const [creatingSub, setCreatingSub] = useState(false);

  const subFields: FieldConfig<UserSubscriptionFormState>[] = [
    {
      name: "userId",
      label: t("adminSubscriptions.user"),
      type: "select",
      options: (users.data?.data ?? []).map((u) => ({ value: u.id, label: u.email })),
    },
    {
      name: "planId",
      label: t("adminSubscriptions.plan"),
      type: "select",
      options: (plans.data?.data ?? []).map((p) => ({ value: p.id, label: p.name })),
    },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(USER_SUBSCRIPTION_STATUSES),
    },
    { name: "startsAtLocal", label: t("adminCommon.startsAt"), type: "datetime" },
    { name: "expiresAtLocal", label: t("adminCommon.expiresAt"), type: "datetime" },
  ];

  const planColumns: Column<SubscriptionPlan>[] = [
    {
      key: "name",
      header: t("adminSubscriptions.plan"),
      render: (p) => <span className="text-srapi-text-primary">{p.name}</span>,
    },
    {
      key: "price",
      header: t("adminSubscriptions.price"),
      align: "right",
      render: (p) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(p.price, p.currency)} / {t("adminSubscriptions.validity", { days: p.validity_days })}
        </span>
      ),
    },
    {
      key: "status",
      header: t("common.active"),
      render: (p) => <QuietBadge status={quietStatusFor(p.status)} label={statusLabel(t, p.status)} />,
    },
  ];

  const subColumns: Column<UserSubscription>[] = [
    {
      key: "user",
      header: t("adminSubscriptions.user"),
      render: (s) => <span className="font-mono text-2xs text-srapi-text-secondary">{s.user_id}</span>,
    },
    {
      key: "plan",
      header: t("adminSubscriptions.plan"),
      render: (s) => <span className="font-mono text-2xs text-srapi-text-secondary">{s.plan_id}</span>,
    },
    {
      key: "period",
      header: t("adminSubscriptions.period"),
      hideOnMobile: true,
      render: (s) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDate(s.starts_at)} – {formatDate(s.expires_at)}
        </span>
      ),
    },
    {
      key: "status",
      header: t("common.active"),
      render: (s) => <QuietBadge status={quietStatusFor(s.status)} label={statusLabel(t, s.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminSubscriptions.title")}
        description={t("adminSubscriptions.subtitle")}
        actions={
          <Button variant="primary" size="sm" onClick={() => setCreatingSub(true)}>
            ＋ {t("adminSubscriptions.createSubscription")}
          </Button>
        }
      />
      <Tabs defaultValue="plans">
        <TabsList>
          <TabsTrigger value="plans">{t("adminSubscriptions.tabPlans")}</TabsTrigger>
          <TabsTrigger value="subs">{t("adminSubscriptions.tabSubscriptions")}</TabsTrigger>
        </TabsList>
        <TabsContent value="plans">
          <AdminListView
            query={plans}
            columns={planColumns}
            getRowId={(p) => p.id}
            emptyIcon={CreditCard}
            emptyTitle={t("adminSubscriptions.emptyPlans")}
            emptyBody={t("adminSubscriptions.emptyPlansBody")}
            minWidth={480}
          />
        </TabsContent>
        <TabsContent value="subs">
          <AdminListView
            query={subs}
            columns={subColumns}
            getRowId={(s) => s.id}
            emptyIcon={CreditCard}
            emptyTitle={t("adminSubscriptions.emptySubs")}
            emptyBody={t("adminSubscriptions.emptySubsBody")}
            minWidth={560}
          />
        </TabsContent>
      </Tabs>

      {creatingSub ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreatingSub}
          title={t("adminSubscriptions.createSubscription")}
          fields={subFields}
          initial={emptyUserSubscriptionForm()}
          buildBody={buildCreateUserSubscriptionBody}
          submit={(body) => createSub.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createSub.isPending}
        />
      ) : null}
    </>
  );
}
