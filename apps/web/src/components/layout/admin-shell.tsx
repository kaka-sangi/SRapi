import { AppShell } from "@/components/layout/app-shell";

/**
 * Thin wrapper so every admin page reads the same. Admin pages are gated to the
 * admin role (the shared sidebar already shows the admin nav for that role).
 */
export function AdminShell({ children }: { children: React.ReactNode }) {
  return <AppShell allowedRole="admin">{children}</AppShell>;
}
