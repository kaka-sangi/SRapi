"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { useLanguage } from "@/context/LanguageContext";

export default function NotFound() {
  const { t } = useLanguage();

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-5 px-6 text-center">
      <div className="font-mono text-2xs uppercase tracking-widest text-srapi-text-secondary">
        404
      </div>
      <h1 className="font-serif text-4xl tracking-[-0.02em]">{t("common.notFound")}</h1>
      <p className="max-w-sm text-sm text-srapi-text-secondary">{t("common.notFoundBody")}</p>
      <Button asChild variant="primary">
        <Link href="/">{t("common.backHome")}</Link>
      </Button>
    </div>
  );
}
