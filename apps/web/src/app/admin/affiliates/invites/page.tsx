import { redirect } from "next/navigation";

// Legacy route: now lives as a tab on /admin/affiliates. Redirect preserves
// existing bookmarks + external links.
export default function LegacyAffiliateInvitesPage(): never {
  redirect("/admin/affiliates?tab=invites");
}
