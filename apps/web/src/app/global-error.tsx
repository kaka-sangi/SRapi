"use client";

import { useEffect } from "react";
import { captureException } from "@/lib/telemetry";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    captureException(error, { digest: error.digest ?? null, scope: "global" });
  }, [error]);

  return (
    <html lang="en">
      <body
        style={{
          minHeight: "100dvh",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          gap: "1rem",
          fontFamily: "system-ui, sans-serif",
          textAlign: "center",
          padding: "1.5rem",
        }}
      >
        <h1 style={{ fontSize: "1.5rem" }}>SRapi failed to load</h1>
        <p style={{ color: "#6b6459", maxWidth: "24rem" }}>
          A fatal error occurred. Reload the page to continue.
        </p>
        <button
          onClick={reset}
          style={{
            borderRadius: "9999px",
            background: "#191919",
            color: "#f9f6f0",
            padding: "0.625rem 1.25rem",
            fontSize: "0.8125rem",
          }}
        >
          Reload
        </button>
      </body>
    </html>
  );
}
