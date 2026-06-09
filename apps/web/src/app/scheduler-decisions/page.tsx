import { redirect } from "next/navigation";

export default function SchedulerDecisionsPage() {
  redirect("/admin/ops?tab=scheduler-decisions");
}
