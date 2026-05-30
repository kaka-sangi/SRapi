"use client";

import type { ReactNode } from "react";
import { AppShell } from "@/components/layout";
import { AdminSidebar } from "@/components/layout/AdminSidebar";
import { useLanguage } from "@/context/LanguageContext";

export function useAdminLocale() {
  const { language } = useLanguage();
  return {
    isZh: language === "zh",
    language,
  };
}

export function AdminShell({ children }: { children: ReactNode }) {
  return (
    <AppShell allowedRole="admin">
      <div className="grid grid-cols-1 items-start gap-8 lg:grid-cols-12">
        <div className="lg:col-span-3">
          <AdminSidebar />
        </div>
        <div className="min-w-0 space-y-8 lg:col-span-9">{children}</div>
      </div>
    </AppShell>
  );
}
