"use client";

import * as React from "react";
import { Button, Card, CardDescription, CardTitle } from "@/components/ui";

export default function RootError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  React.useEffect(() => {
    if (process.env.NODE_ENV !== "production") {
      console.error("[srapi] route error", error);
    }
  }, [error]);

  return (
    <div className="flex min-h-[60vh] items-center justify-center p-8">
      <Card className="max-w-lg space-y-6 text-center">
        <div className="space-y-2">
          <CardTitle>Something went wrong</CardTitle>
          <CardDescription>
            SRapi could not render this page. The control plane and your data are unaffected.
          </CardDescription>
        </div>
        {error.digest ? (
          <p className="font-mono text-[11px] text-srapi-text-secondary">
            Error reference: {error.digest}
          </p>
        ) : null}
        <div className="flex justify-center gap-3">
          <Button variant="outline" onClick={() => (window.location.href = "/")}>
            Back to home
          </Button>
          <Button onClick={() => reset()}>Try again</Button>
        </div>
      </Card>
    </div>
  );
}
