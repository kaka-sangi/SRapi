"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { apiService } from "@/lib/api";

/**
 * FirstRunRedirect sends visitors to the first-run setup wizard while the
 * system has no owner/admin account yet. It renders nothing and is a no-op on
 * already-provisioned systems (the backend memoizes that an administrator
 * exists, so the status check stays cheap).
 */
export function FirstRunRedirect() {
  const router = useRouter();
  useEffect(() => {
    let active = true;
    apiService
      .getSetupStatus()
      .then((needsSetup) => {
        if (active && needsSetup) {
          router.replace("/setup");
        }
      })
      .catch(() => {
        // If the status check fails (e.g. backend offline), don't block the
        // login page — the normal sign-in flow surfaces connectivity errors.
      });
    return () => {
      active = false;
    };
  }, [router]);
  return null;
}
