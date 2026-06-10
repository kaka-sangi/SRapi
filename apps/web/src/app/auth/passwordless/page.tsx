"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { apiService } from "@/lib/api";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE, SIGN_IN_ROUTE } from "@/lib/routes";
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
      .catch(() => setError("Sign-in link expired or has already been used."));
  }, [router, token]);

  const visibleError = token ? error : "Missing sign-in token.";

  return (
    <div className="grid min-h-dvh place-items-center bg-srapi-bg px-4">
      <Card className="w-full max-w-sm p-8 text-center">
        {visibleError ? (
          <>
            <h1 className="font-serif text-2xl text-srapi-text-primary">Email sign-in failed</h1>
            <p className="mt-2 text-sm text-srapi-text-secondary">{visibleError}</p>
            <a href={SIGN_IN_ROUTE} className="mt-6 inline-block text-sm text-srapi-primary underline-offset-4 hover:underline">
              Back to sign in
            </a>
          </>
        ) : (
          <div className="flex flex-col items-center gap-3">
            <Spinner className="size-5 text-srapi-text-tertiary" />
            <p className="text-sm text-srapi-text-secondary">Signing you in...</p>
          </div>
        )}
      </Card>
    </div>
  );
}

function PasswordlessShell() {
  return (
    <div className="grid min-h-dvh place-items-center bg-srapi-bg px-4">
      <Card className="w-full max-w-sm p-8 text-center">
        <div className="flex flex-col items-center gap-3">
          <Spinner className="size-5 text-srapi-text-tertiary" />
          <p className="text-sm text-srapi-text-secondary">Signing you in...</p>
        </div>
      </Card>
    </div>
  );
}
