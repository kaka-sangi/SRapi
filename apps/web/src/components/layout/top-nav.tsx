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
    <header className="sticky top-0 z-20 flex items-center gap-3 border-b border-srapi-border bg-srapi-bg/85 px-4 py-3 backdrop-blur sm:px-5">
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
        className="flex w-full max-w-xs items-center gap-2 rounded-full border border-srapi-border bg-srapi-card-muted px-3 py-1.5 text-sm text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:text-srapi-text-primary"
      >
        <Search className="size-4" />
        <span className="hidden truncate sm:inline">{t("common.search")}</span>
        <span className="ml-auto hidden font-mono text-2xs sm:inline">⌘K</span>
      </button>

      <div className="ml-auto flex items-center gap-2">
        <span className="hidden items-center gap-1.5 font-mono text-2xs text-srapi-text-secondary sm:flex">
          <span
            className={cn(
              "size-1.5 rounded-full",
              live ? "bg-srapi-success" : "bg-srapi-text-secondary",
            )}
          />
          {live ? t("common.live") : t("common.apiOffline")}
        </span>
        <AnnouncementBell />
        <LanguageToggle />
        <ThemeToggle />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="icon" aria-label={user.name}>
              <span className="font-serif text-sm text-srapi-primary">
                {(user.name?.[0] ?? "U").toUpperCase()}
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>
              <div className="text-srapi-text-primary">{user.name}</div>
              <div className="mt-0.5 flex items-center gap-1.5 font-mono text-2xs font-normal text-srapi-text-tertiary">
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
