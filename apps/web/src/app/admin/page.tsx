import { redirect } from "next/navigation";
import { ADMIN_HOME_ROUTE } from "@/lib/routes";

export default function AdminIndexPage() {
  redirect(ADMIN_HOME_ROUTE);
}
