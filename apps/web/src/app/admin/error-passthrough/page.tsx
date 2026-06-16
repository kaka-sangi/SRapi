import { redirect } from "next/navigation";

// Legacy route: now a tab on /admin/gateway-policies. Redirect preserves
// existing bookmarks + external links.
export default function LegacyErrorPassthroughPage(): never {
  redirect("/admin/gateway-policies?tab=error-passthrough");
}
