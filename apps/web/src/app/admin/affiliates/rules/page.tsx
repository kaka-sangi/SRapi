import { redirect } from "next/navigation";

export default function LegacyAffiliateRulesPage(): never {
  redirect("/admin/affiliates?tab=rules");
}
