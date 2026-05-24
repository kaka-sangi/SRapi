"use client";

import * as React from "react";
import { captureException } from "@/lib/telemetry";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  React.useEffect(() => {
    captureException(error, {
      boundary: "global",
      digest: error.digest ?? null,
    });
  }, [error]);

  return (
    <html lang="en">
      <body
        style={{
          minHeight: "100vh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          padding: "2rem",
          backgroundColor: "#F9F6F0",
          color: "#191919",
          fontFamily: "system-ui, -apple-system, sans-serif",
        }}
      >
        <div style={{ maxWidth: 540, textAlign: "center" }}>
          <h1 style={{ fontSize: "1.5rem", marginBottom: "0.75rem" }}>
            SRapi could not load
          </h1>
          <p style={{ fontSize: "0.95rem", color: "#6E6A5F", marginBottom: "1.5rem" }}>
            A critical error occurred before the application shell rendered.
            {error.digest ? ` (ref ${error.digest})` : ""}
          </p>
          <button
            onClick={() => reset()}
            style={{
              padding: "0.75rem 1.5rem",
              borderRadius: 999,
              border: "1px solid #191919",
              background: "#191919",
              color: "white",
              cursor: "pointer",
            }}
          >
            Try again
          </button>
        </div>
      </body>
    </html>
  );
}
