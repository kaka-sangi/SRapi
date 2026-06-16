import { redirect } from "next/navigation";

export default function LegacyAuditLogsPage(): never {
  redirect("/admin/logs");
}
