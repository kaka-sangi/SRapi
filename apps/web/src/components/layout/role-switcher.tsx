"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { UserCheck } from "lucide-react";
import { apiService } from "@/lib/api";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";

interface RoleSwitcherProps {
  currentRole: "admin" | "user";
  authMode: "live" | "demo" | undefined;
  className?: string;
}

/**
 * SRapi v0.1.0 demo-only role switcher. Hidden in live mode because role
 * changes there must go through the backend.
 */
export function RoleSwitcher({ currentRole, authMode, className }: RoleSwitcherProps) {
  const router = useRouter();
  const { t } = useLanguage();
  if (authMode !== "demo") return null;

  const handleSwitch = async () => {
    const newRole = currentRole === "admin" ? "user" : "admin";
    const targetEmail = newRole === "admin" ? "admin@srapi.local" : "developer@srapi.local";
    await apiService.login(targetEmail, "password123");
    router.push(newRole === "admin" ? "/admin" : "/dashboard");
  };

  return (
    <button
      type="button"
      onClick={handleSwitch}
      title={t("demoOnlySwitch")}
      aria-label={t("demoOnlySwitch")}
      className={cn(
        "hidden rounded-full border border-srapi-border p-1.5 text-srapi-text-secondary transition-colors sm:block",
        "hover:bg-srapi-card-muted hover:text-srapi-text-primary",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary focus-visible:ring-offset-2 focus-visible:ring-offset-srapi-bg",
        className,
      )}
    >
      <UserCheck size={14} aria-hidden="true" />
    </button>
  );
}
