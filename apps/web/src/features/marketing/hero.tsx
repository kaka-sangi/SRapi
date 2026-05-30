"use client";

import * as React from "react";
import { ArrowDown } from "lucide-react";
import { GradientText, buttonVariants } from "@/components/ui";
import { useLanguage } from "@/context/LanguageContext";
import { cn } from "@/lib/cn";

/**
 * Marketing hero. The headline keeps the contiguous phrase
 * "One endpoint, every provider." (asserted by the landing e2e) and adds a
 * restrained spectrum accent on the second line.
 */
export function Hero() {
  const { t } = useLanguage();

  return (
    <div className="max-w-xl space-y-7">
      <div className="font-mono text-2xs font-bold uppercase tracking-[0.2em] text-srapi-primary">
        {t("mktEyebrow")}
      </div>

      <h1 className="font-serif text-display-sm font-normal leading-[1.05] tracking-tight text-srapi-text-primary md:text-display-md">
        {t("mktHeroLead")}{" "}
        <GradientText>{t("mktHeroAccent")}</GradientText>
      </h1>

      <p className="max-w-lg text-sm leading-relaxed text-srapi-text-secondary">
        {t("mktHeroSub")}
      </p>

      <div className="flex flex-wrap items-center gap-3 lg:hidden">
        <a href="#login" className={cn(buttonVariants({ variant: "primary", size: "lg" }))}>
          {t("mktCtaStart")}
          <ArrowDown size={14} aria-hidden="true" />
        </a>
      </div>
    </div>
  );
}
