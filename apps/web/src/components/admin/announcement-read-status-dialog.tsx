"use client";

import { Users } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useAnnouncementReadStatus } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
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
  const userLookup = useUserEmailLookup();
  const query = useAnnouncementReadStatus(announcementId, open);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <div className="flex items-start gap-3">
            <IconBubble tone="accent" size="md">
              <Users />
            </IconBubble>
            <div className="min-w-0 flex-1">
              <DialogTitle className="text-lg font-semibold tracking-tight">
                {t("adminAnnouncements.readStatus")}
              </DialogTitle>
              {title ? (
                <p className="mt-0.5 truncate text-sm text-srapi-text-tertiary">{title}</p>
              ) : null}
            </div>
          </div>
          <DialogDescription className="sr-only">
            {t("adminAnnouncements.readStatus")}
          </DialogDescription>
        </DialogHeader>

        <div className="mt-2 max-h-[60vh] overflow-y-auto">
          <PageQueryState
            query={query}
            skeleton={<DialogListSkeleton rows={3} />}
          >
            {(status) =>
              status.readers.length === 0 ? (
                <IllustratedEmptyState
                  illust="users"
                  title={t("adminAnnouncements.readStatusEmpty")}
                />
              ) : (
              <>
                <div className="mb-3 flex items-center justify-between gap-2">
                  <span className="text-[11px] uppercase tracking-[0.12em] text-srapi-text-tertiary">
                    {t("adminAnnouncements.readStatusUser")}
                  </span>
                  <DataPill tone="accent" size="sm">
                    <span className="metric-secondary tabular">{status.total}</span>
                    <span className="text-srapi-text-tertiary">
                      {t("adminAnnouncements.readStatusTotal", { count: "" })
                        .replace(/\{count\}\s*/, "")
                        .trim()}
                    </span>
                  </DataPill>
                </div>

                <ol className="space-y-1.5">
                  {status.readers.map((reader) => (
                    <li
                      key={reader.user_id}
                      className="log-row flex items-center justify-between gap-3 rounded-lg border border-srapi-border bg-srapi-card px-3 py-2"
                      data-sev="success"
                    >
                      <span className="min-w-0 truncate text-sm text-srapi-text-secondary">
                        {userLookup.get(reader.user_id)}
                      </span>
                      <span className="shrink-0 font-mono text-[11px] tabular text-srapi-text-tertiary">
                        {formatDateTime(reader.read_at)}
                      </span>
                    </li>
                  ))}
                </ol>
              </>
              )
            }
          </PageQueryState>
        </div>
      </DialogContent>
    </Dialog>
  );
}
