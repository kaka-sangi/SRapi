"use client";

import { useState } from "react";
import { Scale } from "lucide-react";
import {
  AdminListView,
  ListCount,
  type Column,
} from "@/components/admin/admin-list-view";
import { PageHeader } from "@/components/layout/page-header";
import {
  ResourceFormDialog,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAffiliateManualAdjustments,
  useCreateAffiliateManualAdjustment,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { Button } from "@/components/ui/button";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import type { AffiliateLedgerEntry } from "@/lib/sdk-types";

// Admin recovery / corrections — credit or debit a user's affiliate balance
// outside the normal accrual/payout flow. Negative amounts debit; positive
// credit. The backend rejects debits that would go below the available
// affiliate balance (modules/affiliate/service/service.go:560) so the admin
// can't accidentally pull a user into negative.
type FormState = {
  user_id: string;
  amount: string;
  currency: string;
  reason: string;
  reference_id: string;
};

function emptyForm(): FormState {
  return { user_id: "", amount: "", currency: "USD", reason: "", reference_id: "" };
}

export function ManualAdjustmentsPanel() {
  const { t } = useLanguage();
  const query = useAffiliateManualAdjustments();
  const createMut = useCreateAffiliateManualAdjustment();
  const userLookup = useUserEmailLookup();
  const [creating, setCreating] = useState(false);

  const fields: FieldConfig<FormState>[] = [
    {
      name: "user_id",
      label: t("adminAffiliates.adjustmentUserId"),
      required: true,
      validate: (value) => {
        const n = Number(value);
        if (!Number.isInteger(n) || n <= 0) return t("adminAffiliates.adjustmentUserIdHint");
        return undefined;
      },
    },
    {
      name: "amount",
      label: t("adminAffiliates.adjustmentAmount"),
      required: true,
      placeholder: "e.g. 10.00 or -5.00",
      validate: (value) => {
        if (!/^-?\d+(\.\d+)?$/.test(value.trim())) return t("adminAffiliates.adjustmentAmountHint");
        if (Number(value) === 0) return t("adminAffiliates.adjustmentAmountHint");
        return undefined;
      },
    },
    { name: "currency", label: t("adminCommon.currency") },
    {
      name: "reason",
      label: t("adminAffiliates.adjustmentReason"),
      type: "textarea",
      required: true,
    },
    {
      name: "reference_id",
      label: t("adminAffiliates.adjustmentReference"),
      placeholder: t("adminAffiliates.adjustmentReferenceHint"),
    },
  ];

  const columns: Column<AffiliateLedgerEntry>[] = [
    {
      key: "date",
      header: t("adminAffiliates.date"),
      render: (r) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.created_at)}
        </span>
      ),
    },
    {
      key: "user",
      header: t("adminAffiliates.inviter"),
      render: (r) => (
        <span className="text-srapi-text-secondary">{userLookup.get(r.user_id)}</span>
      ),
    },
    {
      key: "amount",
      header: t("adminAffiliates.amount"),
      align: "right",
      render: (r) => {
        const negative = String(r.amount).startsWith("-");
        return (
          <span
            className={
              "font-mono tabular " +
              (negative ? "text-srapi-error" : "text-srapi-success")
            }
          >
            {formatMoney(r.amount, r.currency)}
          </span>
        );
      },
    },
    {
      key: "reason",
      header: t("adminAffiliates.adjustmentReason"),
      hideOnMobile: true,
      render: (r) => {
        const reason =
          r.metadata && typeof r.metadata === "object" && "reason" in r.metadata
            ? String((r.metadata as Record<string, unknown>).reason ?? "")
            : "";
        return <span className="text-srapi-text-secondary">{reason || "—"}</span>;
      },
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminAffiliates.adjustmentsTitle")}
        description={t("adminAffiliates.adjustmentsSubtitle")}
        actions={
          <div className="flex items-center gap-2">
            {query.data ? (
              <ListCount total={query.data.pagination?.total ?? query.data.data.length} />
            ) : null}
            <Button type="button" variant="primary" size="sm" onClick={() => setCreating(true)}>
              ＋ {t("adminAffiliates.createAdjustment")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(r) => r.id}
        emptyIcon={Scale}
        emptyTitle={t("adminAffiliates.emptyAdjustments")}
        emptyBody={t("adminAffiliates.emptyAdjustmentsBody")}
        minWidth={620}
      />

      {creating ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreating}
          title={t("adminAffiliates.createAdjustment")}
          fields={fields}
          initial={emptyForm()}
          buildBody={(form) => ({
            user_id: form.user_id.trim(),
            amount: form.amount.trim(),
            currency: form.currency.trim() || undefined,
            reason: form.reason.trim(),
            reference_id: form.reference_id.trim() || undefined,
          })}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : null}
    </>
  );
}
