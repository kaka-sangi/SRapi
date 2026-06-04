"use client";

import { useState } from "react";
import { AuthGate, useAuthUser } from "./auth-gate";
import { SidebarNav, SidebarBrand } from "./sidebar-nav";
import { TopNav } from "./top-nav";
import { CommandPaletteProvider } from "./command-palette";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { useRuntimeStatus } from "@/hooks/queries";

/**
 * Unified console shell: role-aware sidebar (static on desktop, drawer on
 * mobile) + sticky top nav + content. Wrap every authenticated page.
 */
export function AppShell({
  allowedRole,
  children,
}: {
  allowedRole?: "admin" | "user";
  children: React.ReactNode;
}) {
  return (
    <AuthGate allowedRole={allowedRole}>
      <ShellInner>{children}</ShellInner>
    </AuthGate>
  );
}

function ShellInner({ children }: { children: React.ReactNode }) {
  const user = useAuthUser();
  const [navOpen, setNavOpen] = useState(false);
  const runtime = useRuntimeStatus();
  const live = runtime.data?.connected ?? false;

  return (
    <div className="mx-auto flex min-h-dvh w-full max-w-[1500px]">
      {/* Desktop sidebar */}
      <aside className="sticky top-0 hidden h-dvh w-64 shrink-0 flex-col border-r border-srapi-border bg-srapi-card-muted p-4 lg:flex">
        <SidebarBrand />
        <div className="mt-2 flex-1 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <SidebarNav role={user.role} />
        </div>
        <div className="mt-auto flex items-center gap-2.5 border-t border-srapi-border px-2 pt-4">
          <div className="grid size-8 place-items-center rounded-full bg-srapi-primary/15 font-serif text-srapi-primary">
            {(user.name?.[0] ?? "U").toUpperCase()}
          </div>
          <div className="min-w-0 text-xs leading-tight">
            <div className="truncate text-srapi-text-primary">{user.name}</div>
            <div className="truncate font-mono text-2xs text-srapi-text-secondary">{user.email}</div>
          </div>
        </div>
      </aside>

      {/* Mobile drawer */}
      <Sheet open={navOpen} onOpenChange={setNavOpen}>
        <SheetContent side="left" className="p-4">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <SidebarBrand />
          <div className="mt-2 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            <SidebarNav role={user.role} onNavigate={() => setNavOpen(false)} />
          </div>
        </SheetContent>
      </Sheet>

      <CommandPaletteProvider role={user.role}>
        <div className="flex min-w-0 flex-1 flex-col">
          <TopNav user={user} onOpenNav={() => setNavOpen(true)} live={live} />
          <main className="anim-page flex-1 space-y-6 p-5 sm:p-7">{children}</main>
        </div>
      </CommandPaletteProvider>
    </div>
  );
}
