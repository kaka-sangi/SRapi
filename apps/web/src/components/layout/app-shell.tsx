"use client";

import { useState } from "react";
import { AuthGate, useAuthUser } from "./auth-gate";
import { SidebarNav, SidebarBrand } from "./sidebar-nav";
import { TopNav } from "./top-nav";
import { CommandPaletteProvider } from "./command-palette";
import { TourProvider } from "@/components/onboarding/tour-provider";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { CopilotPet } from "@/components/admin/copilot-pet";
import { ScrollProgress } from "@/components/visual/scroll-progress";
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
    <div className="flex h-dvh w-full overflow-hidden">
      {/* Top-of-viewport scroll progress — only renders for tall pages */}
      <ScrollProgress />
      {/* Desktop sidebar — wider, brighter, soft-card user pill at the bottom.
          The right edge is a clean 1px rule (no inset glow trick) so the page
          reads as «two surfaces» rather than «one folded sheet». */}
      <aside className="sticky top-0 hidden h-dvh w-[272px] shrink-0 flex-col border-r border-srapi-border bg-srapi-card-muted/70 px-3.5 pb-4 pt-4 lg:flex">
        <SidebarBrand />
        <div className="mt-4 flex-1 overflow-y-auto pr-1 [scrollbar-width:none] [&::-webkit-scrollbar:vertical]:hidden">
          <SidebarNav role={user.role} />
        </div>
        <div className="mt-3 flex items-center gap-3 rounded-xl border border-srapi-border bg-srapi-card/85 p-3 ">
          <div className="grid size-10 place-items-center rounded-lg bg-srapi-card-muted text-base font-semibold text-srapi-text-primary">
            {(user.name?.[0] ?? "U").toUpperCase()}
          </div>
          <div className="min-w-0 text-xs leading-tight">
            <div className="truncate text-sm font-medium text-srapi-text-primary">{user.name}</div>
            <div className="truncate text-[11px] text-srapi-text-tertiary">{user.email}</div>
          </div>
        </div>
      </aside>

      {/* Mobile drawer */}
      <Sheet open={navOpen} onOpenChange={setNavOpen}>
        <SheetContent side="left" className="p-4">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <SidebarBrand />
          <div className="mt-3 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            <SidebarNav role={user.role} onNavigate={() => setNavOpen(false)} />
          </div>
        </SheetContent>
      </Sheet>

      <CommandPaletteProvider role={user.role}>
        <TourProvider>
          <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
            <TopNav user={user} onOpenNav={() => setNavOpen(true)} live={live} />
            <main className="flex-1 overflow-y-auto overscroll-contain">
              <div className="mx-auto w-full max-w-[1360px] space-y-8 px-5 py-7 sm:px-8 sm:py-9 lg:px-10">
                {children}
              </div>
            </main>
          </div>
        </TourProvider>
      </CommandPaletteProvider>

      {/* 小r — the floating AI copilot pet (admin only). */}
      {user.role === "admin" ? <CopilotPet /> : null}
    </div>
  );
}
