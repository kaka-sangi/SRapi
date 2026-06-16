import { redirect } from "next/navigation";

export default function LegacyAffiliateWithdrawalsPage(): never {
  redirect("/admin/affiliates?tab=withdrawals");
}
