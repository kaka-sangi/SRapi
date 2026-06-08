"use client";

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableScroll,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useAdminPromoCodeUsages } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";

export function PromoCodeUsagesDialog({
  promoId,
  code,
  open,
  onOpenChange,
}: {
  promoId: string | null;
  code: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const query = useAdminPromoCodeUsages(promoId, open);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("adminPromos.usagesTitle")}
            {code ? <span className="font-mono text-srapi-text-tertiary"> · {code}</span> : null}
          </DialogTitle>
        </DialogHeader>
        <div className="mt-3 max-h-[60vh] overflow-y-auto">
          <PageQueryState
            query={query}
            skeleton={<DialogListSkeleton rows={4} />}
            isEmpty={(d) => d.data.length === 0}
            emptyTitle={t("adminPromos.usagesEmpty")}
          >
            {(result) => (
              <TableScroll minWidth={520}>
                <Table>
                  <TableHeader>
                    <tr>
                      <TableHead>{t("adminPromos.usagesTime")}</TableHead>
                      <TableHead>{t("adminPromos.usagesUser")}</TableHead>
                      <TableHead>{t("adminPromos.usagesOrder")}</TableHead>
                      <TableHead>{t("adminPromos.usagesDiscount")}</TableHead>
                      <TableHead>{t("adminPromos.usagesFinal")}</TableHead>
                    </tr>
                  </TableHeader>
                  <TableBody>
                    {result.data.map((usage) => (
                      <TableRow key={usage.id}>
                        <TableCell className="font-mono text-2xs text-srapi-text-tertiary tabular">
                          {formatDateTime(usage.applied_at)}
                        </TableCell>
                        <TableCell className="font-mono text-2xs text-srapi-text-secondary">
                          #{usage.user_id}
                        </TableCell>
                        <TableCell className="font-mono text-2xs text-srapi-text-tertiary">
                          {usage.order_no}
                        </TableCell>
                        <TableCell className="tabular">
                          {formatMoney(usage.discount_amount, usage.currency)}
                        </TableCell>
                        <TableCell className="tabular">
                          {formatMoney(usage.final_amount, usage.currency)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableScroll>
            )}
          </PageQueryState>
        </div>
      </DialogContent>
    </Dialog>
  );
}
