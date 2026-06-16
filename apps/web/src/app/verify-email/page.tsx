"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { apiService } from "@/lib/api";
import { meErrorMessage } from "@/lib/me-api";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

/**
 * Landing page for the email-verification link the backend emails
 * (/verify-email?token=…). Reads the token, confirms it against
 * POST /api/v1/auth/email-verification/confirm, and shows the result. Without
 * this page the verification link 404'd and email verification was unreachable.
 */
type State = { kind: "verifying" } | { kind: "ok" } | { kind: "error"; message: string };

export default function VerifyEmailPage() {
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
      <div className="animate-bloom w-full max-w-sm">
        <Card className="card-raised p-8 text-center">
          {state.kind === "verifying" && (
            <>
              <Loader2 className="mx-auto size-8 animate-spin text-srapi-text-tertiary" />
              <h1 className="mt-4 font-serif text-xl text-srapi-text-primary">Verifying your email…</h1>
            </>
          )}
          {state.kind === "ok" && (
            <>
              <CheckCircle2 className="mx-auto size-8 text-srapi-success" />
              <h1 className="mt-4 font-serif text-xl text-srapi-text-primary">Email verified</h1>
              <p className="mt-2 text-sm text-srapi-text-secondary">
                Your email address has been verified. You can now sign in.
              </p>
              <Link href="/" className="mt-6 block">
                <Button variant="primary" size="lg" className="w-full">
                  Continue to sign in
                </Button>
              </Link>
            </>
          )}
          {state.kind === "error" && (
            <>
              <XCircle className="mx-auto size-8 text-srapi-error" />
              <h1 className="mt-4 font-serif text-xl text-srapi-text-primary">Verification failed</h1>
              <p className="mt-2 text-sm text-srapi-text-secondary">{state.message}</p>
              <Link href="/" className="mt-6 block">
                <Button variant="outline" size="lg" className="w-full">
                  Back to sign in
                </Button>
              </Link>
            </>
          )}
        </Card>
      </div>
    </main>
  );
}
