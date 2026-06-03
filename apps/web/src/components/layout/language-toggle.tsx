"use client";

import { Button } from "@/components/ui/button";
import { useLanguage } from "@/context/LanguageContext";

export function LanguageToggle() {
  const { toggleLanguage, t } = useLanguage();
  return (
    <Button
      variant="outline"
      size="sm"
      className="font-mono normal-case tracking-normal"
      onClick={toggleLanguage}
      aria-label={t("common.language")}
    >
      {t("common.language")}
    </Button>
  );
}
