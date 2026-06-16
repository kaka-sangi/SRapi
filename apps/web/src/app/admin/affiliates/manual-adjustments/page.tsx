import { redirect } from "next/navigation";

export default function LegacyAffiliateManualAdjustmentsPage(): never {
  redirect("/admin/affiliates?tab=manual-adjustments");
}
