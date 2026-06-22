"use client";

import { useState } from "react";
import { UserPlus } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useAffiliateInvites, useBatchSetAffiliateRebateRate } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { formatDateTime } from "@/lib/admin-format";
import { adminErrorMessage } from "@/lib/admin-api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import type { AffiliateInviteRecord } from "@/lib/sdk-types";

// Panel body for /admin/affiliates?tab=invites. Identical UI to the legacy
// /admin/affiliates/invites page; the standalone page is now a redirect.
export function InvitesPanel() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const invites = useAffiliateInvites();
  const userLookup = useUserEmailLookup();
  const colVis = useColumnVisibility("admin-affiliate-invites", []);
  const batchRebate = useBatchSetAffiliateRebateRate();
  const [bulkRebateOpen, setBulkRebateOpen] = useState(false);

  // applyBulkRebate is the verbatim wiring for the port of sub2api's
  // AffiliateHandler.BatchSetRate. The dialog parses the operator's textarea
  // into items and the server applies them per-row with idempotent overlays.
  async function applyBulkRebate(items: { user_id: string; rate_percent?: number | null; clear?: boolean }[]) {
    if (items.length === 0) return;
    try {
      const result = await batchRebate.mutateAsync(items);
      const failedCount = result.errors.length;
      const succeededCount = result.updated_count;
      if (failedCount > 0 && succeededCount > 0) {
        toast({
          title: t("feedback.batchPartial", { succeeded: succeededCount, failed: failedCount }),
          tone: "warning",
        });
      } else if (failedCount > 0) {
        toast({ title: t("feedback.batchAllFailed", { count: items.length }), tone: "error" });
      } else {
        toast({ title: t("feedback.batchAllSucceeded", { count: succeededCount }), tone: "success" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const columns: Column<AffiliateInviteRecord>[] = [
    {
      key: "inviter",
      header: t("adminAffiliates.inviter"),
      pinned: true,
      render: (r) => (
        <span className="text-srapi-text-secondary">{userLookup.get(r.inviter_user_id)}</span>
      ),
    },
    {
      key: "invitee",
      header: t("adminAffiliates.invitee"),
      render: (r) => (
        <span className="text-srapi-text-secondary">{userLookup.get(r.invitee_user_id)}</span>
      ),
    },
    {
      key: "date",
      header: t("adminAffiliates.date"),
      align: "right",
      render: (r) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">
          {formatDateTime(r.created_at)}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminAffiliates.invitesTitle")}
        description={t("adminAffiliates.invitesSubtitle")}
        actions={
          <div className="flex items-center gap-3">
            {invites.data ? (
              <ListCount total={invites.data.pagination?.total ?? 0} />
            ) : null}
            <Button
              variant="outline"
              size="sm"
              loading={batchRebate.isPending}
              onClick={() => setBulkRebateOpen(true)}
            >
              {t("adminAffiliates.bulkRebateRate")}
            </Button>
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </div>
        }
      />
      <AdminListView
        query={invites}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(r) => r.id}
        emptyIcon={UserPlus}
        emptyTitle={t("adminAffiliates.emptyTitle")}
        emptyBody={t("adminAffiliates.emptyBody")}
        minWidth={520}
      />

      {bulkRebateOpen ? (
        <BulkRebateRateDialog
          isPending={batchRebate.isPending}
          onSubmit={async (items) => {
            await applyBulkRebate(items);
            setBulkRebateOpen(false);
          }}
          onClose={() => setBulkRebateOpen(false)}
        />
      ) : null}
    </>
  );
}

// Operator-friendly dialog for the verbatim port of sub2api's
// AffiliateHandler.BatchSetRate. One row per line in `user_id,rate` format
// (or `user_id,clear` to remove the override). Empty lines are skipped; the
// server still validates each row.
function BulkRebateRateDialog({
  isPending,
  onSubmit,
  onClose,
}: {
  isPending: boolean;
  onSubmit: (items: { user_id: string; rate_percent?: number | null; clear?: boolean }[]) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [raw, setRaw] = useState("");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const items: { user_id: string; rate_percent?: number | null; clear?: boolean }[] = [];
    const lines = raw.split(/\r?\n/);
    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed === "") continue;
      const parts = trimmed.split(",").map((s) => s.trim());
      if (parts.length < 2) {
        setError(t("adminAffiliates.bulkRebateBadLine") as string);
        return;
      }
      const [userID, value] = parts;
      if (value.toLowerCase() === "clear") {
        items.push({ user_id: userID, clear: true });
        continue;
      }
      const rate = Number(value);
      if (!Number.isFinite(rate)) {
        setError(t("adminAffiliates.bulkRebateBadLine") as string);
        return;
      }
      items.push({ user_id: userID, rate_percent: rate });
    }
    if (items.length === 0) {
      setError(t("adminAffiliates.bulkRebateEmpty") as string);
      return;
    }
    void onSubmit(items);
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t("adminAffiliates.bulkRebateTitle")}
          </DialogTitle>
          <DialogDescription>{t("adminAffiliates.bulkRebateBody")}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-3">
          <Label htmlFor="bulk-rebate-textarea">
            {t("adminAffiliates.bulkRebateInputLabel")}
          </Label>
          <textarea
            id="bulk-rebate-textarea"
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            rows={6}
            className="w-full rounded-xl border border-srapi-border bg-srapi-card-muted/60 px-3 py-2 text-sm tabular text-srapi-text-primary placeholder:text-srapi-text-tertiary focus:border-srapi-primary/40 focus:outline-none focus:ring-2 focus:ring-srapi-primary/20"
            placeholder="123,0.15&#10;456,clear"
          />
          {error ? (
            <p className="text-xs text-srapi-error">{error}</p>
          ) : null}
          <DialogFooter>
            <Button variant="ghost" type="button" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" loading={isPending}>
              {t("adminAffiliates.bulkRebateSubmit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
