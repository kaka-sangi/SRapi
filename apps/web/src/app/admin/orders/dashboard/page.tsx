import { redirect } from "next/navigation";

// Payment dashboard rolls up into the orders list for now.
export default function OrdersDashboardPage() {
  redirect("/admin/orders");
}
