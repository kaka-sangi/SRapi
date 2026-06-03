"use client";

import { useState } from "react";
import { Coins } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { useAffiliate, useAffiliateLedger, useTransferToBalance } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import { meErrorMessage } from "@/lib/me-api";
import type { AffiliateLedgerEntry } from "@/lib/sdk-types";

export default function AffiliatePage() {
  return (
    <AppShell allowedRole="user">
      <AffiliateContent />
    </AppShell>
  );
}

function AffiliateContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const affiliate = useAffiliate();
  const ledger = useAffiliateLedger();
  const transferMut = useTransferToBalance();

  const primary = affiliate.data?.balances?.[0];
  const [amount, setAmount] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function transfer(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    try {
      await transferMut.mutateAsync({
        amount: amount.trim(),
        currency: primary?.currency,
      });
      toast({ title: t("feedback.saved"), tone: "success" });
      setAmount("");
    } catch (err) {
      setError(meErrorMessage(err));
    }
  }

  const columns: Column<AffiliateLedgerEntry>[] = [
    {
      key: "date",
      header: t("affiliate.date"),
      render: (r) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.created_at)}
        </span>
      ),
    },
    {
      key: "type",
      header: t("affiliate.type"),
      render: (r) => <span className="text-srapi-text-secondary">{r.type || "—"}</span>,
    },
    {
      key: "amount",
      header: t("affiliate.amount"),
      align: "right",
      render: (r) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(r.amount, r.currency)}
        </span>
      ),
    },
    {
      key: "status",
      header: t("billing.status"),
      render: (r) => <QuietBadge status={quietStatusFor(r.status)} label={statusLabel(t, r.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAccount")}
        title={t("affiliate.title")}
        description={t("affiliate.subtitle")}
      />

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardContent className="flex flex-wrap items-baseline gap-x-10 gap-y-4">
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("affiliate.available")}
              </div>
              {affiliate.isLoading ? (
                <Skeleton className="mt-2 h-9 w-28" />
              ) : (
                <div className="mt-1 font-serif text-3xl text-srapi-text-primary tabular">
                  {primary ? formatMoney(primary.available_balance, primary.currency) : "—"}
                </div>
              )}
            </div>
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("affiliate.accrued")}
              </div>
              <div className="mt-1 font-serif text-3xl text-srapi-text-tertiary tabular">
                {primary ? formatMoney(primary.accrued_amount, primary.currency) : "—"}
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <form onSubmit={transfer} className="space-y-4">
              <h3 className="font-serif text-lg text-srapi-text-primary">{t("affiliate.transfer")}</h3>
              <div>
                <Label htmlFor="amount">{t("affiliate.transferAmount")}</Label>
                <Input
                  id="amount"
                  inputMode="decimal"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  disabled={transferMut.isPending}
                />
              </div>
              {error ? (
                <p role="alert" className="text-sm text-srapi-error">
                  {error}
                </p>
              ) : null}
              <Button
                type="submit"
                variant="primary"
                loading={transferMut.isPending}
                disabled={!amount.trim()}
              >
                {t("affiliate.transfer")}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>

      <AdminListView
        query={ledger}
        columns={columns}
        getRowId={(r) => r.id}
        emptyIcon={Coins}
        emptyTitle={t("affiliate.emptyLedger")}
        emptyBody={t("affiliate.emptyLedgerBody")}
        minWidth={520}
      />
    </>
  );
}
