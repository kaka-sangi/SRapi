"use client";

import { useEffect } from "react";
import { captureException } from "@/lib/telemetry";
import { Button } from "@/components/ui/button";
import { useLanguage } from "@/context/LanguageContext";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const { t } = useLanguage();

  useEffect(() => {
    captureException(error, { digest: error.digest ?? null });
  }, [error]);

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-5 px-6 text-center">
      <h1 className="font-serif text-3xl tracking-[-0.02em]">{t("common.error")}</h1>
      <p className="max-w-sm text-sm text-srapi-text-secondary">{t("common.errorRetryHint")}</p>
      <Button variant="primary" onClick={reset}>
        {t("common.retry")}
      </Button>
    </div>
  );
}
