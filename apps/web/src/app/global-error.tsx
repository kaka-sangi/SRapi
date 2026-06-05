"use client";

import { useEffect, useState } from "react";
import { captureException } from "@/lib/telemetry";

// global-error renders outside the app's LanguageProvider, so it can't use the
// i18n context. Read the persisted locale directly (same key the context uses)
// so a fatal error is at least shown in the user's chosen language.
const FATAL_COPY = {
  en: {
    title: "SRapi failed to load",
    body: "A fatal error occurred. Reload the page to continue.",
    reload: "Reload",
  },
  zh: {
    title: "SRapi 加载失败",
    body: "发生了致命错误，请重新加载页面以继续。",
    reload: "重新加载",
  },
} as const;

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const [lang] = useState<"en" | "zh">(() => {
    if (typeof window === "undefined") return "en";
    return window.localStorage.getItem("srapi_lang") === "zh" ? "zh" : "en";
  });
  const copy = FATAL_COPY[lang];

  useEffect(() => {
    captureException(error, { digest: error.digest ?? null, scope: "global" });
  }, [error]);

  return (
    <html lang={lang}>
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
        <h1 style={{ fontSize: "1.5rem" }}>{copy.title}</h1>
        <p style={{ color: "#6b6459", maxWidth: "24rem" }}>{copy.body}</p>
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
          {copy.reload}
        </button>
      </body>
    </html>
  );
}
