"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { apiService } from "@/lib/api";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE, SIGN_IN_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";

export default function PasswordlessPage() {
  return (
    <Suspense fallback={<PasswordlessShell />}>
      <PasswordlessContent />
    </Suspense>
  );
}

function PasswordlessContent() {
  const { t } = useLanguage();
  const router = useRouter();
  const params = useSearchParams();
  const token = params.get("token") || "";
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      return;
    }
    apiService.passwordlessLogin(token)
      .then((user) => {
        router.replace(user.role === "admin" ? ADMIN_HOME_ROUTE : USER_HOME_ROUTE);
      })
      .catch(() => setError(t("passwordless.expired")));
  }, [router, token, t]);

  const visibleError = token ? error : t("passwordless.missingToken");

  return (
    <div className="grid min-h-dvh place-items-center bg-srapi-bg px-4">
      <Card className="w-full max-w-sm p-8 text-center">
        {visibleError ? (
          <>
            <h1 className="text-2xl font-semibold tracking-tight text-srapi-text-primary">{t("passwordless.failed")}</h1>
            <p className="mt-2 text-sm text-srapi-text-secondary">{visibleError}</p>
            <a href={SIGN_IN_ROUTE} className="mt-6 inline-block text-sm text-srapi-primary underline-offset-4 hover:underline">
              {t("passwordless.backToSignIn")}
            </a>
          </>
        ) : (
          <div className="flex flex-col items-center gap-3">
            <Spinner className="size-5 text-srapi-text-tertiary" />
            <p className="text-sm text-srapi-text-secondary">{t("passwordless.signingIn")}</p>
          </div>
        )}
      </Card>
    </div>
  );
}

function PasswordlessShell() {
  const { t } = useLanguage();
  return (
    <div className="grid min-h-dvh place-items-center bg-srapi-bg px-4">
      <Card className="w-full max-w-sm p-8 text-center">
        <div className="flex flex-col items-center gap-3">
          <Spinner className="size-5 text-srapi-text-tertiary" />
          <p className="text-sm text-srapi-text-secondary">{t("passwordless.signingIn")}</p>
        </div>
      </Card>
    </div>
  );
}
