"use client";

import * as React from "react";
import { usePathname } from "next/navigation";
import { useLanguage } from "@/context/LanguageContext";
import { useSpotlight } from "@/hooks/use-spotlight";
import { AuthGate } from "./auth-gate";
import { TopNav } from "./top-nav";
import { PageHeader } from "./page-header";

interface AppShellProps {
  children: React.ReactNode;
  allowedRole?: "admin" | "user";
}

/**
 * SRapi v0.1.0 authenticated shell. Composes auth gate + chrome + page header
 * so each route only owns its content.
 */
export function AppShell({ children, allowedRole }: AppShellProps) {
  const pathname = usePathname();
  const { t } = useLanguage();
  const meta = getRouteMeta(pathname, t);
  const spotlightRef = useSpotlight<HTMLDivElement>();

  return (
    <AuthGate allowedRole={allowedRole} loadingLabel={t("authenticating")}>
      {({ user, runtimeStatus }) => (
        <div
          ref={spotlightRef}
          className="spotlight paper-grain relative min-h-screen bg-srapi-bg pb-24 font-sans text-srapi-text-primary antialiased transition-colors duration-300"
        >
          <TopNav user={user} runtimeStatus={runtimeStatus} />
          <main className="relative z-10 mx-auto mt-12 max-w-6xl px-6 md:mt-16 md:px-8">
            <PageHeader
              category={meta.category}
              title={meta.title}
              description={meta.desc}
              user={{ name: user.name, balance: user.balance }}
              showSmoke={user.role === "admin"}
            />
            <div className="animate-bloom delay-200">{children}</div>
          </main>
        </div>
      )}
    </AuthGate>
  );
}

function getRouteMeta(
  pathname: string,
  t: (key: string, vars?: Record<string, string | number>) => string,
): { category: string; title: string; desc: string } {
  switch (pathname) {
    case "/dashboard":
      return { category: t("devCat"), title: t("devTitle"), desc: t("devDesc") };
    case "/admin":
      return { category: t("adminCat"), title: t("adminTitle"), desc: t("adminDesc") };
    case "/api-keys":
      return { category: t("apiCat"), title: t("apiTitle"), desc: t("apiDesc") };
    case "/usage":
      return { category: t("usageCat"), title: t("usageTitle"), desc: t("usageDesc") };
    case "/provider-accounts":
      return { category: t("provCat"), title: t("provTitle"), desc: t("provDesc") };
    case "/scheduler-decisions":
      return { category: t("schedCat"), title: t("schedTitle"), desc: t("schedDesc") };
    default:
      return {
        category: "SRapi",
        title: "SRapi console",
        desc: "Self-hosted AI gateway with built-in scheduler, account management and audit logs.",
      };
  }
}
