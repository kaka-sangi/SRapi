import { redirect } from "next/navigation";

// Legacy route: now a tab on /admin/identity. Redirect preserves existing
// bookmarks + external links.
export default function LegacyRolesPage(): never {
  redirect("/admin/identity?tab=roles");
}
