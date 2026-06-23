"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { apiService } from "@/lib/api";
import { meErrorMessage } from "@/lib/me-api";
import { useLanguage } from "@/context/LanguageContext";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconBubble } from "@/components/ui/icon-bubble";

/**
 * Landing page for the email-verification link the backend emails
 * (/verify-email?token=…). Reads the token, confirms it against
 * POST /api/v1/auth/email-verification/confirm, and shows the result. Without
 * this page the verification link 404'd and email verification was unreachable.
 */
type State = { kind: "verifying" } | { kind: "ok" } | { kind: "error"; message: string };

export default function VerifyEmailPage() {
  const { t } = useLanguage();
  const [state, setState] = useState<State>({ kind: "verifying" });

  useEffect(() => {
    let active = true;
    const run = async () => {
      const token = new URLSearchParams(window.location.search).get("token") || "";
      if (!token) throw new Error("Missing verification token.");
      await apiService.confirmEmailVerification(token);
    };
    run()
      .then(() => active && setState({ kind: "ok" }))
      .catch((err) => active && setState({ kind: "error", message: meErrorMessage(err) }));
    return () => {
      active = false;
    };
  }, []);

  return (
    <main className="flex min-h-dvh items-center justify-center px-6 py-10">
      <div className="w-full max-w-sm">
        <Card className="p-8 text-center">
          {state.kind === "verifying" && (
            <>
              <IconBubble tone="neutral" size="lg" className="mx-auto">
                <Loader2 className="animate-spin" />
              </IconBubble>
              <h1 className="mt-4 text-xl font-semibold tracking-tight text-srapi-text-primary">{t("verifyEmail.verifying")}</h1>
            </>
          )}
          {state.kind === "ok" && (
            <>
              <IconBubble tone="success" size="lg" className="mx-auto">
                <CheckCircle2 />
              </IconBubble>
              <h1 className="mt-4 text-xl font-semibold tracking-tight text-srapi-text-primary">{t("verifyEmail.successTitle")}</h1>
              <p className="mt-2 text-sm text-srapi-text-secondary">
                {t("verifyEmail.successBody")}
              </p>
              <Link href="/" className="mt-6 block">
                <Button variant="primary" size="lg" className="h-11 w-full rounded-xl">
                  {t("verifyEmail.successCta")}
                </Button>
              </Link>
            </>
          )}
          {state.kind === "error" && (
            <>
              <IconBubble tone="error" size="lg" className="mx-auto">
                <XCircle />
              </IconBubble>
              <h1 className="mt-4 text-xl font-semibold tracking-tight text-srapi-text-primary">{t("verifyEmail.failTitle")}</h1>
              <p className="mt-2 rounded-xl bg-srapi-error/10 px-3 py-2 text-sm text-srapi-error">{state.message}</p>
              <Link href="/" className="mt-6 block">
                <Button variant="outline" size="lg" className="h-11 w-full rounded-xl">
                  {t("verifyEmail.failCta")}
                </Button>
              </Link>
            </>
          )}
        </Card>
      </div>
    </main>
  );
}
