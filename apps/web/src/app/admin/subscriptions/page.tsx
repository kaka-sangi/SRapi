import { redirect } from "next/navigation";

export default function LegacySubscriptionsPage(): never {
  redirect("/admin/billing-admin?tab=subscriptions");
}
