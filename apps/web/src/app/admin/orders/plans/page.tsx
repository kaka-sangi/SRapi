import { redirect } from "next/navigation";

// Legacy route: now a tab on /admin/billing-admin. Redirect preserves
// existing bookmarks + external links.
export default function LegacyOrderPlansPage(): never {
  redirect("/admin/billing-admin?tab=plans");
}
