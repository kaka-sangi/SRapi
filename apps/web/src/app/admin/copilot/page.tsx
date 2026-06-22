"use client";

import Link from "next/link";
import { Settings2 } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { CopilotChat } from "@/components/admin/copilot-chat";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { ChatSkeleton } from "@/components/charts/chart-skeleton";
import { useAdminCopilotConfig } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { ADMIN_ROUTES } from "@/lib/routes";

// Kept intentionally: although this route is not in the sidebar nav, the
// floating copilot-pet's "open full page" control links here via
// ADMIN_ROUTES.copilot (see components/admin/copilot-pet.tsx), so the page is
// reachable and must not be deleted.
export default function AdminCopilotPage() {
  return (
    <AdminShell>
      <CopilotContent />
    </AdminShell>
  );
}

function CopilotContent() {
  const config = useAdminCopilotConfig();

  return (
    <PageQueryState query={config} skeleton={<ChatSkeleton />}>
      {(data) =>
        data.enabled && data.configured ? (
          <div className="h-[calc(100vh-9rem)] min-h-[30rem]">
            <CopilotChat models={data.models ?? []} defaultModel={data.model ?? ""} />
          </div>
        ) : (
          <DisabledNotice reason={!data.enabled ? "disabled" : "unconfigured"} />
        )
      }
    </PageQueryState>
  );
}

function DisabledNotice({ reason }: { reason: "disabled" | "unconfigured" }) {
  const { t } = useLanguage();
  return (
    <EmptyState
      icon={Settings2}
      title={reason === "disabled" ? t("copilot.disabledTitle") : t("copilot.unconfiguredTitle")}
      description={reason === "disabled" ? t("copilot.disabledBody") : t("copilot.unconfiguredBody")}
      action={
        <Button asChild variant="primary" size="sm">
          <Link href={`${ADMIN_ROUTES.settings}?tab=copilot`}>{t("copilot.openSettings")}</Link>
        </Button>
      }
    />
  );
}
