import { redirect } from "next/navigation";

export default function LegacyUserAttributesPage(): never {
  redirect("/admin/identity?tab=user-attributes");
}
