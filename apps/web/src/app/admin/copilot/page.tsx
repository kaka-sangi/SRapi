"use client";

import Link from "next/link";
import { Settings2 } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { CopilotChat } from "@/components/admin/copilot-chat";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useAdminCopilotConfig } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { ADMIN_ROUTES } from "@/lib/routes";

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
    <PageQueryState query={config} skeleton={<Skeleton className="h-[70vh] rounded-2xl" />}>
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
    <div className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-srapi-border bg-srapi-card px-6 py-16 text-center">
      <div className="flex size-12 items-center justify-center rounded-full bg-srapi-card-muted">
        <Settings2 className="size-6 text-srapi-text-tertiary" />
      </div>
      <div className="max-w-md space-y-1">
        <h3 className="font-serif text-lg text-srapi-text-primary">
          {reason === "disabled" ? t("copilot.disabledTitle") : t("copilot.unconfiguredTitle")}
        </h3>
        <p className="text-sm text-srapi-text-secondary">
          {reason === "disabled" ? t("copilot.disabledBody") : t("copilot.unconfiguredBody")}
        </p>
      </div>
      <Button asChild variant="primary" size="sm">
        <Link href={`${ADMIN_ROUTES.settings}?tab=copilot`}>{t("copilot.openSettings")}</Link>
      </Button>
    </div>
  );
}
