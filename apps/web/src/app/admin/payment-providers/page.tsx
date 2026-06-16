import { redirect } from "next/navigation";

export default function LegacyPaymentProvidersPage(): never {
  redirect("/admin/billing-admin?tab=payment-providers");
}
