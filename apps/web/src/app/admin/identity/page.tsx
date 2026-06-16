"use client";

import { Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AdminShell } from "@/components/layout/admin-shell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLanguage } from "@/context/LanguageContext";
import { RolesPanel } from "./_panels/roles-panel";
import { UserAttributesPanel } from "./_panels/user-attributes-panel";

// Aggregated identity-config view — replaces two standalone pages
// (/admin/roles, /admin/user-attributes) with a single tabbed surface.
// Both are *schema* config (role definitions; custom-attribute schemas)
// rather than user data — the data CRUD lives at /admin/users and stays
// standalone. Grouping them here gives the admin one place for "what
// fields exist on a user and what roles can be assigned".
//
// Standalone routes remain as 308 redirects so deeplinks + bookmarks
// keep working. Tab is URL-synced via ?tab= for share + back-button.
const TABS = ["roles", "user-attributes"] as const;

type Tab = (typeof TABS)[number];

const DEFAULT_TAB: Tab = "roles";

function isTab(value: string | null): value is Tab {
  return value !== null && (TABS as readonly string[]).includes(value);
}

export default function IdentityAdminPage() {
  return (
    <AdminShell>
      <Suspense>
        <Content />
      </Suspense>
    </AdminShell>
  );
}

function Content() {
  const { t } = useLanguage();
  const router = useRouter();
  const params = useSearchParams();
  const raw = params.get("tab");
  const active: Tab = isTab(raw) ? raw : DEFAULT_TAB;

  function setTab(next: string) {
    const q = new URLSearchParams();
    if (next !== DEFAULT_TAB) q.set("tab", next);
    const qs = q.toString();
    router.replace(`/admin/identity${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  return (
    <>
      <Tabs value={active} onValueChange={setTab}>
        <TabsList className="flex flex-wrap">
          <TabsTrigger value="roles">{t("nav.adminRoles")}</TabsTrigger>
          <TabsTrigger value="user-attributes">{t("nav.adminUserAttributes")}</TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="mt-4">
        {active === "roles" ? <RolesPanel /> : null}
        {active === "user-attributes" ? <UserAttributesPanel /> : null}
      </div>
    </>
  );
}
