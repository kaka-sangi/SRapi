"use client";

import * as React from "react";
import { useRouter, usePathname } from "next/navigation";
import { LogOut, Server } from "lucide-react";
import { Badge } from "@/components/ui";
import { cn } from "@/lib/cn";
import { apiService, type ApiRuntimeStatus } from "@/lib/api";
import { useLanguage } from "@/context/LanguageContext";
import { LanguageToggle } from "./language-toggle";
import { ThemeToggle } from "./theme-toggle";
import { RoleSwitcher } from "./role-switcher";

interface TopNavProps {
  user: { role: "admin" | "user"; authMode?: "live" | "demo" };
  runtimeStatus: ApiRuntimeStatus | null;
}

const userNavigation = [
  { key: "navOverview", href: "/dashboard" },
  { key: "navApiKeys", href: "/api-keys" },
  { key: "navUsageHistory", href: "/usage" },
] as const;

const adminNavigation = [
  { key: "navOverview", href: "/admin" },
  { key: "navProviderAccounts", href: "/provider-accounts" },
  { key: "navSchedulerDecisions", href: "/scheduler-decisions" },
  { key: "navUsageLogs", href: "/usage" },
] as const;

export function TopNav({ user, runtimeStatus }: TopNavProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { t } = useLanguage();
  const isDemoRuntime = user.authMode === "demo" || runtimeStatus?.mode === "demo";

  const handleLogout = async () => {
    await apiService.logout();
    router.push("/");
  };

  const navigation = user.role === "admin" ? adminNavigation : userNavigation;

  return (
    <header className="sticky top-0 z-40 animate-bloom border-b border-srapi-border bg-srapi-bg/85 backdrop-blur-md">
      <div className="mx-auto flex h-20 max-w-6xl items-center justify-between px-6 md:px-8">
        <div className="flex items-center gap-4">
          <a
            href={user.role === "admin" ? "/admin" : "/dashboard"}
            className="font-serif text-xl font-medium italic tracking-tight text-srapi-primary"
          >
            SRapi.
          </a>
          <Badge>{user.role === "admin" ? t("operatorConsole") : t("developerConsole")}</Badge>
          <span
            title={runtimeStatus?.apiBaseUrl ?? ""}
            className={cn(
              "hidden items-center gap-1.5 rounded-full border px-2.5 py-0.5 font-mono text-[11px] font-bold uppercase tracking-wider md:inline-flex",
              isDemoRuntime
                ? "border-srapi-primary/30 bg-srapi-primary/5 text-srapi-primary"
                : "border-srapi-success/30 bg-srapi-success/5 text-srapi-success",
            )}
          >
            <Server size={11} aria-hidden="true" />
            {isDemoRuntime ? t("demoData") : t("liveApi")}
          </span>
        </div>

        <div className="flex items-center gap-6 md:gap-8">
          <nav className="flex gap-6 font-mono text-xs uppercase tracking-widest text-srapi-text-secondary md:gap-8">
            {navigation.map((item) => {
              const isActive = pathname === item.href;
              return (
                <a
                  key={item.key}
                  href={item.href}
                  aria-current={isActive ? "page" : undefined}
                  className={cn(
                    "transition-colors hover:text-srapi-primary",
                    isActive && "border-b border-srapi-primary pb-0.5 font-bold text-srapi-primary",
                  )}
                >
                  {t(item.key)}
                </a>
              );
            })}
          </nav>

          <div className="flex items-center gap-4 border-l border-srapi-border pl-4 md:gap-6 md:pl-6">
            <LanguageToggle />
            <RoleSwitcher currentRole={user.role} authMode={user.authMode} />
            <ThemeToggle />
            <button
              type="button"
              onClick={handleLogout}
              title={t("terminateSession")}
              aria-label={t("terminateSession")}
              className={cn(
                "rounded-full border border-srapi-error/30 p-1.5 text-srapi-error transition-colors",
                "hover:bg-srapi-error/5",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-error focus-visible:ring-offset-2 focus-visible:ring-offset-srapi-bg",
              )}
            >
              <LogOut size={14} aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
    </header>
  );
}
