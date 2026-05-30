"use client";

import * as React from "react";
import { ShieldCheck, Route, Gauge, Plug } from "lucide-react";
import { BentoGrid, BentoItem } from "@/components/ui";
import { useLanguage } from "@/context/LanguageContext";
import { useReveal } from "@/hooks/use-reveal";
import { cn } from "@/lib/cn";

type Feature = {
  icon: React.ComponentType<{ size?: number; className?: string }>;
  titleKey: string;
  bodyKey: string;
  span: 2 | 3 | 4 | 6;
};

const FEATURES: Feature[] = [
  { icon: ShieldCheck, titleKey: "mktFeat1Title", bodyKey: "mktFeat1Body", span: 4 },
  { icon: Plug, titleKey: "mktFeat4Title", bodyKey: "mktFeat4Body", span: 2 },
  { icon: Gauge, titleKey: "mktFeat3Title", bodyKey: "mktFeat3Body", span: 2 },
  { icon: Route, titleKey: "mktFeat2Title", bodyKey: "mktFeat2Body", span: 4 },
];

/**
 * Asymmetric bento of product highlights — large/small tiles alternate so the
 * grid reads as editorial rather than a uniform wall. Reveals on scroll.
 */
export function FeatureBento({ className }: { className?: string }) {
  const { t } = useLanguage();
  const ref = useReveal<HTMLDivElement>();

  return (
    <div ref={ref} className={cn("reveal", className)}>
      <BentoGrid>
        {FEATURES.map(({ icon: Icon, titleKey, bodyKey, span }) => (
          <BentoItem
            key={titleKey}
            span={span}
            className="surface surface-interactive surface-glow group flex flex-col gap-3 rounded-2xl p-5"
          >
            <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-srapi-primary/10 text-srapi-primary transition-colors group-hover:bg-srapi-primary/15">
              <Icon size={16} />
            </span>
            <span className="font-serif text-base font-medium text-srapi-text-primary">
              {t(titleKey)}
            </span>
            <span className="text-xs leading-relaxed text-srapi-text-secondary">
              {t(bodyKey)}
            </span>
          </BentoItem>
        ))}
      </BentoGrid>
    </div>
  );
}
