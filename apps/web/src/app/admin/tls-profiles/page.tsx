import { redirect } from "next/navigation";

export default function LegacyTlsProfilesPage(): never {
  redirect("/admin/gateway-policies?tab=tls-profiles");
}
