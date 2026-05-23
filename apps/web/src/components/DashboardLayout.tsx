"use client";

import * as React from "react";
import { AppShell } from "@/components/layout";

interface DashboardLayoutProps {
  children: React.ReactNode;
  allowedRole?: "admin" | "user";
}

/**
 * SRapi v0.1.0 compatibility wrapper.
 *
 * The original 480-line DashboardLayout has been split into focused
 * components under `@/components/layout` (AppShell, AuthGate, TopNav,
 * PageHeader, SmokeDrawer, ThemeToggle, LanguageToggle, RoleSwitcher).
 * Pages keep importing from this path while migration completes; new code
 * should import `AppShell` directly.
 */
export default function DashboardLayout({ children, allowedRole }: DashboardLayoutProps) {
  return <AppShell allowedRole={allowedRole}>{children}</AppShell>;
}
