"use client";

import { useState } from "react";
import { Coins, Link2, UserPlus, WalletCards, Wallet, TrendingUp } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { SectionHero } from "@/components/visual/section-hero";
import {
  useAffiliate,
  useAffiliateInviteCodes,
  useAffiliateLedger,
  useCreateAffiliateInviteCode,
  useRequestAffiliateWithdrawal,
  useTransferToBalance,
} from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { IconBubble } from "@/components/ui/icon-bubble";
import { CopyButton, CopyableValue } from "@/components/ui/copy-button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import { meErrorMessage } from "@/lib/me-api";
import type { AffiliateInviteCode, AffiliateLedgerEntry } from "@/lib/sdk-types";

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
  const inviteCodes = useAffiliateInviteCodes();
  const ledger = useAffiliateLedger();
  const transferMut = useTransferToBalance();
  const createInviteMut = useCreateAffiliateInviteCode();
  const withdrawMut = useRequestAffiliateWithdrawal();

  const primary = affiliate.data?.balances?.[0];
  const codes = affiliate.data?.invite_codes?.length
    ? affiliate.data.invite_codes
    : inviteCodes.data?.data ?? [];
  const [amount, setAmount] = useState("");
  const [withdrawAmount, setWithdrawAmount] = useState("");
  const [withdrawDestination, setWithdrawDestination] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [withdrawError, setWithdrawError] = useState<string | null>(null);

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

  async function createInviteCode() {
    try {
      await createInviteMut.mutateAsync({});
      toast({ title: t("feedback.created"), tone: "success" });
    } catch (err) {
      toast({ title: meErrorMessage(err), tone: "error" });
    }
  }

  async function requestWithdrawal(event: React.FormEvent) {
    event.preventDefault();
    setWithdrawError(null);
    try {
      await withdrawMut.mutateAsync({
        amount: withdrawAmount.trim(),
        currency: primary?.currency,
        destination: withdrawDestination.trim() || undefined,
      });
      toast({ title: t("feedback.created"), tone: "success" });
      setWithdrawAmount("");
      setWithdrawDestination("");
    } catch (err) {
      setWithdrawError(meErrorMessage(err));
    }
  }

  const columns: Column<AffiliateLedgerEntry>[] = [
    {
      key: "date",
      header: t("affiliate.date"),
      render: (r) => (
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
          {formatDateTime(r.created_at)}
        </span>
      ),
    },
    {
      key: "type",
      header: t("affiliate.type"),
      render: (r) => <span className="text-sm text-srapi-text-secondary">{r.type || "—"}</span>,
    },
    {
      key: "amount",
      header: t("affiliate.amount"),
      align: "right",
      render: (r) => (
        <span className="text-[12px] font-medium tabular text-srapi-text-primary">
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
      <SectionHero
        eyebrow={t("affiliate.eyebrow")}
        title={t("affiliate.title")}
        description={t("affiliate.subtitle")}
        metrics={
          primary
            ? [
                {
                  label: t("affiliate.available"),
                  value: formatMoney(primary.available_balance, primary.currency),
                  tone: "success",
                },
                {
                  label: t("affiliate.accrued"),
                  value: formatMoney(primary.accrued_amount, primary.currency),
                },
              ]
            : undefined
        }
      />

      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="border-l-4 border-l-srapi-primary">
          <CardContent className="flex flex-wrap items-start gap-x-8 gap-y-4">
            <div className="flex items-start gap-3">
              <IconBubble tone="accent" size="lg">
                <Wallet aria-hidden />
              </IconBubble>
              <div>
                <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("affiliate.available")}
                </div>
                {affiliate.isLoading ? (
                  <Skeleton className="mt-2 h-9 w-28" />
                ) : (
                  <div className="mt-1 text-3xl font-semibold tracking-tight tabular text-srapi-text-primary">
                    {primary ? formatMoney(primary.available_balance, primary.currency) : "—"}
                  </div>
                )}
              </div>
            </div>
            <div className="hidden h-12 w-px self-center bg-srapi-border sm:block" />
            <div className="flex items-start gap-3">
              <IconBubble tone="neutral" size="lg">
                <TrendingUp aria-hidden />
              </IconBubble>
              <div>
                <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("affiliate.accrued")}
                </div>
                <div className="mt-1 text-3xl font-semibold tracking-tight tabular text-srapi-text-tertiary">
                  {primary ? formatMoney(primary.accrued_amount, primary.currency) : "—"}
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <form onSubmit={transfer} className="space-y-4">
              <h3 className="text-lg font-semibold tracking-tight text-srapi-text-primary">{t("affiliate.transfer")}</h3>
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
                disabled={!amount.trim() || !primary}
              >
                {t("affiliate.transfer")}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
                  {t("affiliate.inviteCodes")}
                </h3>
                <p className="mt-1 text-sm text-srapi-text-secondary">
                  {t("affiliate.invitedCount", { count: affiliate.data?.invited_count ?? 0 })}
                </p>
              </div>
              <Button
                type="button"
                variant="primary"
                size="sm"
                loading={createInviteMut.isPending}
                onClick={createInviteCode}
              >
                ＋ {t("affiliate.generateInviteCode")}
              </Button>
            </div>

            {inviteCodes.isLoading && !affiliate.data ? (
              <Skeleton className="h-16 w-full" />
            ) : codes.length > 0 ? (
              <div className="space-y-2">
                {codes.slice(0, 5).map((code) => (
                  <InviteCodeRow key={code.id} code={code} />
                ))}
              </div>
            ) : (
              <div className="flex min-h-28 flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-srapi-border/70 bg-srapi-card-muted/40 px-4 py-6 text-center">
                <IconBubble tone="accent">
                  <UserPlus aria-hidden />
                </IconBubble>
                <p className="text-sm text-srapi-text-secondary">
                  {t("affiliate.emptyInviteCodes")}
                </p>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <form onSubmit={requestWithdrawal} className="space-y-4">
              <h3 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
                {t("affiliate.withdraw")}
              </h3>
              <div className="grid gap-3 sm:grid-cols-2">
                <div>
                  <Label htmlFor="withdraw-amount">{t("affiliate.withdrawAmount")}</Label>
                  <Input
                    id="withdraw-amount"
                    inputMode="decimal"
                    value={withdrawAmount}
                    onChange={(e) => setWithdrawAmount(e.target.value)}
                    disabled={withdrawMut.isPending}
                  />
                </div>
                <div>
                  <Label htmlFor="withdraw-destination">
                    {t("affiliate.withdrawDestination")}
                  </Label>
                  <Input
                    id="withdraw-destination"
                    value={withdrawDestination}
                    onChange={(e) => setWithdrawDestination(e.target.value)}
                    disabled={withdrawMut.isPending}
                  />
                </div>
              </div>
              {withdrawError ? (
                <p role="alert" className="text-sm text-srapi-error">
                  {withdrawError}
                </p>
              ) : null}
              <Button
                type="submit"
                variant="outline"
                loading={withdrawMut.isPending}
                disabled={!withdrawAmount.trim() || !primary}
              >
                <WalletCards className="size-4" aria-hidden />
                {t("affiliate.withdraw")}
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

function InviteCodeRow({ code }: { code: AffiliateInviteCode }) {
  const { t } = useLanguage();
  const inviteLink = invitePathForCode(code.code);

  return (
    <div className="flex min-w-0 flex-wrap items-center justify-between gap-3 rounded-xl border border-srapi-border bg-srapi-card-muted px-3.5 py-2.5">
      <div className="min-w-0 flex-1">
        <CopyableValue value={code.code} label={t("affiliate.copyCode")} />
        <div className="mt-1 flex min-w-0 items-center gap-1 text-xs text-srapi-text-tertiary">
          <Link2 className="size-3 shrink-0" aria-hidden />
          <span className="truncate" title={inviteLink}>{inviteLink}</span>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <QuietBadge status={quietStatusFor(code.status)} label={statusLabel(t, code.status)} />
        <CopyButton value={inviteLink} label={t("affiliate.copyLink")} />
      </div>
    </div>
  );
}

function invitePathForCode(code: string): string {
  // Full shareable URL pointing at the real register route (/auth/register, not
  // /register), with the invite code the register page reads on submit.
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  return `${origin}/auth/register?invite_code=${encodeURIComponent(code)}`;
}
