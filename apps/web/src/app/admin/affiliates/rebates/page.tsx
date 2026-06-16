import { redirect } from "next/navigation";

export default function LegacyAffiliateRebatesPage(): never {
  redirect("/admin/affiliates?tab=rebates");
}
