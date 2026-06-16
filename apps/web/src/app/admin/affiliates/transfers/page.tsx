import { redirect } from "next/navigation";

export default function LegacyAffiliateTransfersPage(): never {
  redirect("/admin/affiliates?tab=transfers");
}
