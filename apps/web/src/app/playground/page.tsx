"use client";

import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { PlaygroundChat } from "@/components/playground/playground-chat";
import { Skeleton } from "@/components/ui/skeleton";
import { usePlaygroundModels } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";

export default function PlaygroundPage() {
  return (
    <AppShell allowedRole="user">
      <PlaygroundContent />
    </AppShell>
  );
}

function PlaygroundContent() {
  const { t } = useLanguage();
  const models = usePlaygroundModels();
  return (
    <>
      <PageHeader title={t("nav.playground")} />
      <PageQueryState query={models} skeleton={<Skeleton className="h-[70vh] rounded-2xl" />}>
        {(data) => {
          const names = data.map((m) => m.id);
          return <PlaygroundChat models={names} defaultModel={names[0] ?? ""} />;
        }}
      </PageQueryState>
    </>
  );
}
