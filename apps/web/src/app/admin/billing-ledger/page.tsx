import { redirect } from "next/navigation";

export default function LegacyBillingLedgerPage(): never {
  redirect("/admin/logs?tab=billing-ledger");
}
