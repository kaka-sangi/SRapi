"use client";

import { AppShell } from "@/components/layout/app-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { SectionHero } from "@/components/visual/section-hero";
import { PlaygroundChat } from "@/components/playground/playground-chat";
import { ChatSkeleton } from "@/components/charts/chart-skeleton";
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
      <SectionHero
        eyebrow={t("playground.eyebrow")}
        title={t("nav.playground")}
        description={t("playground.subtitle")}
      />
      <PageQueryState query={models} skeleton={<ChatSkeleton />}>
        {(data) => {
          const names = data.map((m) => m.id);
          return <PlaygroundChat models={names} defaultModel={names[0] ?? ""} />;
        }}
      </PageQueryState>
    </>
  );
}
