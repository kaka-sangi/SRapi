"use client";

import { useState } from "react";
import { Bell } from "lucide-react";
import { useMyAnnouncements, useMarkAnnouncementRead } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";
import { formatDate } from "@/lib/admin-format";
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import type { UserAnnouncement } from "@/lib/sdk-types";

const SEVERITY_DOT: Record<string, string> = {
  info: "bg-srapi-text-tertiary",
  warning: "bg-srapi-warning",
  critical: "bg-srapi-error",
};

// Compact, notification-style timestamp ("3m" / "5h" / "2d", then a date).
function relativeTime(iso: string | undefined, justNow: string): string {
  if (!iso) return "";
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const mins = Math.floor((Date.now() - then) / 60_000);
  if (mins < 1) return justNow;
  if (mins < 60) return `${mins}m`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h`;
  const days = Math.floor(hrs / 24);
  if (days < 7) return `${days}d`;
  return formatDate(iso);
}

export function AnnouncementBell() {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const query = useMyAnnouncements();
  const markRead = useMarkAnnouncementRead();

  const items = query.data?.data ?? [];
  const unread = query.data?.unread ?? 0;
  const hasUnread = unread > 0;

  function handleSelect(item: UserAnnouncement) {
    if (!item.read) markRead.mutate(String(item.announcement.id));
  }

  async function handleMarkAll() {
    const unreadIds = items.filter((i) => !i.read).map((i) => String(i.announcement.id));
    await Promise.allSettled(unreadIds.map((id) => markRead.mutateAsync(id)));
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t("announcements.title")}
          className="relative flex size-9 items-center justify-center rounded-xl border border-srapi-border bg-srapi-card text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:text-srapi-text-primary"
        >
          <Bell className="size-4" />
          {hasUnread ? (
            <span className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-srapi-primary px-1 text-[10px] font-semibold leading-none tabular text-srapi-invert-fg">
              {unread > 9 ? "9+" : unread}
            </span>
          ) : null}
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[22rem] max-w-[calc(100vw-2rem)] p-0">
        <div className="flex items-center justify-between border-b border-srapi-border px-4 py-3">
          <span className="text-sm font-semibold tracking-tight text-srapi-text-primary">
            {t("announcements.title")}
          </span>
          {hasUnread ? (
            <button
              type="button"
              onClick={handleMarkAll}
              disabled={markRead.isPending}
              className="text-xs font-medium text-srapi-primary transition-colors hover:text-srapi-primary-hover disabled:opacity-50"
            >
              {t("announcements.markAllRead")}
            </button>
          ) : null}
        </div>

        <div className="max-h-[min(24rem,60vh)] overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar:vertical]:hidden">
          {query.isLoading ? (
            <div className="space-y-2 p-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full rounded-lg" />
              ))}
            </div>
          ) : items.length === 0 ? (
            <div className="flex flex-col items-center gap-2 px-4 py-10 text-center">
              <span className="grid size-9 place-items-center rounded-xl bg-srapi-card-muted text-srapi-text-secondary">
                <Bell className="size-4" />
              </span>
              <p className="text-xs text-srapi-text-tertiary">
                {t("announcements.empty")}
              </p>
            </div>
          ) : (
            <ul className="divide-y divide-srapi-border/70">
              {items.map((item) => {
                const a = item.announcement;
                return (
                  <li key={String(a.id)}>
                    <button
                      type="button"
                      onClick={() => handleSelect(item)}
                      className={cn(
                        "flex w-full gap-2.5 px-4 py-3 text-left transition-colors hover:bg-srapi-card-muted/60",
                        !item.read && "bg-srapi-accent-soft",
                      )}
                    >
                      <span
                        className={cn(
                          "mt-1.5 size-1.5 shrink-0 rounded-full",
                          item.read
                            ? "bg-transparent ring-1 ring-srapi-border-strong"
                            : (SEVERITY_DOT[a.severity] ?? SEVERITY_DOT.info),
                        )}
                        aria-hidden
                      />
                      <span className="min-w-0 flex-1">
                        <span className="flex items-baseline justify-between gap-2">
                          <span
                            className={cn(
                              "truncate text-sm",
                              item.read
                                ? "text-srapi-text-secondary"
                                : "font-medium text-srapi-text-primary",
                            )}
                          >
                            {a.title}
                          </span>
                          <span className="shrink-0 text-[11px] tabular text-srapi-text-tertiary">
                            {relativeTime(a.created_at, t("announcements.justNow"))}
                          </span>
                        </span>
                        <span className="mt-0.5 line-clamp-2 block text-xs leading-relaxed text-srapi-text-tertiary">
                          {a.content}
                        </span>
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
