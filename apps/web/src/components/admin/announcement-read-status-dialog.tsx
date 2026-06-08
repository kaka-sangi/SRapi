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
import { useAnnouncementReadStatus } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";

export function AnnouncementReadStatusDialog({
  announcementId,
  title,
  open,
  onOpenChange,
}: {
  announcementId: string | null;
  title: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const query = useAnnouncementReadStatus(announcementId, open);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t("adminAnnouncements.readStatus")}
            {title ? <span className="text-srapi-text-tertiary"> · {title}</span> : null}
          </DialogTitle>
        </DialogHeader>
        <div className="mt-3 max-h-[60vh] overflow-y-auto">
          <PageQueryState
            query={query}
            skeleton={<DialogListSkeleton rows={3} />}
            isEmpty={(d) => d.readers.length === 0}
            emptyTitle={t("adminAnnouncements.readStatusEmpty")}
          >
            {(status) => (
              <>
                <p className="mb-2 text-2xs text-srapi-text-tertiary">
                  {t("adminAnnouncements.readStatusTotal", { count: status.total })}
                </p>
                <TableScroll minWidth={320}>
                  <Table>
                    <TableHeader>
                      <tr>
                        <TableHead>{t("adminAnnouncements.readStatusUser")}</TableHead>
                        <TableHead>{t("adminAnnouncements.readStatusTime")}</TableHead>
                      </tr>
                    </TableHeader>
                    <TableBody>
                      {status.readers.map((reader) => (
                        <TableRow key={reader.user_id}>
                          <TableCell className="font-mono text-2xs text-srapi-text-secondary">
                            #{reader.user_id}
                          </TableCell>
                          <TableCell className="font-mono text-2xs text-srapi-text-tertiary tabular">
                            {formatDateTime(reader.read_at)}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableScroll>
              </>
            )}
          </PageQueryState>
        </div>
      </DialogContent>
    </Dialog>
  );
}
