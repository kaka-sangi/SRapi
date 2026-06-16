import { redirect } from "next/navigation";

export default function LegacyErrorLogsPage(): never {
  redirect("/admin/logs?tab=error");
}
