import { redirect } from "next/navigation";

export default function ProviderAccountsPage() {
  redirect("/admin/accounts?view=health");
}
