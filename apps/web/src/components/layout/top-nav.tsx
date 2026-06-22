"use client";

import { useRouter } from "next/navigation";
import { Menu, Search, LogOut, User as UserIcon } from "lucide-react";
import { apiService } from "@/lib/api";
import type { CurrentUser } from "@/lib/srapi-types";
import { SIGN_IN_ROUTE } from "@/lib/routes";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { ThemeToggle } from "./theme-toggle";
import { LanguageToggle } from "./language-toggle";
import { AnnouncementBell } from "./announcement-bell";
import { useCommandPalette } from "./command-palette";

export function TopNav({
  user,
  onOpenNav,
  live,
}: {
  user: CurrentUser;
  onOpenNav: () => void;
  live?: boolean;
}) {
  const router = useRouter();
  const { t } = useLanguage();
  const { open: openCommand } = useCommandPalette();

  async function handleSignOut() {
    await apiService.logout();
    router.replace(SIGN_IN_ROUTE);
  }

  return (
    <header className="sticky top-0 z-20 flex items-center gap-3 border-b border-srapi-border bg-srapi-bg/85 px-4 py-3.5 backdrop-blur-md sm:px-7">
      <Button
        variant="outline"
        size="icon"
        className="lg:hidden"
        onClick={onOpenNav}
        aria-label={t("common.openNav")}
      >
        <Menu className="size-4" />
      </Button>

      <button
        type="button"
        onClick={openCommand}
        data-tour="search-bar"
        className="group flex h-10 w-full max-w-md items-center gap-2.5 rounded-xl border border-srapi-border bg-srapi-card/85 px-3.5 text-sm text-srapi-text-secondary transition-[border-color,color,background-color,box-shadow] duration-150 ease-[var(--ease-out-quint)] hover:border-srapi-border-strong hover:bg-srapi-card hover:text-srapi-text-primary focus-visible:border-srapi-border-strong"
      >
        <Search className="size-4 text-srapi-text-tertiary transition-colors group-hover:text-srapi-text-secondary" />
        <span className="hidden truncate sm:inline">{t("common.search")}</span>
        <span className="ml-auto hidden rounded-md border border-srapi-border bg-srapi-card-muted px-1.5 py-0.5 font-mono text-[10px] tracking-wider text-srapi-text-tertiary sm:inline">
          ⌘K
        </span>
      </button>

      <div className="ml-auto flex items-center gap-2">
        <span
          className={cn(
            "hidden items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium sm:inline-flex",
            live
              ? "bg-srapi-success/10 text-srapi-success"
              : "bg-srapi-card-muted text-srapi-text-tertiary",
          )}
        >
          <span
            className={cn(
              "relative inline-block size-1.5 rounded-full",
              live ? "bg-srapi-success" : "bg-srapi-text-tertiary",
            )}
          >
            {live ? (
              <span
                className="absolute inset-0 -m-1 animate-ping rounded-full bg-srapi-success/40"
                aria-hidden
              />
            ) : null}
          </span>
          {live ? t("common.live") : t("common.apiOffline")}
        </span>
        <AnnouncementBell />
        <LanguageToggle />
        <ThemeToggle />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="icon" aria-label={user.name}>
              <span className="text-sm font-semibold text-srapi-primary">
                {(user.name?.[0] ?? "U").toUpperCase()}
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>
              <div className="text-srapi-text-primary">{user.name}</div>
              <div className="mt-0.5 flex items-center gap-1.5 text-[11px] font-normal text-srapi-text-tertiary">
                <UserIcon className="size-3" aria-hidden />
                {user.email} ·{" "}
                {user.role === "admin" ? t("nav.sectionAdmin") : t("nav.sectionWorkspace")}
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem destructive onClick={handleSignOut}>
              <LogOut className="size-4" />
              {t("nav.signOut")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
