"use client";

import { PageHeader } from "@/components/layout/page-header";
import { Card } from "@/components/ui/card";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { useLanguage } from "@/context/LanguageContext";

/**
 * Honest placeholder for admin surfaces whose editor is not rebuilt yet.
 * Per the content rule: don't fake data — state plainly that it's in progress.
 */
export function ComingSoon({ title, subtitle }: { title: string; subtitle: string }) {
  const { t } = useLanguage();
  return (
    <>
      <PageHeader eyebrow={t("nav.sectionAdmin")} title={title} description={subtitle} />
      <Card>
        <IllustratedEmptyState illust="cog" title={title} description={t("adminSettings.comingSoon")} />
      </Card>
    </>
  );
}
