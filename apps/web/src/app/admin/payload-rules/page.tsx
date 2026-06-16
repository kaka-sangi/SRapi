import { redirect } from "next/navigation";

export default function LegacyPayloadRulesPage(): never {
  redirect("/admin/gateway-policies?tab=payload-rules");
}
