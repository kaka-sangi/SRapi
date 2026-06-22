"use client";

import { useState, useRef, useEffect } from "react";
import { usePathname } from "next/navigation";
import { AuthGate, useAuthUser } from "./auth-gate";
import { SidebarNav, SidebarBrand } from "./sidebar-nav";
import { TopNav } from "./top-nav";
import { CommandPaletteProvider } from "./command-palette";
import { TourProvider } from "@/components/onboarding/tour-provider";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { CopilotPet } from "@/components/admin/copilot-pet";
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
  const pathname = usePathname();
  const [navOpen, setNavOpen] = useState(false);
  const runtime = useRuntimeStatus();
  const live = runtime.data?.connected ?? false;

  const pageRef = useRef<HTMLDivElement>(null);
  const prevPath = useRef(pathname);
  useEffect(() => {
    if (prevPath.current !== pathname) {
      prevPath.current = pathname;
      const mm = window.matchMedia("(prefers-reduced-motion: reduce)");
      if (!mm.matches && pageRef.current) {
        pageRef.current.animate(
          [{ opacity: 0 }, { opacity: 1 }],
          { duration: 180, easing: "cubic-bezier(0.22, 1, 0.36, 1)" },
        );
      }
    }
  }, [pathname]);

  return (
    <div className="flex min-h-dvh w-full">
      {/* Desktop sidebar — pinned flush to the left edge.
          A 1px hard rule + soft inner-glow on the right edge mimics a folded
          paper crease against the page surface. */}
      <aside className="sticky top-0 hidden h-dvh w-64 shrink-0 flex-col border-r border-srapi-border bg-srapi-card-muted/85 p-4 lg:flex shadow-[inset_-1px_0_0_var(--color-srapi-border),inset_-9px_0_18px_-12px_rgba(28,26,23,0.06)]">
        <SidebarBrand />
        <div className="mt-3 flex-1 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <SidebarNav role={user.role} />
        </div>
        <div className="mt-auto flex items-center gap-3 border-t border-srapi-border pt-4">
          <div className="grid size-9 place-items-center rounded-full bg-srapi-primary/12 font-serif text-base text-srapi-primary ring-1 ring-inset ring-srapi-primary/15">
            {(user.name?.[0] ?? "U").toUpperCase()}
          </div>
          <div className="min-w-0 text-xs leading-tight">
            <div className="truncate font-medium text-srapi-text-primary">{user.name}</div>
            <div className="truncate font-mono text-2xs text-srapi-text-tertiary">{user.email}</div>
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
        <TourProvider>
          <div className="flex min-w-0 flex-1 flex-col">
            <TopNav user={user} onOpenNav={() => setNavOpen(true)} live={live} />
            <main className="flex-1">
              <div
                ref={pageRef}
                className="anim-page mx-auto w-full max-w-[1320px] space-y-7 px-5 py-6 sm:px-8 sm:py-8 lg:px-10"
              >
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
